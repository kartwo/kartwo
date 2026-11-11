// 退款核对与校正 / Refund Reconciliation
// 功能：完整平台快照校正退款金额、恢复历史请求并保留审计证据，拒绝并发覆盖和不完整记录
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 16:43:04
package payment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type RefundCheckResult struct {
	ID            string `json:"public_id"`
	RefundedCents int64  `json:"refunded_cents"`
	Pending       bool   `json:"pending"`
}

// ReconcileRefunds only reads the provider. It neither submits another refund nor
// guesses that an absent/expired platform record means a failed transaction.
func (s *Service) ReconcileRefunds(ctx context.Context, orderID, provider, ref string) (RefundCheckResult, error) {
	return s.reconcileRefunds(ctx, orderID, provider, ref, "")
}

func (s *Service) reconcileRefunds(ctx context.Context, orderID, provider, ref, expectedRefund string) (RefundCheckResult, error) {
	if s.refundGateway(provider) == nil || !validRefundReference(ref) {
		return RefundCheckResult{}, ErrMismatch
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	fingerprint, err := s.paymentFingerprint(provider)
	if err != nil {
		return RefundCheckResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RefundCheckResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	ord, err := s.q.WithTx(tx).GetOrderByPublicID(ctx, orderID)
	if err != nil {
		return RefundCheckResult{}, err
	}
	if err = ensureLegacyReceipt(ctx, tx, ord); err != nil {
		return RefundCheckResult{}, err
	}
	p, err := getReceipt(ctx, tx, provider, ref)
	if err != nil {
		return RefundCheckResult{}, err
	}
	if p.OrderID != ord.ID {
		return RefundCheckResult{}, ErrMismatch
	}
	requests, err := refundCheckRequests(ctx, tx, p.ID, fingerprint)
	if err != nil {
		return RefundCheckResult{}, err
	}
	before, err := refundLocalFingerprint(ctx, tx, p)
	if err != nil {
		return RefundCheckResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return RefundCheckResult{}, err
	}
	// No database connection is held during network calls. Repeat the bounded
	// inventory to detect new refunds or status changes during pagination.
	first, err := s.refundInventory(ctx, provider, ref, p.Currency, p.Amount)
	if err != nil {
		return RefundCheckResult{}, err
	}
	second, err := s.refundInventory(ctx, provider, ref, p.Currency, p.Amount)
	if err != nil {
		return RefundCheckResult{}, err
	}
	raw, _ := json.Marshal(first)
	current, _ := json.Marshal(second)
	if !bytes.Equal(raw, current) {
		return RefundCheckResult{}, ErrRefundBusy
	}
	if expectedRefund != "" {
		found := false
		for _, r := range second.Refunds {
			found = found || r.ID == expectedRefund
		}
		if !found {
			return RefundCheckResult{}, ErrRefundUnconfirmed
		}
	}
	afterConfig, err := s.paymentFingerprint(provider)
	if err != nil {
		return RefundCheckResult{}, err
	}
	if fingerprint != afterConfig {
		return RefundCheckResult{}, ErrPaymentConfigChanged
	}
	tx, err = s.db.BeginTx(ctx, nil)
	if err != nil {
		return RefundCheckResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	after, err := refundLocalFingerprint(ctx, tx, p)
	if err != nil {
		return RefundCheckResult{}, err
	}
	if before != after {
		return RefundCheckResult{}, ErrRefundBusy
	}
	if err = validateRefundCheckHistory(ctx, tx, p, provider, second, requests); err != nil {
		return RefundCheckResult{}, err
	}
	result := RefundCheckResult{ID: uuid.Must(uuid.NewV7()).String(), RefundedCents: second.Total, Pending: second.Pending}
	if err = applyRefundCheck(ctx, tx, p, provider, second, requests); err != nil {
		return RefundCheckResult{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO refund_check(public_id,receipt_id,before_cents,after_cents,pending,refunds_json) VALUES(?,?,?,?,?,?)`, result.ID, p.ID, p.Refunded, second.Total, second.Pending, string(current)); err != nil {
		return RefundCheckResult{}, err
	}
	if err = reconcileOrderFinancialStatus(ctx, tx, p.OrderID); err != nil {
		return RefundCheckResult{}, err
	}
	return result, tx.Commit()
}

func refundCheckRequests(ctx context.Context, tx *sql.Tx, id int64, fingerprint string) ([]refundAttempt, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+refundColumns+` FROM refund_request WHERE receipt_id=? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var result []refundAttempt
	for rows.Next() {
		a, err := scanRefund(rows)
		if err != nil {
			return nil, err
		}
		if a.Fingerprint != refundRequestVersion+":"+fingerprint {
			return nil, ErrPaymentConfigChanged
		}
		if a.LeaseUntil > time.Now().Unix() {
			return nil, ErrRefundBusy
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// Hash every relevant row, not timestamps: multiple writes can share the same
// millisecond. The last unique check ID also prevents two stale checks committing.
func refundLocalFingerprint(ctx context.Context, tx *sql.Tx, p receipt) ([32]byte, error) {
	var buf bytes.Buffer
	queries := []struct {
		sql string
		id  int64
	}{
		{`SELECT * FROM payment_receipt WHERE id=?`, p.ID},
		{`SELECT * FROM refund_request WHERE receipt_id=? ORDER BY id`, p.ID},
		{`SELECT * FROM payment_refund WHERE receipt_id=? ORDER BY id`, p.ID},
		{`SELECT * FROM refund_reconciliation WHERE receipt_id=? ORDER BY id`, p.ID},
		{`SELECT public_id FROM refund_check WHERE receipt_id=? ORDER BY id DESC LIMIT 1`, p.ID},
		{`SELECT * FROM refund WHERE order_id=? ORDER BY id`, p.OrderID},
	}
	for _, q := range queries {
		if err := encodeRefundRows(ctx, tx, &buf, q.sql, q.id); err != nil {
			return [32]byte{}, err
		}
	}
	return sha256.Sum256(buf.Bytes()), nil
}
func encodeRefundRows(ctx context.Context, tx *sql.Tx, buf *bytes.Buffer, query string, id int64) error {
	rows, err := tx.QueryContext(ctx, query, id)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	enc := json.NewEncoder(buf)
	if err = enc.Encode(query); err != nil {
		return err
	}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err = rows.Scan(ptrs...); err != nil {
			return err
		}
		if err = enc.Encode(vals); err != nil {
			return err
		}
	}
	return rows.Err()
}

func validateRefundCheckHistory(ctx context.Context, tx *sql.Tx, p receipt, provider string, inv refundInventory, requests []refundAttempt) error {
	byID := make(map[string]gatewayRefund)
	byRequest := make(map[string]gatewayRefund)
	for _, r := range inv.Refunds {

		var conflict bool
		err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM payment_refund WHERE provider=? AND refund_ref=? AND receipt_id<>? UNION SELECT 1 FROM refund_reconciliation WHERE provider=? AND refund_ref=? AND receipt_id<>? UNION SELECT 1 FROM refund_request WHERE provider=? AND public_id=? AND receipt_id<>?)`, provider, r.ID, p.ID, provider, r.ID, p.ID, provider, r.RequestID, p.ID).Scan(&conflict)
		if err != nil {
			return err
		}
		if conflict {
			return ErrMismatch
		}
		byID[r.ID] = r
		if r.RequestID != "" {
			if _, ok := byRequest[r.RequestID]; ok {
				return ErrMismatch
			}
			byRequest[r.RequestID] = r
		}
	}
	// Historical postings and unresolved issues cannot silently disappear from a
	// complete platform inventory, including imports from versions before receipts.
	rows, err := tx.QueryContext(ctx, `SELECT refund_ref,amount_cents FROM payment_refund WHERE receipt_id=? UNION SELECT refund_ref,amount_cents FROM refund_reconciliation WHERE receipt_id=? UNION SELECT provider_refund_id,amount_cents FROM refund WHERE order_id=? AND provider=? AND NOT EXISTS(SELECT 1 FROM payment_refund pr WHERE pr.provider=refund.provider AND pr.refund_ref=refund.provider_refund_id)`, p.ID, p.ID, p.OrderID, provider)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		var amount int64
		if err := rows.Scan(&id, &amount); err != nil {
			return err
		}
		r, ok := byID[id]
		if !ok {
			return ErrRefundUnconfirmed
		}
		if r.Amount != amount {
			return ErrMismatch
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	for _, a := range requests {
		r, ok := byRequest[a.ID]
		if !ok {
			if a.State == "failed" && a.GatewayRef == "" && a.Reason == "rejected" {
				continue
			}
			return ErrRefundUnconfirmed
		}
		if r.Amount != a.AmountCents || !strings.EqualFold(r.Currency, a.Currency) || r.PaymentRef != a.PaymentRef || a.GatewayRef != "" && a.GatewayRef != r.ID {
			return ErrMismatch
		}
	}
	return nil
}

func applyRefundCheck(ctx context.Context, tx *sql.Tx, p receipt, provider string, inv refundInventory, requests []refundAttempt) error {
	byRequest := make(map[string]refundAttempt)
	for _, a := range requests {
		byRequest[a.ID] = a
	}
	for _, r := range inv.Refunds {
		// Upsert the current observation for ALL refunds so failed historical entries
		// remain explained. Original posting rows are immutable financial history.
		state := "resolved"
		if r.State == "pending" {
			state = "open"
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO refund_reconciliation(public_id,receipt_id,provider,refund_ref,amount_cents,observed_state,state) VALUES(?,?,?,?,?,?,?) ON CONFLICT(provider,refund_ref) DO UPDATE SET observed_state=excluded.observed_state,state=excluded.state,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')`, uuid.Must(uuid.NewV7()).String(), p.ID, provider, r.ID, r.Amount, r.State, state); err != nil {
			return err
		}
		if a, ok := byRequest[r.RequestID]; ok {
			reason := ""
			if r.State == "failed" {
				reason = "gateway_failed"
			}
			if r.State == "pending" {
				reason = "gateway_pending"
			}
			if _, err := tx.ExecContext(ctx, `UPDATE refund_request SET gateway_ref=?,state=?,reason=?,lease_token='',lease_until=0,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id=?`, r.ID, r.State, reason, a.ID); err != nil {
				return err
			}
		}
		if r.State == "succeeded" {
			if _, err := tx.ExecContext(ctx, `INSERT INTO payment_refund(receipt_id,provider,refund_ref,amount_cents) VALUES(?,?,?,?) ON CONFLICT(provider,refund_ref) DO NOTHING`, p.ID, provider, r.ID, r.Amount); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO refund(order_id,provider,provider_refund_id,amount_cents) VALUES(?,?,?,?) ON CONFLICT(provider,provider_refund_id) DO NOTHING`, p.OrderID, provider, r.ID, r.Amount); err != nil {
				return err
			}
		}
	}
	_, err := tx.ExecContext(ctx, `UPDATE payment_receipt SET refunded_cents=?,review_reason=CASE WHEN review_reason='legacy_refund' THEN '' ELSE review_reason END,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?`, inv.Total, p.ID)
	return err
}

func receiptUsesRefundChecks(ctx context.Context, tx *sql.Tx, id int64) (bool, error) {
	var tracked bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM refund_check WHERE receipt_id=?)`, id).Scan(&tracked)
	return tracked, err
}

// Verified notifications for a reconciled receipt always re-read the complete
// inventory; an old cumulative charge event must never resurrect a failed refund.
func (s *Service) syncTrackedRefundEvent(ctx context.Context, provider string, ev WebhookEvent) (bool, error) {
	var orderID, currency string
	var amount int64
	err := s.db.QueryRowContext(ctx, `SELECT o.public_id,p.currency,p.amount_cents FROM payment_receipt p JOIN "order" o ON o.id=p.order_id WHERE p.provider=? AND p.payment_ref=? AND EXISTS(SELECT 1 FROM refund_check c WHERE c.receipt_id=p.id)`, provider, ev.PaymentRef).Scan(&orderID, &currency, &amount)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	if ev.Currency != "" && !strings.EqualFold(currency, ev.Currency) || ev.Type == "charge.refunded" && ev.AmountCents != amount {
		return true, ErrMismatch
	}
	var seen bool
	if err = s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM webhook_event WHERE provider=? AND event_id=?)`, provider, ev.ID).Scan(&seen); err != nil || seen {
		return true, err
	}
	if _, err = s.reconcileRefunds(ctx, orderID, provider, ev.PaymentRef, ev.RefundRef); err != nil {
		return true, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return true, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = recordEvent(ctx, s.q.WithTx(tx), provider, ev); err != nil {
		return true, err
	}
	err = tx.Commit()
	return true, err
}
