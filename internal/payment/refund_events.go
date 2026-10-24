// 退款状态通知 / Refund Status Events
// 功能：验签后查询平台当前退款、恢复未知编号的原请求，并持久标记记账状态矛盾
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 23:55:00
package payment

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
)

// The signed event is a notification, not an ordered financial snapshot. Always
// retrieve the current refund so a delayed "pending" event cannot undo success.
func (s *Service) handleRefundStatus(ctx context.Context, provider string, ev WebhookEvent) error {
	if ev.ID == "" || ev.RefundRef == "" || len(ev.RefundRef) > 255 {
		return ErrMismatch
	}
	var seen bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM webhook_event WHERE provider=? AND event_id=?)`, provider, ev.ID).Scan(&seen); err != nil || seen {
		return err
	}
	a, err := scanRefund(s.db.QueryRowContext(ctx, `SELECT `+refundColumns+` FROM refund_request WHERE provider=? AND (gateway_ref=? OR public_id=?)`, provider, ev.RefundRef, ev.RefundRequestID))
	known := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if known {
		fingerprint, err := s.paymentFingerprint(provider)
		if err != nil {
			return err
		}
		if a.Fingerprint != refundRequestVersion+":"+fingerprint {
			return ErrPaymentConfigChanged
		}
	}
	r, err := s.refundGateway(provider).lookupRefund(ctx, ev.RefundRef)
	if err != nil {
		return err
	}
	if r.ID != ev.RefundRef || ev.PaymentRef != "" && r.PaymentRef != ev.PaymentRef || ev.AmountCents > 0 && r.Amount != ev.AmountCents || ev.Currency != "" && !strings.EqualFold(r.Currency, ev.Currency) {
		return ErrMismatch
	}
	// Some event payloads omit metadata. The authenticated resource lookup can
	// still identify a request whose original POST response was lost.
	if !known && r.RequestID != "" {
		a, err = scanRefund(s.db.QueryRowContext(ctx, `SELECT `+refundColumns+` FROM refund_request WHERE provider=? AND public_id=?`, provider, r.RequestID))
		known = err == nil
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if known {
			fingerprint, err := s.paymentFingerprint(provider)
			if err != nil {
				return err
			}
			if a.Fingerprint != refundRequestVersion+":"+fingerprint {
				return ErrPaymentConfigChanged
			}
		}
	}
	if known {
		if r.RequestID != a.ID || r.PaymentRef != a.PaymentRef || r.Amount != a.AmountCents || !strings.EqualFold(r.Currency, a.Currency) || a.GatewayRef != "" && a.GatewayRef != r.ID {
			return ErrMismatch
		}
		if a.State != "succeeded" && a.State != "failed" {
			var orderID string
			if err := s.db.QueryRowContext(ctx, `SELECT public_id FROM "order" WHERE id=?`, a.OrderID).Scan(&orderID); err != nil {
				return err
			}
			in := RefundInput{ClientKey: a.ClientKey, Provider: a.Provider, PaymentRef: a.PaymentRef, AmountCents: a.AmountCents, ExpectedRefundedCents: a.InitialRefunded}
			claimed, err := s.claimRefund(ctx, orderID, in, r.ID)
			if err != nil {
				return err // An in-flight POST keeps its lease; provider retries the event.
			}
			if _, err := s.saveRefundResult(ctx, claimed, r, nil); err != nil {
				return err
			}
		}
	}
	return s.recordRefundObservation(ctx, provider, ev, r, known, a.ID)
}

func (s *Service) recordRefundObservation(ctx context.Context, provider string, ev WebhookEvent, r gatewayRefund, known bool, requestID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	added, err := recordEvent(ctx, s.q.WithTx(tx), provider, ev)
	if err != nil || !added {
		return err
	}
	p, err := getReceipt(ctx, tx, provider, r.PaymentRef)
	if errors.Is(err, sql.ErrNoRows) {
		var orderID string
		if err := tx.QueryRowContext(ctx, `SELECT public_id FROM "order" WHERE payment_provider=? AND payment_ref=?`, provider, r.PaymentRef).Scan(&orderID); err != nil {
			return ErrMismatch
		}
		ord, loadErr := s.q.WithTx(tx).GetOrderByPublicID(ctx, orderID)
		if loadErr != nil {
			return loadErr
		}
		if err := ensureLegacyReceipt(ctx, tx, ord); err != nil {
			return err
		}
		p, err = getReceipt(ctx, tx, provider, r.PaymentRef)
	}
	if err != nil {
		return err // Refund before receipt: roll back event deduplication for replay.
	}
	if r.Amount <= 0 || r.Amount > p.Amount || !strings.EqualFold(r.Currency, p.Currency) {
		return ErrMismatch
	}
	var recordedReceipt, recordedAmount int64
	err = tx.QueryRowContext(ctx, `SELECT receipt_id,amount_cents FROM payment_refund WHERE provider=? AND refund_ref=?`, provider, r.ID).Scan(&recordedReceipt, &recordedAmount)
	recorded := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if recorded && (recordedReceipt != p.ID || recordedAmount != r.Amount) {
		return ErrMismatch
	}
	needsReview := r.State != "succeeded" && (recorded || !known && p.Refunded > 0)
	if known {
		current, err := scanRefund(tx.QueryRowContext(ctx, `SELECT `+refundColumns+` FROM refund_request WHERE public_id=?`, requestID))
		if err != nil {
			return err
		}
		needsReview = needsReview || (current.State == "succeeded" || current.State == "failed") && current.State != r.State
	} else if r.State == "pending" {
		needsReview = true // External pending refund must also block new refunds/fulfillment.
	}
	if needsReview {
		_, err = tx.ExecContext(ctx, `INSERT INTO refund_reconciliation(public_id,receipt_id,provider,refund_ref,amount_cents,observed_state)
            VALUES(?,?,?,?,?,?) ON CONFLICT(provider,refund_ref) DO UPDATE SET observed_state=excluded.observed_state,state='open',updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')`, uuid.Must(uuid.NewV7()).String(), p.ID, provider, r.ID, r.Amount, r.State)
		if err != nil {
			return err
		}
	}
	// External successful refunds still contribute to the ledger. An open issue
	// keeps the whole payment under review until a complete reconciliation, even
	// if a later aggregate amount looks plausible. Never guess a reversal delta.
	if !known && r.State == "succeeded" {
		if r.TotalRefunded < r.Amount || r.TotalRefunded > p.Amount {
			return ErrMismatch
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO payment_refund(receipt_id,provider,refund_ref,amount_cents) VALUES(?,?,?,?) ON CONFLICT(provider,refund_ref) DO NOTHING`, p.ID, provider, r.ID, r.Amount); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO refund(order_id,provider,provider_refund_id,amount_cents) VALUES(?,?,?,?) ON CONFLICT(provider,provider_refund_id) DO NOTHING`, p.OrderID, provider, r.ID, r.Amount); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE payment_receipt SET refunded_cents=MAX(refunded_cents,?),updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?`, r.TotalRefunded, p.ID); err != nil {
			return err
		}
	}
	if err := reconcileOrderFinancialStatus(ctx, tx, p.OrderID); err != nil {
		return err
	}
	return tx.Commit()
}
