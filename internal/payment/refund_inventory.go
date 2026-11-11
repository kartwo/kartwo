// 平台退款清单 / Provider Refund Inventory
// 功能：读取指定付款的完整退款清单，校验付款、币种、金额与分页边界，不发起资金操作
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 16:43:04
package payment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// A bounded inventory must fail closed, never treat a truncated list as complete.
const maxRefundInventory = 1000

type refundInventory struct {
	Refunds []gatewayRefund
	Total   int64
	Pending bool
}

func validateRefundInventory(rows []gatewayRefund, ref, currency string, amount int64) (refundInventory, error) {
	result := refundInventory{Refunds: rows}
	seen := make(map[string]bool)
	active := int64(0)
	for _, r := range rows {
		if !validRefundReference(r.ID) || seen[r.ID] || r.PaymentRef != ref || !strings.EqualFold(r.Currency, currency) || r.Amount <= 0 || r.Amount > amount {
			return result, ErrMismatch
		}
		seen[r.ID] = true
		if r.State == "succeeded" || r.State == "pending" {
			if r.Amount > amount-active {
				return result, ErrMismatch
			}
			active += r.Amount
		}
		switch r.State {
		case "succeeded":
			if r.Amount > amount-result.Total {
				return result, ErrMismatch
			}
			result.Total += r.Amount
		case "pending":
			result.Pending = true
		case "failed":
		default:
			return result, ErrRefundUnconfirmed
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return result, nil
}

func (s *Service) refundInventory(ctx context.Context, provider, ref, currency string, amount int64) (refundInventory, error) {
	if !validRefundReference(ref) {
		return refundInventory{}, ErrMismatch
	}
	var rows []gatewayRefund
	var err error
	switch provider {
	case "stripe":
		rows, err = s.stripe.listRefunds(ctx, ref, currency, amount)
	case "paypal":
		rows, err = s.paypal.listRefunds(ctx, ref, currency, amount)
	default:
		return refundInventory{}, ErrMismatch
	}
	if err != nil {
		return refundInventory{}, err
	}
	return validateRefundInventory(rows, ref, currency, amount)
}

func (p *StripeProvider) listRefunds(ctx context.Context, ref, currency string, amount int64) ([]gatewayRefund, error) {
	raw, err := p.refundHTTP(ctx, http.MethodGet, "/v1/payment_intents/"+url.PathEscape(ref), nil, "")
	if err != nil {
		return nil, err
	}
	var intent struct {
		ID, Currency, Status string
		Received             int64 `json:"amount_received"`
	}
	if json.Unmarshal(raw, &intent) != nil || intent.ID != ref || intent.Received != amount || !strings.EqualFold(intent.Currency, currency) || intent.Status != "succeeded" {
		return nil, ErrMismatch
	}
	rows := make([]gatewayRefund, 0)
	cursor := ""
	seen := make(map[string]bool)
	for len(rows) < maxRefundInventory {
		q := url.Values{"payment_intent": {ref}, "limit": {"100"}}
		if cursor != "" {
			q.Set("starting_after", cursor)
		}
		raw, err = p.refundHTTP(ctx, http.MethodGet, "/v1/refunds?"+q.Encode(), nil, "")
		if err != nil {
			return nil, err
		}
		var page struct {
			Object string
			Data   []json.RawMessage
			More   *bool `json:"has_more"`
		}
		if json.Unmarshal(raw, &page) != nil || page.Object != "list" || page.More == nil || len(page.Data) > 100 {
			return nil, ErrMismatch
		}
		for _, item := range page.Data {
			r, _, err := parseStripeRefund(item)
			if err != nil {
				return nil, err
			}
			if seen[r.ID] {
				return nil, ErrMismatch
			}
			seen[r.ID] = true
			rows = append(rows, r)
			cursor = r.ID
		}
		if !*page.More {
			return rows, nil
		}
		if len(page.Data) == 0 || !validRefundReference(cursor) {
			return nil, ErrMismatch
		}
	}
	return nil, ErrRefundUnconfirmed
}

func (p *PayPalProvider) listRefunds(ctx context.Context, ref, currency string, amount int64) ([]gatewayRefund, error) {
	raw, err := p.refundHTTP(ctx, http.MethodGet, "/v2/payments/captures/"+url.PathEscape(ref), nil, "")
	if err != nil {
		return nil, err
	}
	var capture struct {
		ID, Status    string
		Amount        paypalMoney
		Supplementary struct {
			Related struct {
				OrderID string `json:"order_id"`
			} `json:"related_ids"`
		} `json:"supplementary_data"`
	}
	if json.Unmarshal(raw, &capture) != nil {
		return nil, ErrMismatch
	}
	received, err := decimalToCents(capture.Amount.Value)
	orderID := capture.Supplementary.Related.OrderID
	if err != nil || capture.ID != ref || received != amount || !strings.EqualFold(capture.Amount.Currency, currency) || !validRefundReference(orderID) {
		return nil, ErrMismatch
	}
	switch capture.Status {
	case "COMPLETED", "PARTIALLY_REFUNDED", "REFUNDED":
	default:
		return nil, ErrRefundUnconfirmed
	}
	raw, err = p.refundHTTP(ctx, http.MethodGet, "/v2/checkout/orders/"+url.PathEscape(orderID), nil, "")
	if err != nil {
		return nil, err
	}
	var order struct {
		ID    string
		Units []struct {
			Payments struct {
				Captures []struct {
					ID     string
					Amount paypalMoney
				}
				Refunds []json.RawMessage
			}
		} `json:"purchase_units"`
	}
	if json.Unmarshal(raw, &order) != nil || order.ID != orderID {
		return nil, ErrMismatch
	}
	rows := make([]gatewayRefund, 0)
	matches := 0
	for _, unit := range order.Units {
		belongs := false
		for _, c := range unit.Payments.Captures {
			if c.ID == ref {
				n, err := decimalToCents(c.Amount.Value)
				if err != nil || n != amount || !strings.EqualFold(c.Amount.Currency, currency) {
					return nil, ErrMismatch
				}
				belongs = true
				matches++
			}
		}
		if !belongs {
			continue
		}
		if len(unit.Payments.Refunds) > maxRefundInventory {
			return nil, ErrRefundUnconfirmed
		}
		for _, item := range unit.Payments.Refunds {
			r, err := parsePayPalRefund(item, false)
			if err != nil {
				return nil, err
			}
			// A purchase unit may contain multiple captures; bind each refund via its
			// capture link, never assume every refund belongs to the selected receipt.
			if r.PaymentRef == ref {
				rows = append(rows, r)
			}
		}
	}
	if matches != 1 || len(rows) > maxRefundInventory {
		return nil, ErrMismatch
	}
	// A refunded capture without any refund history is not proof of zero refunds.
	if capture.Status != "COMPLETED" && len(rows) == 0 {
		return nil, ErrRefundUnconfirmed
	}
	return rows, nil
}
