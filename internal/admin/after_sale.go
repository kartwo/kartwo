// 后台售后接口 / Admin After Sale HTTP
// 功能：客户资料授权后的申请历史及版本化审核，不触发网关退款或库存写入
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 21:24:50
package admin

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/kartwo/kartwo/internal/order"
)

func (h *HTTP) getAfterSales(w http.ResponseWriter, r *http.Request) {
	if orderCustomerDataHidden(r) {
		writeErr(w, http.StatusForbidden, "查看售后需要订单和客户信息权限")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	snapshot, err := h.orders.AdminAfterSales(ctx, r.PathValue("id"))
	if err != nil {
		afterSaleError(w, err)
		return
	}
	if len(snapshot.Cases) > 0 {
		h.recordAudit(r, authFrom(r.Context()).AdminID, "order.aftersales_viewed", "order", r.PathValue("id"))
	}
	// Read-only nonce generation avoids another endpoint and keeps UUIDv7 keys
	// available to all clients, including browsers without a UUIDv7 library.
	writeJSON(w, http.StatusOK, map[string]any{"order_id": r.PathValue("id"), "snapshot": snapshot, "next_action_key": uuid.Must(uuid.NewV7()).String()})
}

func (h *HTTP) reviewAfterSale(w http.ResponseWriter, r *http.Request) {
	if orderCustomerDataHidden(r) {
		writeErr(w, http.StatusForbidden, "处理售后需要订单和客户信息权限")
		return
	}
	var input struct {
		RequestKey      string `json:"request_key"`
		ExpectedVersion int64  `json:"expected_version"`
		Action          string `json:"action"`
		Message         string `json:"message"`
		RefundRequestID string `json:"refund_request_id"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	result, err := h.orders.ReviewAfterSale(ctx, r.PathValue("id"), r.PathValue("case"), order.AfterSaleAction{
		Key: input.RequestKey, ExpectedVersion: input.ExpectedVersion, Action: input.Action, Message: input.Message, RefundRequestID: input.RefundRequestID,
	})
	if err != nil {
		afterSaleError(w, err)
		return
	}
	if result.OperationApplied {
		h.recordAudit(r, authFrom(r.Context()).AdminID, "order.after_sale."+input.Action, "after_sale", result.PublicID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "order_id": r.PathValue("id"), "case": result})
}

func afterSaleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, http.StatusNotFound, "订单或售后申请不存在")
	case errors.Is(err, order.ErrAfterSaleInvalid):
		writeErr(w, http.StatusBadRequest, "售后参数无效，请检查说明、版本和原请求后重试")
	case errors.Is(err, order.ErrAfterSaleChanged):
		writeErr(w, http.StatusConflict, "售后状态或退款证据已变化，请刷新核对；原请求结果未知时继续原请求")
	case errors.Is(err, order.ErrAfterSaleLimit):
		writeErr(w, http.StatusConflict, "售后记录达到上限，请联系店主核对")
	default:
		writeErr(w, http.StatusInternalServerError, "售后操作结果未确认，请保留原请求并继续核对")
	}
}
