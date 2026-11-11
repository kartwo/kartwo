// 后台订单查询 / Admin Order Search
// 功能：鉴权后从请求体读取筛选条件，限制客户搜索权限，返回工作台分页及最小访问审计
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 13:42:16
package admin

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/kartwo/kartwo/internal/order"
)

func (h *HTTP) searchOrders(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Term     string `json:"term"`
		View     string `json:"view"`
		Status   string `json:"status"`
		Provider string `json:"provider"`
		From     string `json:"from"`
		To       string `json:"to"`
		Page     int64  `json:"page"`
		PageSize int64  `json:"page_size"`
	}
	if !readJSON(w, r, &request) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	customers := !orderCustomerDataHidden(r)
	result, err := h.orders.AdminSearch(ctx, order.OrderSearchFilter{Term: request.Term, View: request.View, Status: request.Status, Provider: request.Provider, From: request.From, To: request.To, Page: request.Page, PageSize: request.PageSize}, customers)
	if errors.Is(err, order.ErrInvalidOrderSearch) {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "查询订单失败，请缩小筛选范围后重试")
		return
	}
	rows := make([]map[string]any, 0, len(result.Orders))
	for _, o := range result.Orders {
		rows = append(rows, map[string]any{"public_id": o.PublicID, "status": o.Status, "email": o.Email, "currency": o.Currency, "total_cents": o.TotalCents, "payment_provider": o.PaymentProvider, "created_at": o.CreatedAt,
			"shipped": o.Shipped, "refund_pending": o.RefundPending, "needs_review": o.NeedsReview, "ready_to_fulfill": o.ReadyToFulfill})
	}
	if customers && len(rows) > 0 {
		h.recordAudit(r, authFrom(r.Context()).AdminID, "order.customer_list_viewed", "orders", "orders")
	}
	writeJSON(w, http.StatusOK, map[string]any{"orders": rows, "counts": result.Counts, "total": result.Total, "page": result.Page, "page_size": result.PageSize, "view": result.View})
}
