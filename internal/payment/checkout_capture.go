// PayPal 扣款保护 / PayPal Capture Guard
// 功能：核对原会话与订单，串行扣款和通道切换；未知结果只用原键恢复，不扣已停用会话
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 16:58:12
package payment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Service) CapturePayPal(ctx context.Context, id string) (string, error) {
	if !validRefundReference(id) {
		return "", ErrMismatch
	}
	ctx, cancel := context.WithTimeout(ctx, checkoutOperationTimeout)
	defer cancel()
	var publicID string
	err := s.db.QueryRowContext(ctx, `SELECT o.public_id FROM payment_attempt a JOIN "order" o ON o.id=a.order_id WHERE a.provider='paypal' AND a.provider_reference=?`, id).Scan(&publicID)
	if errors.Is(err, sql.ErrNoRows) {
		// Upgrades may still receive a return from a pre-attempts hosted order. Only
		// authenticated provider metadata may locate it; never POST an unknown token.
		p, lookupErr := s.paypal.inspectCheckout(ctx, id, OrderForPayment{})
		if lookupErr != nil {
			return "", lookupErr
		}
		publicID = p.OrderRef
	} else if err != nil {
		return "", err
	}
	lock, err := s.lockCheckout(ctx, publicID)
	if err != nil {
		return publicID, err
	}
	defer s.unlockCheckout(ctx, lock)
	ord, err := s.q.GetOrderByPublicID(ctx, publicID)
	if err != nil {
		return publicID, err
	}
	expected := OrderForPayment{PublicID: publicID, AmountCents: ord.TotalCents, Currency: ord.Currency, Email: ord.Email}
	fingerprint, err := s.paymentFingerprint("paypal")
	if err != nil {
		return publicID, err
	}
	var attemptID, state, scope string
	err = s.db.QueryRowContext(ctx, `SELECT public_id,state,config_fingerprint FROM payment_attempt WHERE provider='paypal' AND provider_reference=? AND order_id=?`, id, ord.ID).Scan(&attemptID, &state, &scope)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return publicID, err
	}
	if err == nil && scope != fingerprint {
		return publicID, ErrPaymentConfigChanged
	}
	p, err := s.paypal.inspectCheckout(ctx, id, expected)
	if err != nil {
		return publicID, err
	}
	if len(p.Captures) > 0 {
		if err = s.recordPayPalInspection(ctx, p); err != nil {
			return publicID, err
		}
		if attemptID != "" {
			_, err = s.db.ExecContext(ctx, `UPDATE checkout_capture SET state='complete',updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE attempt_id=?`, attemptID)
		}
		return publicID, err
	}
	if state == "retired" {
		return publicID, ErrCheckoutReplaced
	}
	if err = s.checkoutPending(ctx, ord.ID); err != nil {
		return publicID, err
	}
	var other bool
	if err = s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM payment_attempt WHERE order_id=? AND state<>'retired' AND (provider<>'paypal' OR provider_reference<>?))`, ord.ID, id).Scan(&other); err != nil {
		return publicID, err
	}
	if other {
		return publicID, ErrCheckoutReplaced
	}
	if p.Status != "APPROVED" {
		return publicID, ErrPaymentProcessing
	}
	if attemptID == "" {
		attemptID = uuid.Must(uuid.NewV7()).String()
		expected.IdempotencyKey = attemptID
		raw, err := json.Marshal(expected)
		if err != nil {
			return publicID, err
		}
		if _, err = s.db.ExecContext(ctx, `INSERT INTO payment_attempt(public_id,order_id,provider,config_fingerprint,request_json,state,provider_reference) VALUES(?,?,'paypal',?,?,'ready',?)`, attemptID, ord.ID, fingerprint, string(raw), id); err != nil {
			return publicID, err
		}
	}
	var captureState string
	var firstSent int64
	err = s.db.QueryRowContext(ctx, `SELECT state,first_sent_at FROM checkout_capture WHERE attempt_id=?`, attemptID).Scan(&captureState, &firstSent)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return publicID, err
	}
	if captureState == "complete" || captureState == "pending" && time.Now().Unix()-firstSent > int64((15*time.Minute)/time.Second) {
		return publicID, ErrPaymentProcessing
	}
	if _, err = s.db.ExecContext(ctx, `INSERT INTO checkout_capture(attempt_id,order_id,state,first_sent_at) VALUES(?,?,'pending',?) ON CONFLICT(attempt_id) DO UPDATE SET state='pending',updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')`, attemptID, ord.ID, time.Now().Unix()); err != nil {
		return publicID, err
	}
	res, err := s.paypal.Capture(ctx, id)
	if err != nil {
		return publicID, err
	}
	if !res.Completed {
		return publicID, ErrPaymentProcessing
	}
	if res.OrderRef != publicID || res.AmountCents != ord.TotalCents || !strings.EqualFold(res.Currency, ord.Currency) || !validRefundReference(res.CaptureID) {
		return publicID, ErrMismatch
	}
	saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer saveCancel()
	if err = s.recordPayment(saveCtx, "paypal", WebhookEvent{OrderRef: publicID, PaymentRef: res.CaptureID, AmountCents: res.AmountCents, Currency: res.Currency}, false); err != nil {
		return publicID, err
	}
	_, err = s.db.ExecContext(saveCtx, `UPDATE checkout_capture SET state='complete',updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE attempt_id=?`, attemptID)
	return publicID, err
}
