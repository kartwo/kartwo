// PayPal 通道 / PayPal Provider (Orders v2)
// 功能：OAuth token + 建单(hosted 审批) + 同步 capture(已付判定)；退款/验签留 M3.3b-2。瘦 HTTP 客户端，不引 SDK
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-23 00:20:52
package payment

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	paypalSandboxBase = "https://api-m.sandbox.paypal.com"
	paypalLiveBase    = "https://api-m.paypal.com"
)

// CaptureResult 是 PayPal capture 的规范化结果。
type CaptureResult struct {
	Completed   bool
	OrderRef    string // 我方订单 public_id（来自 custom_id）
	CaptureID   string // 退款锚点
	AmountCents int64
	Currency    string
}

// PayPalProvider 实现 PaymentProvider（付款部分）；密钥实时从 KeyCache 取。
type PayPalProvider struct {
	keys       *KeyCache
	httpClient *http.Client
	apiBase    string // 测试可注入；空则按 mode 取 sandbox/live
	now        func() time.Time

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// NewPayPalProvider 构造 PayPal 通道。
func NewPayPalProvider(keys *KeyCache) *PayPalProvider {
	return &PayPalProvider{
		keys:       keys,
		httpClient: &http.Client{Timeout: 20 * time.Second},
		now:        time.Now,
	}
}

// Name 通道名。
func (p *PayPalProvider) Name() string { return "paypal" }

func (p *PayPalProvider) base() string {
	if p.apiBase != "" {
		return p.apiBase
	}
	if _, _, mode, _ := p.keys.paypalCreds(); mode == "live" {
		return paypalLiveBase
	}
	return paypalSandboxBase
}

// accessToken 取（缓存的）OAuth2 access_token（client_credentials）。
func (p *PayPalProvider) accessToken(ctx context.Context) (string, error) {
	id, secret, _, ok := p.keys.paypalCreds()
	if !ok {
		return "", ErrLocked
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != "" && p.now().Before(p.tokenExp) {
		return p.token, nil
	}
	form := url.Values{"grant_type": {"client_credentials"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.base()+"/v1/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("payment: 构造 PayPal token 请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(id+":"+secret)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("payment: 调用 PayPal token 失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("payment: PayPal token 失败 (%d): %s", resp.StatusCode, paypalErrMsg(body))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.AccessToken == "" {
		return "", fmt.Errorf("payment: 解析 PayPal token 失败")
	}
	p.token = out.AccessToken
	exp := out.ExpiresIn - 60 // 留 60s 余量
	if exp < 0 {
		exp = 0
	}
	p.tokenExp = p.now().Add(time.Duration(exp) * time.Second)
	return p.token, nil
}

// CreatePayment 建 Orders v2 单（intent=CAPTURE），custom_id=订单 public_id；返回 hosted 审批跳转 URL。
func (p *PayPalProvider) CreatePayment(ctx context.Context, ord OrderForPayment) (PaymentSession, error) {
	token, err := p.accessToken(ctx)
	if err != nil {
		return PaymentSession{}, err
	}
	reqBody := map[string]any{
		"intent": "CAPTURE",
		"purchase_units": []map[string]any{{
			"custom_id": ord.PublicID,
			"amount": map[string]any{
				"currency_code": strings.ToUpper(ord.Currency),
				"value":         centsToDecimal(ord.AmountCents),
			},
		}},
		"payment_source": map[string]any{
			"paypal": map[string]any{
				"experience_context": map[string]any{
					"return_url":          ord.SuccessURL,
					"cancel_url":          ord.CancelURL,
					"user_action":         "PAY_NOW",
					"shipping_preference": "NO_SHIPPING",
				},
			},
		},
	}
	raw, err := p.doJSON(ctx, token, http.MethodPost, "/v2/checkout/orders", reqBody)
	if err != nil {
		return PaymentSession{}, err
	}
	var out struct {
		ID    string `json:"id"`
		Links []struct {
			Href string `json:"href"`
			Rel  string `json:"rel"`
		} `json:"links"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.ID == "" {
		return PaymentSession{}, fmt.Errorf("payment: 解析 PayPal 建单响应失败")
	}
	href := ""
	for _, l := range out.Links {
		if l.Rel == "payer-action" || l.Rel == "approve" {
			href = l.Href
			break
		}
	}
	if href == "" {
		return PaymentSession{}, fmt.Errorf("payment: PayPal 未返回审批链接")
	}
	return PaymentSession{RedirectURL: href, Reference: out.ID}, nil
}

// Capture 对已审批的 PayPal 订单做 capture，规范化结果（已付判定的权威同步信号）。
func (p *PayPalProvider) Capture(ctx context.Context, paypalOrderID string) (CaptureResult, error) {
	token, err := p.accessToken(ctx)
	if err != nil {
		return CaptureResult{}, err
	}
	raw, err := p.doJSON(ctx, token, http.MethodPost, "/v2/checkout/orders/"+url.PathEscape(paypalOrderID)+"/capture", map[string]any{})
	if err != nil {
		return CaptureResult{}, err
	}
	var out struct {
		Status        string `json:"status"`
		PurchaseUnits []struct {
			CustomID string `json:"custom_id"`
			Payments struct {
				Captures []struct {
					ID       string `json:"id"`
					CustomID string `json:"custom_id"` // PayPal capture 响应把 custom_id 回填在 capture 对象上
					Amount   struct {
						CurrencyCode string `json:"currency_code"`
						Value        string `json:"value"`
					} `json:"amount"`
				} `json:"captures"`
			} `json:"payments"`
		} `json:"purchase_units"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return CaptureResult{}, fmt.Errorf("payment: 解析 PayPal capture 响应失败: %w", err)
	}
	res := CaptureResult{Completed: out.Status == "COMPLETED"}
	if len(out.PurchaseUnits) > 0 {
		pu := out.PurchaseUnits[0]
		res.OrderRef = pu.CustomID
		if len(pu.Payments.Captures) > 0 {
			cap0 := pu.Payments.Captures[0]
			res.CaptureID = cap0.ID
			res.Currency = cap0.Amount.CurrencyCode
			res.AmountCents, _ = decimalToCents(cap0.Amount.Value)
			// capture 响应里 custom_id 通常在 capture 对象、而非 purchase_unit 顶层；顶层缺失时回退取此处。
			if res.OrderRef == "" {
				res.OrderRef = cap0.CustomID
			}
		}
	}
	return res, nil
}

// Refund 对 capture 整单全额退款：POST /v2/payments/captures/{capture}/refund（空 body=全额）。返回退款 ID。
// v1 仅全额，故忽略 amountCents（空 body 让 PayPal 退全部已捕获金额）。
func (p *PayPalProvider) Refund(ctx context.Context, captureID string, _ int64) (string, error) {
	if captureID == "" {
		return "", fmt.Errorf("payment: 缺 capture 引用，无法退款")
	}
	token, err := p.accessToken(ctx)
	if err != nil {
		return "", err
	}
	raw, err := p.doJSON(ctx, token, http.MethodPost, "/v2/payments/captures/"+url.PathEscape(captureID)+"/refund", map[string]any{})
	if err != nil {
		return "", err
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.ID == "" {
		return "", fmt.Errorf("payment: 解析 PayPal 退款响应失败")
	}
	return out.ID, nil
}

// VerifyWebhook 接口占位——PayPal webhook 多头验签走 VerifyWebhookPayPal（不同签名），不经此通用接口。
func (p *PayPalProvider) VerifyWebhook(_ []byte, _ string) (WebhookEvent, error) {
	return WebhookEvent{}, ErrNotImplemented
}

// VerifyWebhookPayPal 在线验签（/v1/notifications/verify-webhook-signature，需 webhook_id + 多个 PayPal 头）+ 规范化事件。
// 注：PayPal 模拟器事件过不了真实验签（已知限制），真实端到端验收推迟 M4；此处逻辑+单测(mock)就位。
func (p *PayPalProvider) VerifyWebhookPayPal(ctx context.Context, payload []byte, h http.Header) (WebhookEvent, error) {
	webhookID, ok := p.keys.paypalWebhookID()
	if !ok {
		return WebhookEvent{}, ErrLocked
	}
	token, err := p.accessToken(ctx)
	if err != nil {
		return WebhookEvent{}, err
	}
	var event json.RawMessage = payload
	verifyReq := map[string]any{
		"auth_algo":         h.Get("Paypal-Auth-Algo"),
		"cert_url":          h.Get("Paypal-Cert-Url"),
		"transmission_id":   h.Get("Paypal-Transmission-Id"),
		"transmission_sig":  h.Get("Paypal-Transmission-Sig"),
		"transmission_time": h.Get("Paypal-Transmission-Time"),
		"webhook_id":        webhookID,
		"webhook_event":     event,
	}
	raw, err := p.doJSON(ctx, token, http.MethodPost, "/v1/notifications/verify-webhook-signature", verifyReq)
	if err != nil {
		return WebhookEvent{}, err
	}
	var vr struct {
		VerificationStatus string `json:"verification_status"`
	}
	if err := json.Unmarshal(raw, &vr); err != nil || vr.VerificationStatus != "SUCCESS" {
		return WebhookEvent{}, ErrSigMismatch
	}
	return parsePayPalEvent(payload)
}

// parsePayPalEvent 规范化 PayPal 事件（capture 完成/退款）。
func parsePayPalEvent(payload []byte) (WebhookEvent, error) {
	var ev struct {
		ID       string `json:"id"`
		Type     string `json:"event_type"`
		Resource struct {
			ID       string `json:"id"`        // capture id（COMPLETED）/ refund id（REFUNDED）
			CustomID string `json:"custom_id"` // 我方订单 public_id（COMPLETED 时有）
			Amount   struct {
				Value        string `json:"value"`
				CurrencyCode string `json:"currency_code"`
			} `json:"amount"`
			Links []struct {
				Href string `json:"href"`
				Rel  string `json:"rel"`
			} `json:"links"`
		} `json:"resource"`
	}
	if err := json.Unmarshal(payload, &ev); err != nil {
		return WebhookEvent{}, fmt.Errorf("payment: 解析 PayPal 事件失败: %w", err)
	}
	out := WebhookEvent{ID: ev.ID, Type: ev.Type, OrderRef: ev.Resource.CustomID, Currency: ev.Resource.Amount.CurrencyCode}
	if ev.Resource.Amount.Value != "" {
		out.AmountCents, _ = decimalToCents(ev.Resource.Amount.Value)
	}
	switch ev.Type {
	case "PAYMENT.CAPTURE.COMPLETED":
		out.PaymentRef = ev.Resource.ID // capture id
	case "PAYMENT.CAPTURE.REFUNDED":
		out.PaymentRef = captureIDFromLinks(ev.Resource.Links) // 退款事件：从 links 取 capture id
	}
	return out, nil
}

// captureIDFromLinks 从 PayPal 退款资源的 links 里取被退款的 capture id（href 形如 .../captures/{id}）。
func captureIDFromLinks(links []struct {
	Href string `json:"href"`
	Rel  string `json:"rel"`
}) string {
	for _, l := range links {
		if i := strings.Index(l.Href, "/captures/"); i >= 0 {
			rest := l.Href[i+len("/captures/"):]
			if j := strings.IndexByte(rest, '/'); j >= 0 {
				rest = rest[:j]
			}
			return rest
		}
	}
	return ""
}

// doJSON 发一个带 Bearer 的 JSON 请求，返回响应体（4xx+ 视为错误）。
func (p *PayPalProvider) doJSON(ctx context.Context, token, method, path string, payload any) ([]byte, error) {
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("payment: 编码 PayPal 请求失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.base()+path, strings.NewReader(string(buf)))
	if err != nil {
		return nil, fmt.Errorf("payment: 构造 PayPal 请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("payment: 调用 PayPal 失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("payment: PayPal 请求失败 (%d): %s", resp.StatusCode, paypalErrMsg(body))
	}
	return body, nil
}

// centsToDecimal 整数分 → PayPal 小数字符串（v1 仅 2 位小数币种）。
func centsToDecimal(cents int64) string {
	neg := ""
	if cents < 0 {
		neg, cents = "-", -cents
	}
	return fmt.Sprintf("%s%d.%02d", neg, cents/100, cents%100)
}

// decimalToCents 小数字符串 → 整数分（2 位小数）。
func decimalToCents(s string) (int64, error) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("payment: 金额解析失败 %q", s)
	}
	frac := "00"
	if len(parts) == 2 {
		frac = (parts[1] + "00")[:2]
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("payment: 金额解析失败 %q", s)
	}
	if whole < 0 {
		return whole*100 - f, nil
	}
	return whole*100 + f, nil
}

func paypalErrMsg(body []byte) string {
	var e struct {
		Message string `json:"message"`
		Details []struct {
			Description string `json:"description"`
		} `json:"details"`
	}
	if json.Unmarshal(body, &e) == nil {
		if len(e.Details) > 0 && e.Details[0].Description != "" {
			return e.Details[0].Description
		}
		if e.Message != "" {
			return e.Message
		}
	}
	s := string(body)
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

var _ PaymentProvider = (*PayPalProvider)(nil)
