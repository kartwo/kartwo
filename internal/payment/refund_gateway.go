// 退款网关 / Refund Gateway
// 功能：按持久幂等键提交指定金额退款，查询状态并校验付款引用、币种及累计退款金额
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 22:12:00
package payment

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

var errRefundRejected = errors.New("payment: 网关拒绝退款参数")

type gatewayRefund struct {
	ID, State, PaymentRef, Currency, RequestID string
	Amount, TotalRefunded                      int64
}

type refundGateway interface {
	submitRefund(context.Context, refundAttempt) (gatewayRefund, error)
	lookupRefund(context.Context, string) (gatewayRefund, error)
}

func (s *Service) refundGateway(provider string) refundGateway {
	if provider == "stripe" {
		return s.stripe
	}
	if provider == "paypal" {
		return s.paypal
	}
	return nil
}

// Refund requests use their own frozen schema version, independent of checkout.
const refundRequestVersion = "refund-v1"

// Provider refund IDs are opaque ASCII identifiers, not URLs or path fragments.
func validRefundReference(id string) bool {
	if len(id) == 0 || len(id) > 255 {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}

func (p *StripeProvider) refundHTTP(ctx context.Context, method, path string, form url.Values, key string) ([]byte, error) {
	secret, ok := p.keys.secretKey()
	if !ok {
		return nil, ErrLocked
	}
	req, err := http.NewRequestWithContext(ctx, method, p.apiBase+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Stripe-Version", stripeAPIVersion)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		var e struct {
			Error struct {
				Type string `json:"type"`
			} `json:"error"`
		}
		if method == http.MethodPost && resp.StatusCode == 400 && json.Unmarshal(body, &e) == nil && e.Error.Type == "invalid_request_error" {
			return nil, errRefundRejected
		}
		return nil, ErrRefundUnconfirmed
	}
	return body, nil
}

func (p *StripeProvider) submitRefund(ctx context.Context, a refundAttempt) (gatewayRefund, error) {
	form := url.Values{"payment_intent": {a.PaymentRef}, "amount": {strconv.FormatInt(a.AmountCents, 10)}, "metadata[kartwo_refund_id]": {a.ID}}
	raw, err := p.refundHTTP(ctx, http.MethodPost, "/v1/refunds", form, a.ID)
	if err != nil {
		return gatewayRefund{}, err
	}
	return p.decodeRefund(ctx, raw)
}
func (p *StripeProvider) lookupRefund(ctx context.Context, id string) (gatewayRefund, error) {
	if !validRefundReference(id) {
		return gatewayRefund{}, ErrMismatch
	}
	raw, err := p.refundHTTP(ctx, http.MethodGet, "/v1/refunds/"+url.PathEscape(id), nil, "")
	if err != nil {
		return gatewayRefund{}, err
	}
	result, err := p.decodeRefund(ctx, raw)
	if err == nil && result.ID != id {
		return gatewayRefund{}, ErrMismatch
	}
	return result, err
}
func (p *StripeProvider) decodeRefund(ctx context.Context, raw []byte) (gatewayRefund, error) {
	var data struct {
		ID, Status, Currency, Charge string
		PaymentRef                   string `json:"payment_intent"`
		Amount                       int64
		Metadata                     struct {
			RequestID string `json:"kartwo_refund_id"`
		}
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return gatewayRefund{}, err
	}
	r := gatewayRefund{ID: data.ID, PaymentRef: data.PaymentRef, Currency: data.Currency, Amount: data.Amount, RequestID: data.Metadata.RequestID, State: "pending"}
	if r.ID == "" || r.PaymentRef == "" || r.Amount <= 0 || r.Currency == "" {
		return r, ErrMismatch
	}
	switch data.Status {
	case "succeeded":
		r.State = "succeeded"
	case "failed", "canceled":
		r.State = "failed"
	case "pending", "requires_action":
	default:
		return r, ErrRefundUnconfirmed
	}
	if r.State != "succeeded" {
		return r, nil
	}
	if !validRefundReference(data.Charge) {
		return r, ErrMismatch
	}
	raw, err := p.refundHTTP(ctx, http.MethodGet, "/v1/charges/"+url.PathEscape(data.Charge), nil, "")
	if err != nil {
		return r, err
	}
	var charge struct {
		ID, Currency string
		PaymentRef   string `json:"payment_intent"`
		Refunded     int64  `json:"amount_refunded"`
	}
	if json.Unmarshal(raw, &charge) != nil || charge.ID != data.Charge || charge.PaymentRef != r.PaymentRef || !strings.EqualFold(charge.Currency, r.Currency) || charge.Refunded < r.Amount {
		return r, ErrMismatch
	}
	r.TotalRefunded = charge.Refunded
	return r, nil
}

type paypalMoney struct {
	Value    string `json:"value"`
	Currency string `json:"currency_code"`
}

func (p *PayPalProvider) refundHTTP(ctx context.Context, method, path string, payload any, key string) ([]byte, error) {
	token, err := p.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = strings.NewReader(string(raw))
	}
	req, err := http.NewRequestWithContext(ctx, method, p.base()+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation")
	if key != "" {
		req.Header.Set("PayPal-Request-Id", key)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		var e struct {
			Name    string `json:"name"`
			Details []struct {
				Issue string `json:"issue"`
			} `json:"details"`
		}
		if method == http.MethodPost && resp.StatusCode == 400 && json.Unmarshal(raw, &e) == nil && e.Name == "INVALID_REQUEST" {
			for _, detail := range e.Details {
				switch detail.Issue {
				case "INVALID_PARAMETER_VALUE", "MISSING_REQUIRED_PARAMETER", "INVALID_STRING_LENGTH", "INVALID_STRING_PATTERN":
					return nil, errRefundRejected
				}
			}
		}
		// Duplicate-request and processing errors are not proof that no money
		// moved; retain the original request for lookup or same-key recovery.
		return nil, ErrRefundUnconfirmed
	}
	return raw, nil
}
func (p *PayPalProvider) submitRefund(ctx context.Context, a refundAttempt) (gatewayRefund, error) {
	raw, err := p.refundHTTP(ctx, http.MethodPost, "/v2/payments/captures/"+url.PathEscape(a.PaymentRef)+"/refund", map[string]any{
		"amount": paypalMoney{Value: centsToDecimal(a.AmountCents), Currency: a.Currency}, "custom_id": a.ID,
	}, a.ID)
	if err != nil {
		return gatewayRefund{}, err
	}
	return decodePayPalRefund(raw)
}
func (p *PayPalProvider) lookupRefund(ctx context.Context, id string) (gatewayRefund, error) {
	if !validRefundReference(id) {
		return gatewayRefund{}, ErrMismatch
	}
	raw, err := p.refundHTTP(ctx, http.MethodGet, "/v2/payments/refunds/"+url.PathEscape(id), nil, "")
	if err != nil {
		return gatewayRefund{}, err
	}
	r, err := decodePayPalRefund(raw)
	if err == nil && r.ID != id {
		return gatewayRefund{}, ErrMismatch
	}
	return r, err
}
func decodePayPalRefund(raw []byte) (gatewayRefund, error) {
	var data struct {
		ID, Status string
		CustomID   string `json:"custom_id"`
		Amount     paypalMoney
		Breakdown  struct {
			Total paypalMoney `json:"total_refunded_amount"`
		} `json:"seller_payable_breakdown"`
		Links []struct {
			Href string `json:"href"`
			Rel  string `json:"rel"`
		}
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return gatewayRefund{}, err
	}
	amount, err := decimalToCents(data.Amount.Value)
	r := gatewayRefund{ID: data.ID, State: "pending", PaymentRef: captureIDFromLinks(data.Links), Currency: data.Amount.Currency, Amount: amount, RequestID: data.CustomID}
	if err != nil || r.ID == "" || r.PaymentRef == "" || r.Amount <= 0 || r.Currency == "" {
		return r, ErrMismatch
	}
	switch data.Status {
	case "COMPLETED":
		r.State = "succeeded"
	case "FAILED", "CANCELLED":
		r.State = "failed"
	case "PENDING":
	default:
		return r, ErrRefundUnconfirmed
	}
	if r.State == "succeeded" {
		r.TotalRefunded, err = decimalToCents(data.Breakdown.Total.Value)
		if err != nil || r.TotalRefunded < r.Amount || !strings.EqualFold(data.Breakdown.Total.Currency, r.Currency) {
			return r, ErrMismatch
		}
	}
	return r, nil
}
