// 收款账本 / Payment Receipt Ledger
// 功能：原子记录到账与退款，区分重复/迟到款，按支付引用幂等并保留独立履约记录
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 16:30:00
package payment

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/kartwo/kartwo/internal/mail"
	"github.com/kartwo/kartwo/internal/order"
	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

type receipt struct {
	ID, OrderID, Amount, Refunded int64
	Currency, Review              string
}

func getReceipt(ctx context.Context, tx *sql.Tx, provider, ref string) (receipt, error) {
	var p receipt
	err := tx.QueryRowContext(ctx, `SELECT id,order_id,amount_cents,refunded_cents,currency,review_reason FROM payment_receipt WHERE provider=? AND payment_ref=?`, provider, ref).
		Scan(&p.ID, &p.OrderID, &p.Amount, &p.Refunded, &p.Currency, &p.Review)
	return p, err
}

// ensureLegacyReceipt imports the primary payment of an existing order, including
// databases restored from older releases. It never invents a refund from status.
func ensureLegacyReceipt(ctx context.Context, tx *sql.Tx, ord sqlcgen.GetOrderByPublicIDRow) error {
	if ord.PaymentRef == "" || ord.PaymentProvider == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO payment_receipt(order_id,provider,payment_ref,amount_cents,currency,refunded_cents,review_reason,source)
        SELECT ?,?,?,?,?,MIN(?,COALESCE(SUM(amount_cents),0)),?,'legacy' FROM refund WHERE order_id=? AND provider=?
        ON CONFLICT(provider,payment_ref) DO NOTHING`, ord.ID, ord.PaymentProvider, ord.PaymentRef, ord.TotalCents, strings.ToUpper(ord.Currency), ord.TotalCents, legacyReview(ord.Status), ord.ID, ord.PaymentProvider)
	return err
}

func legacyReview(status string) string {
	switch status {
	case "payment_review":
		return "late_payment"
	case "refunded":
		return "legacy_refund"
	default:
		return ""
	}
}

func recordEvent(ctx context.Context, q *sqlcgen.Queries, provider string, ev WebhookEvent) (bool, error) {
	if ev.ID == "" {
		return false, ErrMismatch
	}
	err := q.InsertWebhookEvent(ctx, sqlcgen.InsertWebhookEventParams{Provider: provider, EventID: ev.ID, EventType: ev.Type, OrderRef: ev.OrderRef})
	if isUniqueViolation(err) {
		return false, nil
	}
	return err == nil, err
}

func (s *Service) recordPayment(ctx context.Context, provider string, ev WebhookEvent, webhook bool) error {
	if ev.PaymentRef == "" || ev.OrderRef == "" || ev.AmountCents <= 0 {
		return ErrMismatch
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	q := s.q.WithTx(tx)
	if webhook {
		added, err := recordEvent(ctx, q, provider, ev)
		if err != nil || !added {
			return err
		}
	}
	ord, err := q.GetOrderByPublicID(ctx, ev.OrderRef)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMismatch
	}
	if err != nil {
		return err
	}
	if ord.TotalCents != ev.AmountCents || !strings.EqualFold(ord.Currency, ev.Currency) {
		return ErrMismatch
	}
	if err := ensureLegacyReceipt(ctx, tx, ord); err != nil {
		return err
	}
	existing, err := getReceipt(ctx, tx, provider, ev.PaymentRef)
	if err == nil {
		if existing.OrderID != ord.ID || existing.Amount != ev.AmountCents || !strings.EqualFold(existing.Currency, ev.Currency) {
			return ErrMismatch
		}
		return tx.Commit() // Same payment, different event ID or capture response.
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err := order.ExpirePendingTx(ctx, tx, ord.ID, time.Now()); err != nil {
		return err
	}
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM "order" WHERE id=?`, ord.ID).Scan(&status); err != nil {
		return err
	}
	reason := ""
	switch status {
	case "pending":
	case "cancelled":
		reason = "late_payment"
	default:
		reason = "duplicate_payment"
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO payment_receipt(order_id,provider,payment_ref,amount_cents,currency,review_reason) VALUES(?,?,?,?,?,?)`, ord.ID, provider, ev.PaymentRef, ev.AmountCents, strings.ToUpper(ev.Currency), reason); err != nil {
		return err
	}
	switch status {
	case "pending":
		if _, err := q.MarkOrderPaidByPublicID(ctx, sqlcgen.MarkOrderPaidByPublicIDParams{PaymentProvider: provider, PaymentRef: ev.PaymentRef, PublicID: ord.PublicID}); err != nil {
			return err
		}
		if err := order.FinalizePaidReservationTx(ctx, tx, ord.ID); err != nil {
			return err
		}
		if err := mail.EnqueueOrderConfirmation(ctx, q, mail.OrderInfo{ID: ord.ID, PublicID: ord.PublicID, Email: ord.Email, Currency: ord.Currency, TotalCents: ord.TotalCents}); err != nil {
			return err
		}
	case "cancelled":
		if _, err := tx.ExecContext(ctx, `UPDATE "order" SET status='payment_review',payment_provider=?,payment_ref=?,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?`, provider, ev.PaymentRef, ord.ID); err != nil {
			return err
		}
	default:
		// Keep the original payment reference: a second payment must not replace
		// the first or reserve/deduct inventory or send confirmation again.
		if _, err := tx.ExecContext(ctx, `UPDATE "order" SET status='payment_review',updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?`, ord.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Service) markRefundedFromWebhook(ctx context.Context, provider string, ev WebhookEvent) error {
	if ev.PaymentRef == "" || ev.AmountCents <= 0 || ev.Currency == "" {
		return ErrMismatch
	}
	if provider == "stripe" && (ev.RefundedCents <= 0 || ev.RefundedCents > ev.AmountCents) {
		return ErrMismatch
	}
	if provider == "paypal" && (ev.RefundRef == "" || ev.RefundedCents < ev.AmountCents) {
		return ErrMismatch
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	q := s.q.WithTx(tx)
	added, err := recordEvent(ctx, q, provider, ev)
	if err != nil || !added {
		return err
	}
	p, err := getReceipt(ctx, tx, provider, ev.PaymentRef)
	if errors.Is(err, sql.ErrNoRows) {
		// A refund may arrive before payment confirmation. Unknown references
		// are retried (event insert rolls back), never acknowledged and lost.
		var publicID string
		if err := tx.QueryRowContext(ctx, `SELECT public_id FROM "order" WHERE payment_provider=? AND payment_ref=?`, provider, ev.PaymentRef).Scan(&publicID); errors.Is(err, sql.ErrNoRows) {
			return ErrMismatch
		} else if err != nil {
			return err
		}
		ord, err := q.GetOrderByPublicID(ctx, publicID)
		if err != nil {
			return err
		}
		if err := ensureLegacyReceipt(ctx, tx, ord); err != nil {
			return err
		}
		p, err = getReceipt(ctx, tx, provider, ev.PaymentRef)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if !strings.EqualFold(p.Currency, ev.Currency) {
		return ErrMismatch
	}
	if provider == "stripe" {
		if ev.AmountCents != p.Amount {
			return ErrMismatch
		}
		// charge.refunded carries a cumulative amount. MAX prevents old events
		// delivered after a newer one from undoing a refund.
		if _, err := tx.ExecContext(ctx, `UPDATE payment_receipt SET refunded_cents=MAX(refunded_cents,?),review_reason=CASE WHEN review_reason='legacy_refund' THEN '' ELSE review_reason END,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?`, ev.RefundedCents, p.ID); err != nil {
			return err
		}
	} else {
		if ev.RefundedCents > p.Amount {
			return ErrMismatch
		}
		var receiptID, amount int64
		err := tx.QueryRowContext(ctx, `SELECT receipt_id,amount_cents FROM payment_refund WHERE provider=? AND refund_ref=?`, provider, ev.RefundRef).Scan(&receiptID, &amount)
		if err == nil && (receiptID != p.ID || amount != ev.AmountCents) {
			return ErrMismatch
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO payment_refund(receipt_id,provider,refund_ref,amount_cents) VALUES(?,?,?,?) ON CONFLICT(provider,refund_ref) DO NOTHING`, p.ID, provider, ev.RefundRef, ev.AmountCents); err != nil {
			return err
		}
		// The authoritative cumulative amount includes refunds whose webhook has
		// not arrived yet. Adding an unseen individual refund would double count.
		if _, err := tx.ExecContext(ctx, `UPDATE payment_receipt SET refunded_cents=MAX(refunded_cents,?),updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?`, ev.RefundedCents, p.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO refund(order_id,provider,provider_refund_id,amount_cents) VALUES(?,?,?,?) ON CONFLICT(provider,provider_refund_id) DO NOTHING`, p.OrderID, provider, ev.RefundRef, ev.AmountCents); err != nil {
			return err
		}
	}
	if err := reconcileOrderFinancialStatus(ctx, tx, p.OrderID); err != nil {
		return err
	}
	return tx.Commit()
}

// Financial status can change without erasing shipment history. Full refund does
// not automatically restock physical goods; that belongs to the returns workflow.
func reconcileOrderFinancialStatus(ctx context.Context, tx *sql.Tx, orderID int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE "order" SET status=CASE
        WHEN EXISTS(SELECT 1 FROM refund_reconciliation r JOIN payment_receipt p ON p.id=r.receipt_id WHERE p.order_id="order".id AND r.state='open') THEN 'payment_review'
        WHEN EXISTS(SELECT 1 FROM payment_receipt p WHERE p.order_id="order".id AND p.review_reason<>'' AND p.refunded_cents<p.amount_cents) THEN 'payment_review'
        WHEN EXISTS(SELECT 1 FROM payment_receipt p WHERE p.order_id="order".id AND p.provider="order".payment_provider AND p.payment_ref="order".payment_ref AND p.refunded_cents=p.amount_cents) THEN 'refunded'
        WHEN EXISTS(SELECT 1 FROM shipment WHERE shipment.order_id="order".id) THEN 'fulfilled'
        ELSE 'paid' END,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?`, orderID)
	return err
}
