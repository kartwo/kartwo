// 后台退款请求 / Admin Refund Requests
// 功能：店主按付款及金额退款、重试与核对；复用鉴权、CSRF、对象级校验和审计
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 22:35:00
package admin

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/kartwo/kartwo/internal/payment"
)

type refundRequestService interface {
	RequestRefund(context.Context, string, payment.RefundInput) (payment.RefundResult, error)
	RetryRefund(context.Context, string, string, string) (payment.RefundResult, error)
}

func (h *HTTP) createRefundRequest(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.pay.(refundRequestService)
	if !ok {
		writeErr(w, 503, "退款服务未就绪")
		return
	}
	var in payment.RefundInput
	if !readJSON(w, r, &in) {
		return
	}
	result, err := svc.RequestRefund(r.Context(), r.PathValue("id"), in)
	h.refundRequestResponse(w, r, result, err)
}

func (h *HTTP) retryRefundRequest(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.pay.(refundRequestService)
	if !ok {
		writeErr(w, 503, "退款服务未就绪")
		return
	}
	var in struct {
		GatewayRef string `json:"gateway_ref"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	result, err := svc.RetryRefund(r.Context(), r.PathValue("id"), r.PathValue("refund"), in.GatewayRef)
	h.refundRequestResponse(w, r, result, err)
}

func (h *HTTP) refundRequestResponse(w http.ResponseWriter, r *http.Request, result payment.RefundResult, err error) {
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeErr(w, 404, "订单或退款请求不存在")
		case errors.Is(err, payment.ErrRefundBusy), errors.Is(err, payment.ErrRefundConflict), errors.Is(err, payment.ErrMismatch):
			writeErr(w, 409, "退款参数或余额已变化，或已有请求正在处理；请刷新订单核对")
		case errors.Is(err, payment.ErrPaymentConfigChanged):
			writeErr(w, 409, "收款配置已变更，请恢复原配置后核对这笔退款")
		case errors.Is(err, payment.ErrLocked):
			writeErr(w, 503, "收款密钥未解锁")
		default:
			writeErr(w, 502, "退款结果尚未确认，请刷新订单并继续原退款请求")
		}
		return
	}
	if result.Created {
		h.recordAudit(r, authFrom(r.Context()).AdminID, "order.refund_requested", "order", r.PathValue("id"))
	}
	if result.Completed {
		h.recordAudit(r, authFrom(r.Context()).AdminID, "order.refund", "order", r.PathValue("id"))
	}
	status := http.StatusOK
	if result.State == "pending" || result.State == "prepared" || result.State == "review" {
		status = http.StatusAccepted
	}
	writeJSON(w, status, result)
}
