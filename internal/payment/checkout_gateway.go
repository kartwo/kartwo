// 收银台状态核验 / Checkout State Verification
// 功能：固定网关查询会话归属与金额，关闭未完成 Stripe 会话，读取 PayPal 审批和扣款状态
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 16:58:12
package payment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

type stripeCheckout struct {
	ID, Status, Currency string
	OrderRef             string `json:"client_reference_id"`
	PaymentStatus        string `json:"payment_status"`
	PaymentRef           string `json:"payment_intent"`
	Amount               int64  `json:"amount_total"`
}

func (p *StripeProvider) readCheckout(ctx context.Context, id string, ord OrderForPayment) (stripeCheckout, error) {
	var out stripeCheckout
	if !validRefundReference(id) {
		return out, ErrMismatch
	}
	raw, err := p.refundHTTP(ctx, http.MethodGet, "/v1/checkout/sessions/"+url.PathEscape(id), nil, "")
	if err != nil {
		return out, err
	}
	if json.Unmarshal(raw, &out) != nil || out.ID != id || out.OrderRef != ord.PublicID || out.Amount != ord.AmountCents || !strings.EqualFold(out.Currency, ord.Currency) {
		return stripeCheckout{}, ErrMismatch
	}
	return out, nil
}
func (p *StripeProvider) expireCheckout(ctx context.Context, id string, ord OrderForPayment) (stripeCheckout, error) {
	current, err := p.readCheckout(ctx, id, ord)
	if err != nil {
		return current, err
	}
	if current.Status == "expired" && current.PaymentStatus == "unpaid" {
		return current, nil
	}
	if current.Status != "open" || current.PaymentStatus != "unpaid" {
		return current, ErrPaymentProcessing
	}
	// Retry the same close key after response loss; always verify the actual state
	// afterwards, including a customer completing payment while we try to expire.
	_, closeErr := p.refundHTTP(ctx, http.MethodPost, "/v1/checkout/sessions/"+url.PathEscape(id)+"/expire", nil, "expire:"+ord.IdempotencyKey)
	current, err = p.readCheckout(ctx, id, ord)
	if err != nil {
		return current, err
	}
	if current.Status == "expired" && current.PaymentStatus == "unpaid" {
		return current, nil
	}
	if current.Status == "complete" || current.PaymentStatus == "paid" {
		return current, ErrPaymentProcessing
	}
	if closeErr != nil {
		return current, closeErr
	}
	return current, ErrPaymentProcessing
}

type paypalCheckoutCapture struct {
	ID, Status string
	Amount     paypalMoney
}
type paypalCheckout struct {
	ID, Status, OrderRef string
	Amount               int64
	Currency             string
	Captures             []paypalCheckoutCapture
}

func (p *PayPalProvider) inspectCheckout(ctx context.Context, id string, expected OrderForPayment) (paypalCheckout, error) {
	var out paypalCheckout
	if !validRefundReference(id) {
		return out, ErrMismatch
	}
	raw, err := p.refundHTTP(ctx, http.MethodGet, "/v2/checkout/orders/"+url.PathEscape(id), nil, "")
	if err != nil {
		return out, err
	}
	var data struct {
		ID, Status, Intent string
		Units              []struct {
			CustomID string `json:"custom_id"`
			Amount   paypalMoney
			Payments struct{ Captures []paypalCheckoutCapture }
		} `json:"purchase_units"`
	}
	if json.Unmarshal(raw, &data) != nil || data.ID != id || data.Intent != "CAPTURE" || len(data.Units) != 1 {
		return out, ErrMismatch
	}
	u := data.Units[0]
	amount, err := decimalToCents(u.Amount.Value)
	if err != nil || amount <= 0 || u.CustomID == "" || u.Amount.Currency == "" {
		return out, ErrMismatch
	}
	if expected.PublicID != "" && (u.CustomID != expected.PublicID || amount != expected.AmountCents || !strings.EqualFold(u.Amount.Currency, expected.Currency)) {
		return out, ErrMismatch
	}
	for _, c := range u.Payments.Captures {
		n, err := decimalToCents(c.Amount.Value)
		if err != nil || !validRefundReference(c.ID) || n != amount || !strings.EqualFold(c.Amount.Currency, u.Amount.Currency) {
			return out, ErrMismatch
		}
	}
	out = paypalCheckout{ID: data.ID, Status: data.Status, OrderRef: u.CustomID, Amount: amount, Currency: u.Amount.Currency, Captures: u.Payments.Captures}
	return out, nil
}
