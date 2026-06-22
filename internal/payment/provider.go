// 支付路由层 / Payment Provider Abstraction
// 功能：定义 PaymentProvider 接口与通用类型（按 ARCHITECTURE §9）；规范化的 Webhook 事件供幂等处理
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-20 20:26:08
package payment

import (
	"context"
	"errors"
	"fmt"
)

// 设置项键名（admin 写入、payment 读取；敏感项加密存，见 settings.SetEncrypted）。
const (
	// KeyStripeMode 沙箱/正式开关：test | live（明文）。
	KeyStripeMode = "pay.stripe.mode"
	// KeyStripePublishable 可发布密钥 pk_*（公开值，明文）。
	KeyStripePublishable = "pay.stripe.publishable"
	// KeyStripeSecret 密钥 sk_*（加密存）。
	KeyStripeSecret = "pay.stripe.secret"
	// KeyStripeWebhookSecret Webhook 签名密钥 whsec_*（加密存；本地 CLI 转发与正式端点为不同值，均作普通配置项）。
	KeyStripeWebhookSecret = "pay.stripe.webhook_secret"
)

var (
	// ErrLocked 收款密钥未解锁（进程重启后无人登录过）；Webhook 据此返回非 2xx 交给网关重投。
	ErrLocked = errors.New("payment: 收款密钥未解锁（请登录一次后台以激活收款）")
	// ErrBadSignature Webhook 签名校验失败（伪造/篡改/过期）；下游一律拒绝改单。
	ErrBadSignature = errors.New("payment: Webhook 签名校验失败")
	// ErrSigFormat 签名头格式非法（缺 t 或 v1）。
	ErrSigFormat = fmt.Errorf("%w: 签名头格式非法", ErrBadSignature)
	// ErrSigExpired 时间戳超出容差（防重放）。
	ErrSigExpired = fmt.Errorf("%w: 时间戳超出容差（防重放）", ErrBadSignature)
	// ErrSigMismatch HMAC 不匹配（密钥不符或负载被篡改）。
	ErrSigMismatch = fmt.Errorf("%w: HMAC 不匹配（可能伪造）", ErrBadSignature)
	// ErrMismatch 第二道校验失败：订单号对不上或金额/币种与库内订单不一致（拒绝改单）。
	ErrMismatch = errors.New("payment: 订单引用或金额与库内订单不符（拒绝改单）")
	// ErrNotImplemented 能力尚未落地（如退款留 M3.3）。
	ErrNotImplemented = errors.New("payment: 能力尚未实现")
)

// OrderForPayment 是发起一次收款所需的最小订单信息。
type OrderForPayment struct {
	PublicID    string // 订单 public_id，写入网关 client_reference_id 作回调对账锚点
	Email       string
	Currency    string // 如 USD
	AmountCents int64  // 应收总额（分），与 Webhook amount_total 比对
	Description string // 收银台展示用一行描述
	SuccessURL  string
	CancelURL   string
}

// PaymentSession 是 CreatePayment 的结果：把买家引导到何处付款。
type PaymentSession struct {
	RedirectURL string // 托管收银台 URL
	Reference   string // 网关会话/引用 ID
}

// WebhookEvent 是 VerifyWebhook 验签通过后规范化的事件（供幂等处理）。
type WebhookEvent struct {
	ID            string // 网关事件 ID（幂等去重键）
	Type          string // 如 checkout.session.completed / charge.refunded
	OrderRef      string // 我方订单 public_id（来自 client_reference_id；退款事件无）
	PaymentRef    string // 网关支付引用（Stripe payment_intent）；用于退款与退款事件回查订单
	PaymentStatus string // 如 paid（不假设事件类型即已付，须显式校验）
	AmountCents   int64  // 网关侧成交总额（分）
	Currency      string // 网关侧币种
}

// PaymentProvider 支付通道抽象（ARCHITECTURE §9）。双校验在 VerifyWebhook 内：验签 + 由调用方比对订单/金额。
type PaymentProvider interface {
	Name() string
	CreatePayment(ctx context.Context, ord OrderForPayment) (PaymentSession, error)
	VerifyWebhook(payload []byte, sigHeader string) (WebhookEvent, error)
	// Refund 整数分退款，返回网关退款 ID（provider_refund_id）。
	Refund(ctx context.Context, reference string, amountCents int64) (string, error)
}
