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
}

// NewService 构造支付编排服务。
func NewService(db *sql.DB, settingsSvc *settings.Service, keys *KeyCache) *Service {
	return &Service{
		db:       db,
		q:        sqlcgen.New(db),
		settings: settingsSvc,
		keys:     keys,
		stripe:   NewStripeProvider(keys),
	}
}

// providerFor 按当前主攻市场选通道。v1：市场支持 stripe 即用 stripe（预留 geo/币种细分）。
func (s *Service) providerFor(ctx context.Context) (PaymentProvider, bool) {
	m := s.settings.Market(ctx)
	for _, p := range m.Providers {
		if p == "stripe" {
			return s.stripe, true
		}
	}
	return nil, false
}

// MarketSupports 报告当前市场是否含某通道（供诊断/UI）。
func (s *Service) MarketSupports(ctx context.Context, provider string) bool {
	for _, p := range s.settings.Market(ctx).Providers {
		if p == provider {
			return true
		}
	}
	return false
}

// Ready 报告收款是否就绪（市场有通道 + 密钥已解锁）。storefront 据此决定是否跳转网关。
func (s *Service) Ready(ctx context.Context) bool {
	if _, ok := s.providerFor(ctx); !ok {
		return false
	}
	_, ok := s.keys.secretKey()
	return ok
}

// StartCheckout 为订单发起一次收款，返回托管收银台跳转 URL。
func (s *Service) StartCheckout(ctx context.Context, ord OrderForPayment) (string, error) {
	prov, ok := s.providerFor(ctx)
	if !ok {
		return "", fmt.Errorf("payment: 当前市场未配置支付通道")
	}
	sess, err := prov.CreatePayment(ctx, ord)
	if err != nil {
		return "", err
	}
	return sess.RedirectURL, nil
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
		return s.markPaid(ctx, ev)
	case "charge.refunded":
		return s.markRefundedFromWebhook(ctx, ev)
	default:
		return nil // 其它事件：确认收到、无需动作。
	}
}

// markPaid 在【同一事务】内完成：去重 INSERT（冲突即幂等返回）→ 第二道比对订单/金额 → pending->paid。
// 杜绝「已标记已见过但未处理」的丢单：要么都成、要么都回滚。
func (s *Service) markPaid(ctx context.Context, ev WebhookEvent) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("payment: 开启事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := s.q.WithTx(tx)

	// 1) 去重：事件 ID 唯一约束。冲突=此前已处理过 → 幂等放行（不重复改单）。
	if err := q.InsertWebhookEvent(ctx, sqlcgen.InsertWebhookEventParams{
		Provider: "stripe", EventID: ev.ID, EventType: ev.Type, OrderRef: ev.OrderRef,
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
		PaymentProvider: "stripe", PaymentRef: ev.PaymentRef, PublicID: ev.OrderRef,
	}); err != nil {
		return fmt.Errorf("payment: 更新订单状态失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("payment: 提交事务失败: %w", err)
	}
	return nil
}

// markRefundedFromWebhook 处理 charge.refunded：去重 + 按 payment_ref 把订单 paid->refunded（同事务幂等）。
// v1 仅同步订单状态（退款记录由后台手动退款路径写；外部 Dashboard 退款也能据此回正状态）。
func (s *Service) markRefundedFromWebhook(ctx context.Context, ev WebhookEvent) error {
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
		Provider: "stripe", EventID: ev.ID, EventType: ev.Type, OrderRef: ev.OrderRef,
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

// providerByName 按通道名取 provider（v1 仅 stripe；PayPal 在 M3.3b 接入）。
func (s *Service) providerByName(name string) PaymentProvider {
	switch name {
	case "stripe":
		return s.stripe
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
