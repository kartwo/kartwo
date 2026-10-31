// Stripe 通道 / Stripe Provider
// 功能：建 Checkout 托管收银会话、验 Webhook（验签+规范化事件）；瘦 HTTP 客户端直连 Stripe API，不引 SDK（单静态二进制/可审计）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-20 20:26:08
package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// stripeAPIBase Stripe REST 基址（测试/正式由密钥 sk_test_/sk_live_ 区分，URL 相同）。
const stripeAPIBase = "https://api.stripe.com"

// stripeAPIVersion 显式钉死的 Stripe API 版本（债2，2026-07-06 了结）。
// 为何无 SDK 仍要钉：不带 Stripe-Version 头时 Stripe 按"账号当前默认 API 版本"塑形响应，
// 而该默认会被 Stripe 平台侧在我们不知情时推进，可能令我方手解的稳定字段
// (id/type/client_reference_id/payment_status/amount_total/currency) 某天悄然变形——
// 无编译错、无异常的静默错位。显式钉一个串 = 把响应结构从 Stripe 平台时间线拿回我方 git 时间线。
// 值为 2026-06-24.dahlia（sandbox 验收路即真实带头验证，见 DECISIONS/PROGRESS）；
// 全部出站 Stripe 请求带 Stripe-Version:<此常量>，单一常量=单一事实源。升级须走 CONVENTIONS 流程改此处。
const stripeAPIVersion = "2026-06-24.dahlia"

// StripeProvider 实现 PaymentProvider；密钥实时从 KeyCache 取（支持改密钥即时生效）。
type StripeProvider struct {
	keys       *KeyCache
	httpClient *http.Client
	apiBase    string // Stripe REST 基址；可在测试中改指 httptest
	tolerance  time.Duration
	now        func() time.Time
}

// NewStripeProvider 构造 Stripe 通道。
func NewStripeProvider(keys *KeyCache) *StripeProvider {
	return &StripeProvider{
		keys:       keys,
		httpClient: &http.Client{Timeout: 20 * time.Second},
		apiBase:    stripeAPIBase,
		tolerance:  defaultTolerance,
		now:        time.Now,
	}
}

// Name 通道名。
func (p *StripeProvider) Name() string { return "stripe" }

// CreatePayment 建一个 Checkout Session（mode=payment），把订单 public_id 写入 client_reference_id 作回调对账锚点。
// v1 用单行合计项（金额=订单 total_cents），与 Webhook amount_total 校验天然一致。
func (p *StripeProvider) CreatePayment(ctx context.Context, ord OrderForPayment) (PaymentSession, error) {
	secret, ok := p.keys.secretKey()
	if !ok {
		return PaymentSession{}, ErrLocked
	}

	form := url.Values{}
	form.Set("mode", "payment")
	expiresAt := ord.ExpiresAt
	if expiresAt == 0 {
		expiresAt = p.now().Add(30 * time.Minute).Unix()
	}
	form.Set("expires_at", strconv.FormatInt(expiresAt, 10))
	form.Set("success_url", ord.SuccessURL)
	form.Set("cancel_url", ord.CancelURL)
	form.Set("client_reference_id", ord.PublicID)
	form.Set("metadata[order_ref]", ord.PublicID)
	if ord.Email != "" {
		form.Set("customer_email", ord.Email)
	}
	form.Set("line_items[0][quantity]", "1")
	form.Set("line_items[0][price_data][currency]", strings.ToLower(ord.Currency))
	form.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(ord.AmountCents, 10))
	name := ord.Description
	if name == "" {
		name = "Order " + ord.PublicID
	}
	form.Set("line_items[0][price_data][product_data][name]", name)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiBase+"/v1/checkout/sessions", strings.NewReader(form.Encode()))
	if err != nil {
		return PaymentSession{}, fmt.Errorf("payment: 构造 Stripe 请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Stripe-Version", stripeAPIVersion)
	if ord.IdempotencyKey != "" {
		req.Header.Set("Idempotency-Key", ord.IdempotencyKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return PaymentSession{}, fmt.Errorf("payment: 调用 Stripe 失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode >= 400 {
		var rejected struct {
			Error struct {
				Type  string `json:"type"`
				Param string `json:"param"`
			} `json:"error"`
		}
		if resp.StatusCode == http.StatusBadRequest && json.Unmarshal(body, &rejected) == nil && rejected.Error.Type == "invalid_request_error" && rejected.Error.Param == "expires_at" {
			return PaymentSession{}, errCheckoutExpiryRejected
		}
		return PaymentSession{}, fmt.Errorf("payment: Stripe 建会话失败 (%d): %s", resp.StatusCode, stripeErrMsg(body))
	}

	var out struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return PaymentSession{}, fmt.Errorf("payment: 解析 Stripe 响应失败: %w", err)
	}
	if out.ID == "" || !validCheckoutURL(out.URL) {
		return PaymentSession{}, fmt.Errorf("payment: Stripe 未返回收银台 URL")
	}
	return PaymentSession{RedirectURL: out.URL, Reference: out.ID}, nil
}

// TestConnection validates the saved account key and its test/live prefix.
// Stripe's account object has no livemode field; only a successful authenticated
// account response verifies the key. This does not verify charges or webhooks.
func (p *StripeProvider) TestConnection(ctx context.Context) error {
	secret, mode, ok := p.keys.stripeConnectionCredentials()
	if !ok {
		return ErrLocked
	}
	if mode != "test" && mode != "live" {
		return fmt.Errorf("payment: Stripe 配置模式不合法")
	}
	parts := strings.SplitN(secret, "_", 3)
	if len(parts) != 3 || (parts[0] != "sk" && parts[0] != "rk") || parts[2] == "" || strings.ContainsAny(secret, " \r\n\t\x00") {
		return fmt.Errorf("payment: Stripe 账户密钥格式不合法")
	}
	if parts[1] != mode {
		return fmt.Errorf("payment: Stripe 密钥模式与当前配置不匹配")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.apiBase+"/v1/account", nil)
	if err != nil {
		return fmt.Errorf("payment: 构造 Stripe 测试连接请求失败")
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Stripe-Version", stripeAPIVersion)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("payment: 调用 Stripe 测试连接失败")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("payment: Stripe 测试连接失败 (%d)", resp.StatusCode)
	}
	const maxAccountBody = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAccountBody+1))
	if err != nil || len(body) > maxAccountBody {
		return fmt.Errorf("payment: Stripe 账户响应读取失败")
	}
	var account struct {
		ID     string `json:"id"`
		Object string `json:"object"`
	}
	if json.Unmarshal(body, &account) != nil || account.Object != "account" || !strings.HasPrefix(account.ID, "acct_") || len(account.ID) <= len("acct_") || !validRefundReference(account.ID) {
		return fmt.Errorf("payment: Stripe 账户响应不合法")
	}
	currentSecret, currentMode, currentOK := p.keys.stripeConnectionCredentials()
	if !currentOK || currentSecret != secret || currentMode != mode {
		return fmt.Errorf("payment: Stripe 配置已变化，请重新测试")
	}
	return ctx.Err()
}

// VerifyWebhook 第一道校验：用内存中的 whsec 对原始字节验签 + 时间戳容差；通过后规范化事件。
// 锁定（无 whsec）返回 ErrLocked，让上层回非 2xx 交 Stripe 重投，绝不放行。
func (p *StripeProvider) VerifyWebhook(payload []byte, sigHeader string) (WebhookEvent, error) {
	whsec, ok := p.keys.webhookSecret()
	if !ok {
		return WebhookEvent{}, ErrLocked
	}
	if err := verifyStripeSignature(payload, sigHeader, whsec, p.tolerance, p.now()); err != nil {
		return WebhookEvent{}, err
	}

	// 同时容纳 checkout.session（付款）与 charge（退款）两种 object 形态：字段并存、按需取用。
	var ev struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object struct {
				ID                string `json:"id"`
				ClientReferenceID string `json:"client_reference_id"` // checkout.session
				PaymentStatus     string `json:"payment_status"`      // checkout.session
				Amount            int64  `json:"amount"`
				AmountRefunded    int64  `json:"amount_refunded"`
				AmountTotal       int64  `json:"amount_total"`   // checkout.session
				Currency          string `json:"currency"`       // 二者皆有
				PaymentIntent     string `json:"payment_intent"` // 二者皆有（退款引用锚点）
				Metadata          struct {
					OrderRef        string `json:"order_ref"`
					RefundRequestID string `json:"kartwo_refund_id"`
				} `json:"metadata"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &ev); err != nil {
		return WebhookEvent{}, fmt.Errorf("payment: 解析 Webhook 事件失败: %w", err)
	}

	ref := ev.Data.Object.ClientReferenceID
	if ref == "" {
		ref = ev.Data.Object.Metadata.OrderRef
	}
	amount := ev.Data.Object.AmountTotal
	if ev.Type == "charge.refunded" || strings.HasPrefix(ev.Type, "refund.") {
		amount = ev.Data.Object.Amount
	}
	return WebhookEvent{
		ID:              ev.ID,
		Type:            ev.Type,
		OrderRef:        ref,
		PaymentRef:      ev.Data.Object.PaymentIntent,
		PaymentStatus:   ev.Data.Object.PaymentStatus,
		AmountCents:     amount,
		RefundedCents:   ev.Data.Object.AmountRefunded,
		Currency:        ev.Data.Object.Currency,
		RefundRef:       ev.Data.Object.ID,
		RefundRequestID: ev.Data.Object.Metadata.RefundRequestID,
	}, nil
}

// stripeErrMsg 从 Stripe 错误响应里抽人话消息（失败则返回原文截断）。
func stripeErrMsg(body []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
		return e.Error.Message
	}
	s := string(body)
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
