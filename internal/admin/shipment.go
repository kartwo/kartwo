// 后台运单与通知 / Admin Shipment and Notices
// 功能：受权限和 CSRF 保护的版本历史、更正与人工投递核对，不返回邮件正文或服务器原始错误
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 15:48:40
package admin

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/kartwo/kartwo/internal/order"
)

func (h *HTTP) getShipment(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	history, err := h.orders.ShipmentHistory(ctx, r.PathValue("id"))
	if err != nil {
		shipmentError(w, err)
		return
	}
	versions := make([]map[string]any, 0, len(history.Revisions))
	notices := make([]map[string]any, 0, len(history.Notices))
	for _, revision := range history.Revisions {
		number, link := revision.Number, revision.URL
		if orderCustomerDataHidden(r) {
			number = "已隐藏"
			link = ""
		}
		versions = append(versions, map[string]any{"public_id": revision.PublicID, "version": revision.Version, "carrier": revision.Carrier, "tracking_number": number, "tracking_url": link, "legacy": revision.Legacy, "created_at": revision.CreatedAt})
	}
	for _, notice := range history.Notices {
		notices = append(notices, map[string]any{"public_id": notice.PublicID, "revision_id": notice.RevisionID, "version": notice.Version, "sequence": notice.Sequence, "status": notice.Status, "attempts": notice.Attempts, "origin": notice.Origin, "resolution": notice.Resolution, "reason": notice.Reason, "created_at": notice.CreatedAt, "next_attempt_at": notice.NextAttemptAt, "sent_at": notice.SentAt, "resolved_at": notice.ResolvedAt})
	}
	if history.HasShipment && !orderCustomerDataHidden(r) {
		h.recordAudit(r, authFrom(r.Context()).AdminID, "order.customer_data_viewed", "order", r.PathValue("id"))
	}
	writeJSON(w, http.StatusOK, map[string]any{"order_id": r.PathValue("id"), "has_shipment": history.HasShipment, "current_revision": history.CurrentRevision, "current_version": history.CurrentVersion, "shipped_at": history.ShippedAt, "revision_count": history.RevisionCount, "notice_count": history.NoticeCount, "revisions": versions, "notices": notices})
}

func (h *HTTP) updateShipment(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ExpectedRevision *string `json:"expected_revision"`
		Carrier          string  `json:"carrier"`
		Number           string  `json:"tracking_number"`
		URL              string  `json:"tracking_url"`
	}
	if !readJSON(w, r, &request) {
		return
	}
	if request.ExpectedRevision == nil {
		writeErr(w, http.StatusBadRequest, "缺少当前运单版本，请刷新核对")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := h.orders.UpdateShipment(ctx, r.PathValue("id"), *request.ExpectedRevision, request.Carrier, request.Number, request.URL); err != nil {
		shipmentError(w, err)
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "order.shipment_updated", "order", r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *HTTP) manageShipmentNotice(w http.ResponseWriter, r *http.Request) {
	var request struct {
		RequestKey     string `json:"request_key"`
		Action         string `json:"action"`
		ConfirmUnknown bool   `json:"confirm_unknown"`
	}
	if !readJSON(w, r, &request) {
		return
	}
	if (request.Action == "resend_after_check" || request.Action == "received") && !request.ConfirmUnknown {
		writeErr(w, http.StatusBadRequest, "投递结果未知，请先核对是否已收到邮件再确认")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	id, err := h.orders.ManageShipmentNotice(ctx, r.PathValue("id"), r.PathValue("notice"), request.RequestKey, request.Action)
	if err != nil {
		shipmentError(w, err)
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "order.shipment_notice."+request.Action, "order", r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "notice_id": id, "action": request.Action})
}

func shipmentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, http.StatusNotFound, "订单、运单或通知不存在")
	case errors.Is(err, order.ErrInvalidInfo):
		writeErr(w, http.StatusBadRequest, "运单或通知参数无效，请检查填写内容后重试")
	case errors.Is(err, order.ErrShipmentChanged), errors.Is(err, order.ErrShipmentNoticeChanged):
		writeErr(w, http.StatusConflict, "运单或通知已变化，请刷新核对后再操作")
	case errors.Is(err, order.ErrShipmentSending):
		writeErr(w, http.StatusConflict, "发货通知正在投递，请稍后刷新核对")
	case errors.Is(err, order.ErrShipmentNoticeCooldown):
		w.Header().Set("Retry-After", "60")
		writeErr(w, http.StatusTooManyRequests, "请至少间隔一分钟再重试通知")
	default:
		writeErr(w, http.StatusInternalServerError, "运单或通知结果未确认，请刷新核对后重试")
	}
}
