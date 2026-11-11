// 支付尝试编排 / Payment Attempt Orchestration
// 功能：原子认领持久化请求快照、恢复网关会话，防止同订单同通道重试重复建会话
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 13:16:58
package payment

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/kartwo/kartwo/internal/order"
)

var (
	ErrPaymentInProgress      = errors.New("payment: 正在创建收银台，请稍后重试")
	ErrPaymentConfigChanged   = errors.New("payment: 收款配置已变更，请联系商家核对原付款尝试")
	errCheckoutExpiryRejected = errors.New("payment: 网关拒绝尚未执行请求的过期时间")
)

// 租约覆盖 PayPal token + create 两次各 20 秒超时；进程崩溃后允许同幂等键接管。
const attemptLease = 90 * time.Second

// 改变发起收款的序列化语义时须升级此版本，禁止旧快照用同一键发送不同请求。
const checkoutRequestVersion = "1"

type checkoutAttempt struct {
	ID          string
	Request     OrderForPayment
	RedirectURL string
	LeaseToken  string
}

// StartCheckout 同订单同通道复用首次请求快照及会话。网络请求绝不持有数据库事务。
func (s *Service) startCheckoutSession(ctx context.Context, provider string, input OrderForPayment) (string, error) {
	prov := s.providerByName(provider)
	if prov == nil || !s.providerReady(ctx, provider) {
		return "", fmt.Errorf("payment: 支付方式 %q 不可用", provider)
	}
	a, err := s.claimCheckout(ctx, provider, input)
	if err != nil {
		return "", err
	}
	if a.RedirectURL != "" {
		return a.RedirectURL, nil
	}

	session, createErr := prov.CreatePayment(ctx, a.Request)
	// 崩溃恢复可能晚于 Stripe 的最小 30 分钟建会话窗口。
	// 仅明确的 expires_at 参数校验失败可更新该字段；始终保留同一幂等键，绝不猜测请求失败后换键。
	if errors.Is(createErr, errCheckoutExpiryRejected) && ctx.Err() == nil {
		a.Request.ExpiresAt = time.Now().Add(31 * time.Minute).Unix()
		if err := s.refreshCheckoutExpiry(ctx, a); err != nil {
			createErr = err
		} else {
			session, createErr = prov.CreatePayment(ctx, a.Request)
		}
	}

	// 即使客户端断开也保存已成功创建的会话；失败则释放租约，原幂等键/快照仍保留。
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if createErr != nil {
		_, _ = s.db.ExecContext(saveCtx, `UPDATE payment_attempt SET lease_token='',lease_until=0,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id=? AND lease_token=? AND state='pending'`, a.ID, a.LeaseToken)
		return "", createErr
	}
	if session.Reference == "" || !validCheckoutURL(session.RedirectURL) {
		_, _ = s.db.ExecContext(saveCtx, `UPDATE payment_attempt SET lease_token='',lease_until=0 WHERE public_id=? AND lease_token=? AND state='pending'`, a.ID, a.LeaseToken)
		return "", errors.New("payment: 网关返回的会话无效")
	}
	result, err := s.db.ExecContext(saveCtx, `UPDATE payment_attempt SET state='ready',provider_reference=?,redirect_url=?,lease_token='',lease_until=0,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id=? AND lease_token=? AND state='pending'`, session.Reference, session.RedirectURL, a.ID, a.LeaseToken)
	if err != nil {
		return "", fmt.Errorf("payment: 保存付款会话: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if n != 1 {
		return "", ErrPaymentInProgress
	}
	return session.RedirectURL, nil
}

func (s *Service) claimCheckout(ctx context.Context, provider string, input OrderForPayment) (checkoutAttempt, error) {
	var a checkoutAttempt
	fingerprint, err := s.paymentFingerprint(provider)
	if err != nil {
		return a, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return a, err
	}
	defer func() { _ = tx.Rollback() }()
	var orderID int64
	var status string
	// 金额、币种及邮箱只取数据库快照，调用方不能覆盖。
	if err := tx.QueryRowContext(ctx, `SELECT id,status,email,currency,total_cents FROM "order" WHERE public_id=?`, input.PublicID).Scan(&orderID, &status, &input.Email, &input.Currency, &input.AmountCents); err != nil {
		return a, err
	}
	now := time.Now()
	expired, err := order.ExpirePendingTx(ctx, tx, orderID, now)
	if err != nil {
		return a, err
	}
	if expired || status != "pending" {
		if err := tx.Commit(); err != nil {
			return a, err
		}
		return a, ErrOrderExpired
	}
	input.IdempotencyKey = uuid.Must(uuid.NewV7()).String()
	// Stripe 至少要求未来 30 分钟；留 1 分钟传输/首次重试余量。
	// 库存预留仍由订单的 30 分钟期限决定；迟到支付仍进入 payment_review。
	input.ExpiresAt = now.Add(31 * time.Minute).Unix()
	raw, err := json.Marshal(input)
	if err != nil {
		return a, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO payment_attempt(public_id,order_id,provider,config_fingerprint,request_json) VALUES(?,?,?,?,?) ON CONFLICT(order_id,provider) WHERE state<>'retired' DO NOTHING`, input.IdempotencyKey, orderID, provider, fingerprint, string(raw)); err != nil {
		return a, err
	}
	var storedScope, state, snapshot string
	var until int64
	if err := tx.QueryRowContext(ctx, `SELECT public_id,config_fingerprint,request_json,state,redirect_url,lease_until FROM payment_attempt WHERE order_id=? AND provider=? AND state<>'retired'`, orderID, provider).Scan(&a.ID, &storedScope, &snapshot, &state, &a.RedirectURL, &until); err != nil {
		return a, err
	}
	if storedScope != fingerprint {
		return a, ErrPaymentConfigChanged
	}
	if err := json.Unmarshal([]byte(snapshot), &a.Request); err != nil {
		return a, fmt.Errorf("payment: 读取付款请求快照: %w", err)
	}
	if a.Request.IdempotencyKey != a.ID || a.Request.PublicID != input.PublicID {
		return a, ErrMismatch
	}
	if state == "ready" {
		if !validCheckoutURL(a.RedirectURL) {
			return a, ErrMismatch
		}
		return a, tx.Commit()
	}
	if until > now.Unix() {
		return a, ErrPaymentInProgress
	}
	a.LeaseToken = uuid.Must(uuid.NewV7()).String()
	res, err := tx.ExecContext(ctx, `UPDATE payment_attempt SET lease_token=?,lease_until=?,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id=? AND state='pending' AND lease_until<=?`, a.LeaseToken, now.Add(attemptLease).Unix(), a.ID, now.Unix())
	if err != nil {
		return a, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return a, err
	}
	if n != 1 {
		return a, ErrPaymentInProgress
	}
	return a, tx.Commit()
}

func (s *Service) paymentFingerprint(provider string) (string, error) {
	var config string
	switch provider {
	case "stripe":
		secret, ok := s.keys.secretKey()
		if !ok {
			return "", ErrLocked
		}
		config = provider + "\x00" + checkoutRequestVersion + "\x00" + stripeAPIVersion + "\x00" + secret
	case "paypal":
		id, secret, mode, ok := s.keys.paypalCreds()
		if !ok {
			return "", ErrLocked
		}
		config = provider + "\x00" + checkoutRequestVersion + "\x00" + mode + "\x00" + id + "\x00" + secret
	default:
		return "", ErrLocked
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(config))), nil
}

func validCheckoutURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil
}

// refreshCheckoutExpiry 在重试网络调用前保存新的绝对时间，其他请求参数和幂等键保持不变。
func (s *Service) refreshCheckoutExpiry(ctx context.Context, a checkoutAttempt) error {
	raw, err := json.Marshal(a.Request)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE payment_attempt SET request_json=?,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id=? AND lease_token=? AND state='pending'`, string(raw), a.ID, a.LeaseToken)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrPaymentInProgress
	}
	return nil
}
