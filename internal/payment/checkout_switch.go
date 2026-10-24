// 支付通道切换 / Checkout Provider Switching
// 功能：订单级持久租约串行支付会话与扣款，先关闭旧通道再开启新通道，保留原会话历史
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 16:58:12
package payment

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kartwo/kartwo/internal/order"
)

var (
	ErrPaymentProcessing = errors.New("payment: 原付款已完成或正在确认，请勿切换支付方式")
	ErrCheckoutReplaced  = errors.New("payment: 付款会话已停用，请返回订单使用当前支付方式")
)

// Longer than the bounded sequence of recover-create, inspect, expire and create.
// The operation itself has a shorter deadline, so an old process cannot submit
// another money request after its persisted lease is eligible for takeover.
const checkoutLockLease = 4 * time.Minute
const checkoutOperationTimeout = 3 * time.Minute

type checkoutLock struct {
	OrderID int64
	Token   string
}

func (s *Service) lockCheckout(ctx context.Context, publicID string) (checkoutLock, error) {
	var lock checkoutLock
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return lock, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = tx.QueryRowContext(ctx, `SELECT id FROM "order" WHERE public_id=?`, publicID).Scan(&lock.OrderID); err != nil {
		return lock, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO checkout_lock(order_id) VALUES(?) ON CONFLICT(order_id) DO NOTHING`, lock.OrderID); err != nil {
		return lock, err
	}
	lock.Token = uuid.Must(uuid.NewV7()).String()
	result, err := tx.ExecContext(ctx, `UPDATE checkout_lock SET lease_token=?,lease_until=?,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE order_id=? AND lease_until<=?`, lock.Token, time.Now().Add(checkoutLockLease).Unix(), lock.OrderID, time.Now().Unix())
	if err != nil {
		return lock, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return lock, err
	}
	if n != 1 {
		return lock, ErrPaymentInProgress
	}
	return lock, tx.Commit()
}
func (s *Service) unlockCheckout(ctx context.Context, lock checkoutLock) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, _ = s.db.ExecContext(ctx, `UPDATE checkout_lock SET lease_token='',lease_until=0,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE order_id=? AND lease_token=?`, lock.OrderID, lock.Token)
}
func (s *Service) checkoutPending(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	expired, err := order.ExpirePendingTx(ctx, tx, id, time.Now())
	if err != nil {
		return err
	}
	var state string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM "order" WHERE id=?`, id).Scan(&state); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if expired || state != "pending" {
		return ErrOrderExpired
	}
	return nil
}

func (s *Service) StartCheckout(ctx context.Context, provider string, input OrderForPayment) (string, error) {
	if s.providerByName(provider) == nil || !s.providerReady(ctx, provider) {
		return "", ErrLocked
	}
	ctx, cancel := context.WithTimeout(ctx, checkoutOperationTimeout)
	defer cancel()
	lock, err := s.lockCheckout(ctx, input.PublicID)
	if err != nil {
		return "", err
	}
	defer s.unlockCheckout(ctx, lock)
	if err = s.checkoutPending(ctx, lock.OrderID); err != nil {
		return "", err
	}
	others, err := s.otherCheckouts(ctx, lock.OrderID, provider)
	if err != nil {
		return "", err
	}
	for _, a := range others {
		if err = s.retireCheckout(ctx, a); err != nil {
			return "", err
		}
	}
	return s.startCheckoutSession(ctx, provider, input)
}

type switchAttempt struct {
	ID, Provider, Fingerprint, State, Reference, CaptureState string
	OrderID, LeaseUntil                                       int64
	Request                                                   OrderForPayment
}

func (s *Service) otherCheckouts(ctx context.Context, orderID int64, provider string) ([]switchAttempt, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT public_id,order_id,provider,config_fingerprint,request_json,state,provider_reference,lease_until,COALESCE((SELECT state FROM checkout_capture c WHERE c.attempt_id=payment_attempt.public_id),'') FROM payment_attempt WHERE order_id=? AND provider<>? AND state<>'retired' ORDER BY id`, orderID, provider)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []switchAttempt
	for rows.Next() {
		var a switchAttempt
		var raw string
		if err = rows.Scan(&a.ID, &a.OrderID, &a.Provider, &a.Fingerprint, &raw, &a.State, &a.Reference, &a.LeaseUntil, &a.CaptureState); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &a.Request); err != nil {
			return nil, ErrMismatch
		}
		if a.Request.IdempotencyKey != a.ID {
			return nil, ErrMismatch
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Service) retireCheckout(ctx context.Context, a switchAttempt) error {
	fingerprint, err := s.paymentFingerprint(a.Provider)
	if err != nil {
		return err
	}
	if fingerprint != a.Fingerprint {
		return ErrPaymentConfigChanged
	}
	if a.LeaseUntil > time.Now().Unix() {
		return ErrPaymentInProgress
	}
	// An interrupted create may already have produced a live hosted checkout.
	// Recover the same frozen request/key before attempting to close it.
	if a.Reference == "" {
		if _, err = s.startCheckoutSession(ctx, a.Provider, a.Request); err != nil {
			return err
		}
		if err = s.db.QueryRowContext(ctx, `SELECT provider_reference FROM payment_attempt WHERE public_id=?`, a.ID).Scan(&a.Reference); err != nil {
			return err
		}
	}
	switch a.Provider {
	case "stripe":
		state, closeErr := s.stripe.expireCheckout(ctx, a.Reference, a.Request)
		if state.PaymentStatus == "paid" && state.PaymentRef != "" {
			if err = s.recordPayment(ctx, "stripe", WebhookEvent{OrderRef: a.Request.PublicID, PaymentRef: state.PaymentRef, AmountCents: state.Amount, Currency: state.Currency}, false); err != nil {
				return err
			}
		}
		if closeErr != nil {
			return closeErr
		}
	case "paypal":
		state, err := s.paypal.inspectCheckout(ctx, a.Reference, a.Request)
		if err != nil {
			return err
		}
		declined := len(state.Captures) == 1 && (state.Captures[0].Status == "DECLINED" || state.Captures[0].Status == "FAILED")
		if declined {
			if _, err = s.db.ExecContext(ctx, `UPDATE checkout_capture SET state='failed',updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE attempt_id=?`, a.ID); err != nil {
				return err
			}
		} else {
			if len(state.Captures) > 0 {
				if err = s.recordPayPalInspection(ctx, state); err != nil {
					return err
				}
				return ErrPaymentProcessing
			}
			// Approval alone does not collect money. Retired return URLs are
			// denied before capture; an unknown POST result cannot be retired.
			if a.CaptureState == "pending" {
				return ErrPaymentProcessing
			}
			switch state.Status {
			case "CREATED", "SAVED", "PAYER_ACTION_REQUIRED", "APPROVED", "VOIDED":
			default:
				return ErrPaymentProcessing
			}
		}
	default:
		return ErrMismatch
	}
	if err = s.checkoutPending(ctx, a.OrderID); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE payment_attempt SET state='retired',lease_token='',lease_until=0,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id=? AND state<>'retired'`, a.ID)
	return err
}

// Capture recovery can use an already completed provider record even if the
// storefront order expired. Such a receipt follows the existing late-payment
// review policy; it never silently re-reserves stock.
func (s *Service) recordPayPalInspection(ctx context.Context, p paypalCheckout) error {
	if len(p.Captures) != 1 {
		return ErrPaymentProcessing
	}
	c := p.Captures[0]
	if c.Status != "COMPLETED" {
		return ErrPaymentProcessing
	}
	amount, err := decimalToCents(c.Amount.Value)
	if err != nil {
		return err
	}
	return s.recordPayment(ctx, "paypal", WebhookEvent{OrderRef: p.OrderRef, PaymentRef: c.ID, AmountCents: amount, Currency: c.Amount.Currency}, false)
}
