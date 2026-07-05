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
	"fmt"
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

// StartCheckout 用指定通道为订单发起一次收款，返回托管收银台/审批跳转 URL。
func (s *Service) StartCheckout(ctx context.Context, provider string, ord OrderForPayment) (string, error) {
	prov := s.providerByName(provider)
	if prov == nil || !s.providerReady(ctx, provider) {
		return "", fmt.Errorf("payment: 支付方式 %q 不可用", provider)
	}
	sess, err := prov.CreatePayment(ctx, ord)
	if err != nil {
		return "", err
	}
	return sess.RedirectURL, nil
}

// CapturePayPal 对已审批的 PayPal 订单做 capture 并据其结果改单（已付判定=同步 capture）。
// 校验：completed + custom_id 能对上库内订单 + 金额/币种一致；改单条件更新(pending->paid)幂等。返回订单 public_id。
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
	ord, err := s.q.GetOrderByPublicID(ctx, res.OrderRef)
	if errors.Is(err, sql.ErrNoRows) {
		slog.Warn("PayPal capture 对账：库内无此订单", "order_ref", res.OrderRef)
		return res.OrderRef, ErrMismatch
	} else if err != nil {
		return res.OrderRef, fmt.Errorf("payment: 取订单失败: %w", err)
	}
	if ord.TotalCents != res.AmountCents || !strings.EqualFold(ord.Currency, res.Currency) {
		slog.Warn("PayPal capture 对账：金额/币种不符", "order_ref", res.OrderRef,
			"order_cents", ord.TotalCents, "capture_cents", res.AmountCents,
			"order_currency", ord.Currency, "capture_currency", res.Currency)
		return res.OrderRef, ErrMismatch
	}
	if _, err := s.q.MarkOrderPaidByPublicID(ctx, sqlcgen.MarkOrderPaidByPublicIDParams{
		PaymentProvider: "paypal", PaymentRef: res.CaptureID, PublicID: res.OrderRef,
	}); err != nil {
		return res.OrderRef, fmt.Errorf("payment: 更新订单状态失败: %w", err)
	}
	slog.Info("PayPal 订单已付", "order_ref", res.OrderRef, "capture_id", res.CaptureID)
	return res.OrderRef, nil
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
	case "checkout.session.completed":
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
	switch ev.Type {
	case "PAYMENT.CAPTURE.COMPLETED":
		if ev.OrderRef == "" {
			return nil
		}
		return s.markPaid(ctx, "paypal", ev)
	case "PAYMENT.CAPTURE.REFUNDED":
		return s.markRefundedFromWebhook(ctx, "paypal", ev)
	default:
		return nil
	}
}

// markPaid 在【同一事务】内完成：去重 INSERT（冲突即幂等返回）→ 第二道比对订单/金额 → pending->paid。
// 杜绝「已标记已见过但未处理」的丢单：要么都成、要么都回滚。
func (s *Service) markPaid(ctx context.Context, provider string, ev WebhookEvent) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("payment: 开启事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := s.q.WithTx(tx)

	// 1) 去重：事件 ID 唯一约束。冲突=此前已处理过 → 幂等放行（不重复改单）。
	if err := q.InsertWebhookEvent(ctx, sqlcgen.InsertWebhookEventParams{
		Provider: provider, EventID: ev.ID, EventType: ev.Type, OrderRef: ev.OrderRef,
	}); err != nil {
		if isUniqueViolation(err) {
			return nil // 已处理过，回 2xx。
		}
		return fmt.Errorf("payment: 记录事件失败: %w", err)
	}

	// 2) 第二道校验：订单号能对上，且金额/币种与库内订单一致（防「签名真但张冠李戴/被篡改」）。
	ord, err := q.GetOrderByPublicID(ctx, ev.OrderRef)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMismatch
	} else if err != nil {
		return fmt.Errorf("payment: 取订单失败: %w", err)
	}
	if ord.TotalCents != ev.AmountCents || !strings.EqualFold(ord.Currency, ev.Currency) {
		return ErrMismatch
	}

	// 3) pending->paid（条件更新；非 pending 则 0 行，天然幂等、不回退已取消单）。同时落支付引用供退款。
	if _, err := q.MarkOrderPaidByPublicID(ctx, sqlcgen.MarkOrderPaidByPublicIDParams{
		PaymentProvider: provider, PaymentRef: ev.PaymentRef, PublicID: ev.OrderRef,
	}); err != nil {
		return fmt.Errorf("payment: 更新订单状态失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("payment: 提交事务失败: %w", err)
	}
	return nil
}

// markRefundedFromWebhook 处理退款事件：去重 + 按 payment_ref 把订单 paid->refunded（同事务幂等）。
// v1 仅同步订单状态（退款记录由后台手动退款路径写；外部 Dashboard 退款也能据此回正状态）。
func (s *Service) markRefundedFromWebhook(ctx context.Context, provider string, ev WebhookEvent) error {
	if ev.PaymentRef == "" {
		return nil // 无支付引用无法回查订单：确认收到、忽略。
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("payment: 开启事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := s.q.WithTx(tx)

	if err := q.InsertWebhookEvent(ctx, sqlcgen.InsertWebhookEventParams{
		Provider: provider, EventID: ev.ID, EventType: ev.Type, OrderRef: ev.OrderRef,
	}); err != nil {
		if isUniqueViolation(err) {
			return nil // 已处理过，幂等放行。
		}
		return fmt.Errorf("payment: 记录事件失败: %w", err)
	}
	// 条件更新：仅 paid->refunded；非 paid（含已 refunded）则 0 行、天然幂等。
	if _, err := q.MarkOrderRefundedByPaymentRef(ctx, ev.PaymentRef); err != nil {
		return fmt.Errorf("payment: 更新退款状态失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("payment: 提交事务失败: %w", err)
	}
	return nil
}

// ErrNotRefundable 订单当前状态不可退款（非 paid）。
var ErrNotRefundable = errors.New("payment: 订单当前状态不可退款")

// Refund 后台手动整单全额退款：校验可退 → 调网关退款 → 事务内写退款记录 + 订单 paid->refunded。
func (s *Service) Refund(ctx context.Context, orderPublicID string) error {
	ord, err := s.q.GetOrderByPublicID(ctx, orderPublicID)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.ErrNoRows
	} else if err != nil {
		return fmt.Errorf("payment: 取订单失败: %w", err)
	}
	if ord.Status != "paid" {
		return ErrNotRefundable
	}
	if ord.PaymentRef == "" {
		return fmt.Errorf("payment: 订单缺支付引用，无法退款")
	}

	// 调网关退款（整数分，退到 payment_intent）。先退款再落库：退款成功才改状态。
	prov := s.providerByName(ord.PaymentProvider)
	if prov == nil {
		return fmt.Errorf("payment: 未知支付通道 %q", ord.PaymentProvider)
	}
	refundID, err := prov.Refund(ctx, ord.PaymentRef, ord.TotalCents)
	if err != nil {
		return err
	}

	// 事务内：写退款记录 + 订单 paid->refunded（条件更新，幂等）。
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("payment: 开启事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := s.q.WithTx(tx)
	if err := q.InsertRefund(ctx, sqlcgen.InsertRefundParams{
		OrderID: ord.ID, Provider: ord.PaymentProvider, ProviderRefundID: refundID, AmountCents: ord.TotalCents,
	}); err != nil {
		if isUniqueViolation(err) {
			return nil // 同一退款已记录（重复点击/竞态）：幂等放行。
		}
		return fmt.Errorf("payment: 写退款记录失败: %w", err)
	}
	if _, err := q.MarkOrderRefundedByPublicID(ctx, orderPublicID); err != nil {
		return fmt.Errorf("payment: 更新订单状态失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("payment: 提交事务失败: %w", err)
	}
	return nil
}

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
