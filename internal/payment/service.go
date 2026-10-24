// 支付编排服务 / Payment Orchestration Service
// 功能：按市场选通道发起收款；处理 Webhook 双校验+回调幂等（去重 INSERT 与 pending->paid 同一事务）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-20 20:26:08
package payment

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/kartwo/kartwo/internal/settings"
	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

// Service 支付编排：发起收款 + Webhook 处理。
type Service struct {
	db       *sql.DB
	q        *sqlcgen.Queries
	settings *settings.Service
	keys     *KeyCache
	stripe   *StripeProvider
	paypal   *PayPalProvider
}

// NewService 构造支付编排服务。
func NewService(db *sql.DB, settingsSvc *settings.Service, keys *KeyCache) *Service {
	return &Service{
		db:       db,
		q:        sqlcgen.New(db),
		settings: settingsSvc,
		keys:     keys,
		stripe:   NewStripeProvider(keys),
		paypal:   NewPayPalProvider(keys),
	}
}

// providerReady 报告某通道是否「市场支持 + 密钥就绪」。
func (s *Service) providerReady(ctx context.Context, provider string) bool {
	supported := false
	for _, p := range s.settings.Market(ctx).Providers {
		if p == provider {
			supported = true
			break
		}
	}
	if !supported {
		return false
	}
	switch provider {
	case "stripe":
		_, ok := s.keys.secretKey()
		return ok
	case "paypal":
		_, _, _, ok := s.keys.paypalCreds()
		return ok
	default:
		return false
	}
}

// AvailableMethods 返回当前市场下已就绪（配好密钥）的支付方式，供结算页选择。
func (s *Service) AvailableMethods(ctx context.Context) []string {
	var out []string
	for _, p := range s.settings.Market(ctx).Providers {
		if s.providerReady(ctx, p) {
			out = append(out, p)
		}
	}
	return out
}

// TestStripeConnection 验证当前已激活的 Stripe Secret key 可用且模式匹配。
func (s *Service) TestStripeConnection(ctx context.Context) error {
	return s.stripe.TestConnection(ctx)
}

// ErrOrderExpired 表示订单的 30 分钟库存预留已过期，禁止发起新的收款。
var ErrOrderExpired = errors.New("payment: 订单库存预留已到期")

// CapturePayPal 校验 capture 完成状态后与 Webhook 共用收款账本和订单状态机。
// 支付引用、订单号、金额和币种共同对账；重复调用不重复结算库存。
func (s *Service) CapturePayPal(ctx context.Context, paypalOrderID string) (string, error) {
	res, err := s.paypal.Capture(ctx, paypalOrderID)
	if err != nil {
		// PayPal capture API 调用失败：err 内含 PayPal 返回的状态码+原因（doJSON 已带出）。
		slog.Error("PayPal capture 调用失败", "paypal_order_id", paypalOrderID, "err", err)
		return "", err
	}
	slog.Info("PayPal capture 返回", "paypal_order_id", paypalOrderID, "completed", res.Completed,
		"order_ref", res.OrderRef, "amount_cents", res.AmountCents, "currency", res.Currency, "capture_id", res.CaptureID)
	if !res.Completed || res.OrderRef == "" {
		slog.Warn("PayPal capture 未完成或缺 custom_id", "completed", res.Completed, "order_ref", res.OrderRef)
		return res.OrderRef, ErrMismatch
	}
	err = s.recordPayment(ctx, "paypal", WebhookEvent{
		OrderRef: res.OrderRef, PaymentRef: res.CaptureID, AmountCents: res.AmountCents, Currency: res.Currency,
	}, false)
	return res.OrderRef, err
}

// HandleWebhook 处理 Stripe Webhook：双校验 + 幂等。
// 返回 nil = 已处理或安全忽略（回 2xx）；ErrLocked/ErrBadSignature/ErrMismatch = 拒绝（回非 2xx）。
func (s *Service) HandleWebhook(ctx context.Context, payload []byte, sigHeader string) error {
	// 第一道：验签（含时间戳防重放）。锁定时 ErrLocked，上层回非 2xx 交网关重投。
	ev, err := s.stripe.VerifyWebhook(payload, sigHeader)
	if err != nil {
		return err
	}

	switch ev.Type {
	case "checkout.session.completed", "checkout.session.async_payment_succeeded":
		if ev.PaymentStatus != "paid" {
			return nil // 完成但尚未付清（如异步付款待定）：确认收到、暂不改单。
		}
		return s.markPaid(ctx, "stripe", ev)
	case "charge.refunded":
		return s.markRefundedFromWebhook(ctx, "stripe", ev)
	default:
		return nil // 其它事件：确认收到、无需动作。
	}
}

// HandlePayPalWebhook 处理 PayPal Webhook：在线验签 + 幂等。COMPLETED→已付(备份)，REFUNDED→已退款(同步)。
func (s *Service) HandlePayPalWebhook(ctx context.Context, payload []byte, headers http.Header) error {
	ev, err := s.paypal.VerifyWebhookPayPal(ctx, payload, headers)
	if err != nil {
		return err
	}
	slog.Info("PayPal webhook 签名已验签通过", "provider", "paypal", "event_id", ev.ID,
		"event_type", ev.Type, "transmission_id", headers.Get("Paypal-Transmission-Id"))
	switch ev.Type {
	case "PAYMENT.CAPTURE.COMPLETED":
		if ev.OrderRef == "" {
			return nil
		}
		return s.markPaid(ctx, "paypal", ev)
	case "PAYMENT.CAPTURE.REFUNDED":
		if ev.RefundedCents == 0 {
			r, err := s.paypal.lookupRefund(ctx, ev.RefundRef)
			if err != nil {
				return err
			}
			if r.State != "succeeded" || r.PaymentRef != ev.PaymentRef || r.Amount != ev.AmountCents || !strings.EqualFold(r.Currency, ev.Currency) {
				return ErrMismatch
			}
			ev.RefundedCents = r.TotalRefunded
		}
		return s.markRefundedFromWebhook(ctx, "paypal", ev)
	default:
		return nil
	}
}

// markPaid 在【同一事务】内完成：去重 INSERT（冲突即幂等返回）→ 第二道比对订单/金额 → pending->paid。
// 杜绝「已标记已见过但未处理」的丢单：要么都成、要么都回滚。
func (s *Service) markPaid(ctx context.Context, provider string, ev WebhookEvent) error {
	return s.recordPayment(ctx, provider, ev, true)
}

// ErrNotRefundable 订单当前状态不可退款。
var ErrNotRefundable = errors.New("payment: 订单当前状态不可退款")

// providerByName 按通道名取 provider。
func (s *Service) providerByName(name string) PaymentProvider {
	switch name {
	case "stripe":
		return s.stripe
	case "paypal":
		return s.paypal
	default:
		return nil
	}
}

// isUniqueViolation 判定是否 SQLite 唯一约束冲突（modernc.org/sqlite 文案）。
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// 确保 StripeProvider 满足接口（编译期校验）。
var _ PaymentProvider = (*StripeProvider)(nil)
