// 后台订单 HTTP / Admin Orders Handlers
// 功能：订单列表/详情 + 整单全额退款（鉴权+CSRF+对象级）；退款经支付编排，订单转 refunded
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-22 11:20:11
package admin

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/kartwo/kartwo/internal/payment"
)

func (h *HTTP) listOrders(w http.ResponseWriter, r *http.Request) {
	rows, err := h.orders.AdminList(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, o := range rows {
		out = append(out, map[string]any{
			"public_id": o.PublicID, "status": o.Status, "email": o.Email,
			"currency": o.Currency, "total_cents": o.TotalCents,
			"payment_provider": o.PaymentProvider, "created_at": o.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"orders": out})
}

func (h *HTTP) getOrder(w http.ResponseWriter, r *http.Request) {
	o, err := h.orders.AdminGet(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "订单不存在")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	lines := make([]map[string]any, 0, len(o.Lines))
	for _, l := range o.Lines {
		lines = append(lines, map[string]any{
			"title": l.Title, "spec": l.Spec, "sku": l.SKU,
			"unit_cents": l.UnitCents, "quantity": l.Quantity, "line_cents": l.LineCents,
		})
	}
	refunds := make([]map[string]any, 0, len(o.Refunds))
	for _, rf := range o.Refunds {
		refunds = append(refunds, map[string]any{
			"provider": rf.Provider, "provider_refund_id": rf.ProviderRefundID,
			"amount_cents": rf.AmountCents, "created_at": rf.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"public_id": o.PublicID, "status": o.Status, "email": o.Email,
		"ship_name": o.ShipName, "ship_phone": o.ShipPhone, "ship_address": o.ShipAddress, "ship_country": o.ShipCountry,
		"currency": o.Currency, "subtotal_cents": o.SubtotalCents, "total_cents": o.TotalCents,
		"payment_provider": o.PaymentProvider, "created_at": o.CreatedAt,
		"lines": lines, "refunds": refunds,
	})
}

func (h *HTTP) refundOrder(w http.ResponseWriter, r *http.Request) {
	if h.pay == nil {
		writeErr(w, http.StatusInternalServerError, "支付未装配")
		return
	}
	err := h.pay.Refund(r.Context(), r.PathValue("id"))
	switch {
	case err == nil:
		o, _ := h.orders.AdminGet(r.Context(), r.PathValue("id"))
		status := "refunded"
		if o != nil {
			status = o.Status
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": status})
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, http.StatusNotFound, "订单不存在")
	case errors.Is(err, payment.ErrNotRefundable):
		writeErr(w, http.StatusConflict, "订单当前状态不可退款（仅已付订单可退）")
	case errors.Is(err, payment.ErrLocked):
		writeErr(w, http.StatusServiceUnavailable, "收款密钥未解锁，请重新登录后重试")
	default:
		// 退款失败（如网关拒绝）：admin 场景下回传原因便于排查。
		writeErr(w, http.StatusBadGateway, "退款失败："+err.Error())
	}
}
