// 退款请求编排 / Durable Refund Orchestration
// 功能：保存指定付款和金额、事务认领租约、同键恢复并核对网关结果后记账
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 22:12:00
package payment

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrRefundBusy     = errors.New("payment: 这笔付款有退款正在处理，请在退款请求中继续核对")
	ErrRefundConflict = errors.New("payment: 退款参数或余额已变化，请刷新后重试")
)

type RefundInput struct {
	ClientKey             string `json:"request_key"`
	Provider              string `json:"provider"`
	PaymentRef            string `json:"payment_ref"`
	AmountCents           int64  `json:"amount_cents"`
	ExpectedRefundedCents int64  `json:"expected_refunded_cents"`
}

type RefundResult struct {
	ID          string `json:"public_id"`
	State       string `json:"state"`
	Provider    string `json:"provider"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
	Reason      string `json:"reason"`
	Created     bool   `json:"-"`
	Completed   bool   `json:"-"`
}

type refundAttempt struct {
	RefundResult
	OrderID, ReceiptID                                    int64
	ClientKey, PaymentRef, Fingerprint, GatewayRef, Lease string
	InitialRefunded, FirstSent, LeaseUntil                int64
}

const refundColumns = `public_id,order_id,receipt_id,client_key,provider,payment_ref,amount_cents,currency,initial_refunded_cents,config_fingerprint,state,gateway_ref,reason,first_sent_at,lease_token,lease_until`

type rowScanner interface{ Scan(...any) error }

func scanRefund(row rowScanner) (refundAttempt, error) {
	var a refundAttempt
	err := row.Scan(&a.ID, &a.OrderID, &a.ReceiptID, &a.ClientKey, &a.Provider, &a.PaymentRef, &a.AmountCents, &a.Currency, &a.InitialRefunded, &a.Fingerprint, &a.State, &a.GatewayRef, &a.Reason, &a.FirstSent, &a.Lease, &a.LeaseUntil)
	return a, err
}

// RequestRefund accepts a browser-generated deduplication key and a displayed
// balance snapshot. A second click cannot silently create another partial refund.
func (s *Service) RequestRefund(ctx context.Context, orderID string, in RefundInput) (RefundResult, error) {
	a, err := s.claimRefund(ctx, orderID, in, "")
	if err != nil {
		return a.RefundResult, err
	}
	return s.executeRefund(ctx, a)
}

// RetryRefund always loads the original immutable request. An optional gateway
// reference is verified with a read-only provider lookup, never blindly attached.
func (s *Service) RetryRefund(ctx context.Context, orderID, id, gatewayRef string) (RefundResult, error) {
	if len(gatewayRef) > 255 {
		return RefundResult{}, ErrRefundConflict
	}
	a, err := scanRefund(s.db.QueryRowContext(ctx, `SELECT `+refundColumns+` FROM refund_request WHERE public_id=? AND order_id=(SELECT id FROM "order" WHERE public_id=?)`, id, orderID))
	if err != nil {
		return RefundResult{}, err
	}
	in := RefundInput{ClientKey: a.ClientKey, Provider: a.Provider, PaymentRef: a.PaymentRef, AmountCents: a.AmountCents, ExpectedRefundedCents: a.InitialRefunded}
	a, err = s.claimRefund(ctx, orderID, in, gatewayRef)
	if err != nil {
		return a.RefundResult, err
	}
	return s.executeRefund(ctx, a)
}

func validRefundKey(key string) bool {
	if len(key) < 16 || len(key) > 128 {
		return false
	}
	for _, c := range key {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

func (s *Service) claimRefund(ctx context.Context, publicID string, in RefundInput, lookupID string) (refundAttempt, error) {
	var a refundAttempt
	if !validRefundKey(in.ClientKey) || in.AmountCents <= 0 || in.ExpectedRefundedCents < 0 || len(in.PaymentRef) > 255 || s.refundGateway(in.Provider) == nil {
		return a, ErrRefundConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return a, err
	}
	defer func() { _ = tx.Rollback() }()
	ord, err := s.q.WithTx(tx).GetOrderByPublicID(ctx, publicID)
	if err != nil {
		return a, err
	}
	a, err = scanRefund(tx.QueryRowContext(ctx, `SELECT `+refundColumns+` FROM refund_request WHERE order_id=? AND client_key=?`, ord.ID, in.ClientKey))
	if errors.Is(err, sql.ErrNoRows) {
		if err := ensureLegacyReceipt(ctx, tx, ord); err != nil {
			return a, err
		}
		p, err := getReceipt(ctx, tx, in.Provider, in.PaymentRef)
		if err != nil {
			return a, err
		}
		if p.OrderID != ord.ID {
			return a, ErrMismatch
		}
		if p.Review == "legacy_refund" || p.Refunded != in.ExpectedRefundedCents || in.AmountCents > p.Amount-p.Refunded {
			return a, ErrRefundConflict
		}
		var needsReconciliation bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM refund_reconciliation WHERE receipt_id=? AND state='open')`, p.ID).Scan(&needsReconciliation); err != nil {
			return a, err
		}
		if needsReconciliation {
			return a, ErrRefundConflict
		}
		var active bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM refund_request WHERE receipt_id=? AND state IN ('prepared','pending','review'))`, p.ID).Scan(&active); err != nil {
			return a, err
		}
		if active {
			return a, ErrRefundBusy
		}
		fingerprint, err := s.paymentFingerprint(in.Provider)
		if err != nil {
			return a, err
		}
		a = refundAttempt{RefundResult: RefundResult{ID: uuid.Must(uuid.NewV7()).String(), State: "prepared", Provider: in.Provider, AmountCents: in.AmountCents, Currency: p.Currency, Created: true}, OrderID: ord.ID, ReceiptID: p.ID, ClientKey: in.ClientKey, PaymentRef: in.PaymentRef, InitialRefunded: p.Refunded, Fingerprint: refundRequestVersion + ":" + fingerprint}
		if _, err := tx.ExecContext(ctx, `INSERT INTO refund_request(public_id,order_id,receipt_id,client_key,provider,payment_ref,amount_cents,currency,initial_refunded_cents,config_fingerprint) VALUES(?,?,?,?,?,?,?,?,?,?)`, a.ID, a.OrderID, a.ReceiptID, a.ClientKey, a.Provider, a.PaymentRef, a.AmountCents, a.Currency, a.InitialRefunded, a.Fingerprint); err != nil {
			return a, err
		}
	} else if err != nil {
		return a, err
	}
	if a.Provider != in.Provider || a.PaymentRef != in.PaymentRef || a.AmountCents != in.AmountCents || a.InitialRefunded != in.ExpectedRefundedCents {
		return a, ErrRefundConflict
	}
	var changedResult bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM refund_reconciliation WHERE provider=? AND refund_ref=? AND state='open')`, a.Provider, a.GatewayRef).Scan(&changedResult); err != nil {
		return a, err
	}
	if changedResult {
		a.State, a.Reason = "review", "refund_status_changed"
		return a, tx.Commit()
	}
	if a.State == "succeeded" || a.State == "failed" {
		return a, tx.Commit()
	}
	fingerprint, err := s.paymentFingerprint(a.Provider)
	if err != nil {
		return a, err
	}
	if a.Fingerprint != refundRequestVersion+":"+fingerprint {
		return a, ErrPaymentConfigChanged
	}
	now := time.Now()
	if a.LeaseUntil > now.Unix() {
		return a, ErrRefundBusy
	}
	if lookupID != "" {
		if a.GatewayRef != "" && a.GatewayRef != lookupID {
			return a, ErrRefundConflict
		}
		a.GatewayRef = lookupID
	}
	// Bound unknown-result re-submission; provider key retention is finite. Once
	// this window passes, only a verified refund-ID lookup may resolve it.
	if a.GatewayRef == "" && a.FirstSent > 0 && now.Unix()-a.FirstSent > int64((15*time.Minute)/time.Second) {
		a.State, a.Reason = "review", "unknown_result"
		if _, err := tx.ExecContext(ctx, `UPDATE refund_request SET state='review',reason='unknown_result' WHERE public_id=?`, a.ID); err != nil {
			return a, err
		}
		return a, tx.Commit()
	}
	a.Lease = uuid.Must(uuid.NewV7()).String()
	if a.FirstSent == 0 {
		a.FirstSent = now.Unix()
	}
	_, err = tx.ExecContext(ctx, `UPDATE refund_request SET state='pending',first_sent_at=?,lease_token=?,lease_until=?,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id=?`, a.FirstSent, a.Lease, now.Add(attemptLease).Unix(), a.ID)
	if err != nil {
		return a, err
	}
	a.State = "pending"
	return a, tx.Commit()
}

func (s *Service) executeRefund(ctx context.Context, a refundAttempt) (RefundResult, error) {
	if a.State == "succeeded" || a.State == "failed" || a.State == "review" {
		return s.checkRefundAttempt(ctx, a, a.RefundResult)
	}
	gateway := s.refundGateway(a.Provider)
	var result gatewayRefund
	var err error
	if a.GatewayRef != "" {
		result, err = gateway.lookupRefund(ctx, a.GatewayRef)
	} else {
		result, err = gateway.submitRefund(ctx, a)
	}
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	saved, saveErr := s.saveRefundResult(saveCtx, a, result, err)
	if saveErr != nil {
		return saved, saveErr
	}
	return s.checkRefundAttempt(ctx, a, saved)
}

func (s *Service) saveRefundResult(ctx context.Context, a refundAttempt, r gatewayRefund, callErr error) (RefundResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return a.RefundResult, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := scanRefund(tx.QueryRowContext(ctx, `SELECT `+refundColumns+` FROM refund_request WHERE public_id=?`, a.ID))
	if err != nil {
		return a.RefundResult, err
	}
	if current.State == "succeeded" || current.State == "failed" {
		return current.RefundResult, tx.Commit()
	}
	if current.Lease != a.Lease {
		return a.RefundResult, ErrRefundBusy
	}
	valid := r.ID != "" && len(r.ID) <= 255 && r.PaymentRef == a.PaymentRef && r.Amount == a.AmountCents && strings.EqualFold(r.Currency, a.Currency) && r.RequestID == a.ID
	if valid && a.GatewayRef != "" && r.ID != a.GatewayRef {
		valid = false
	}
	state, reason, ref := "pending", "gateway_unconfirmed", current.GatewayRef
	if valid {
		ref = r.ID
	}
	switch {
	case errors.Is(callErr, errRefundRejected) && current.GatewayRef == "":
		state, reason = "failed", "rejected"
	case errors.Is(callErr, ErrMismatch) || callErr == nil && !valid:
		state, reason = "review", "result_mismatch"
	case callErr == nil && r.State == "failed":
		state, reason = "failed", "gateway_failed"
	case callErr == nil && r.State == "succeeded":
		p, err := getReceipt(ctx, tx, a.Provider, a.PaymentRef)
		if err != nil {
			return a.RefundResult, err
		}
		if p.OrderID != a.OrderID || r.TotalRefunded < r.Amount || r.TotalRefunded > p.Amount {
			state, reason = "review", "result_mismatch"
			break
		}
		var receiptID, amount int64
		err = tx.QueryRowContext(ctx, `SELECT receipt_id,amount_cents FROM payment_refund WHERE provider=? AND refund_ref=?`, a.Provider, r.ID).Scan(&receiptID, &amount)
		if err == nil && (receiptID != p.ID || amount != r.Amount) {
			state, reason = "review", "result_mismatch"
			break
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return a.RefundResult, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO payment_refund(receipt_id,provider,refund_ref,amount_cents) VALUES(?,?,?,?) ON CONFLICT(provider,refund_ref) DO NOTHING`, p.ID, a.Provider, r.ID, r.Amount); err != nil {
			return a.RefundResult, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO refund(order_id,provider,provider_refund_id,amount_cents) VALUES(?,?,?,?) ON CONFLICT(provider,provider_refund_id) DO NOTHING`, a.OrderID, a.Provider, r.ID, r.Amount); err != nil {
			return a.RefundResult, err
		}
		tracked, err := receiptUsesRefundChecks(ctx, tx, p.ID)
		if err != nil {
			return a.RefundResult, err
		}
		if tracked {
			// Publish no cumulative amount after a correction. Hold this receipt
			// until a complete inventory confirms the new refund and net balance.
			_, err = tx.ExecContext(ctx, `INSERT INTO refund_reconciliation(public_id,receipt_id,provider,refund_ref,amount_cents,observed_state) VALUES(?,?,?,?,?,'succeeded') ON CONFLICT(provider,refund_ref) DO UPDATE SET state='open',observed_state='succeeded'`, uuid.Must(uuid.NewV7()).String(), p.ID, a.Provider, r.ID, r.Amount)
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE payment_receipt SET refunded_cents=MAX(refunded_cents,?),updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?`, r.TotalRefunded, p.ID)
		}
		if err != nil {
			return a.RefundResult, err
		}
		if err := reconcileOrderFinancialStatus(ctx, tx, a.OrderID); err != nil {
			return a.RefundResult, err
		}
		state, reason = "succeeded", ""
	case callErr == nil:
		reason = "gateway_pending"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE refund_request SET state=?,reason=?,gateway_ref=?,lease_token='',lease_until=0,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id=?`, state, reason, ref, a.ID); err != nil {
		return a.RefundResult, err
	}
	if err := tx.Commit(); err != nil {
		return a.RefundResult, err
	}
	a.State, a.Reason, a.Completed = state, reason, state == "succeeded"
	return a.RefundResult, nil
}

// Refund retains the original full-refund endpoint while using the same durable
// workflow. New partial refunds use RequestRefund with an explicit payment.
func (s *Service) Refund(ctx context.Context, publicID string) error {
	ord, err := s.q.GetOrderByPublicID(ctx, publicID)
	if err != nil {
		return err
	}
	if ord.Status != "paid" && ord.Status != "fulfilled" && ord.Status != "payment_review" {
		return ErrNotRefundable
	}
	var blocked bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM payment_receipt WHERE order_id=? AND (refunded_cents>0 OR review_reason='duplicate_payment'))`, ord.ID).Scan(&blocked); err != nil {
		return err
	}
	if blocked {
		return ErrNotRefundable
	}
	key := uuid.NewSHA1(uuid.NameSpaceOID, []byte("legacy-refund:"+publicID)).String()
	r, err := s.RequestRefund(ctx, publicID, RefundInput{ClientKey: key, Provider: ord.PaymentProvider, PaymentRef: ord.PaymentRef, AmountCents: ord.TotalCents})
	if err != nil {
		return err
	}
	if r.State == "succeeded" {
		return nil
	}
	return ErrRefundUnconfirmed
}

// A receipt that has been corrected stays on full-inventory accounting for all
// later refunds, including worker retries and requests recovered after a crash.
func (s *Service) checkRefundAttempt(ctx context.Context, a refundAttempt, result RefundResult) (RefundResult, error) {
	localCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var tracked bool
	if err := s.db.QueryRowContext(localCtx, `SELECT EXISTS(SELECT 1 FROM refund_check WHERE receipt_id=?)`, a.ReceiptID).Scan(&tracked); err != nil {
		return result, err
	}
	if !tracked {
		return result, nil
	}
	if ctx.Err() != nil {
		result.State = "review"
		result.Reason = "refund_status_changed"
		result.Completed = false
		return result, nil
	}
	var orderID string
	if err := s.db.QueryRowContext(ctx, `SELECT public_id FROM "order" WHERE id=?`, a.OrderID).Scan(&orderID); err != nil {
		return result, err
	}
	if _, err := s.ReconcileRefunds(ctx, orderID, a.Provider, a.PaymentRef); err != nil {
		return result, err
	}
	current, err := scanRefund(s.db.QueryRowContext(ctx, `SELECT `+refundColumns+` FROM refund_request WHERE public_id=?`, a.ID))
	current.Created = result.Created
	current.Completed = result.Completed && current.State == "succeeded"
	return current.RefundResult, err
}
