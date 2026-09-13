// 审计日志 HTTP 接口 / Audit Log HTTP Handler
// 功能：向已登录管理员提供最近关键后台操作的只读追溯列表
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-24 00:20:00
package admin

import (
	"net/http"
	"strconv"
)

func (h *HTTP) listAuditEvents(w http.ResponseWriter, r *http.Request) {
	if h.audit == nil {
		writeErr(w, http.StatusServiceUnavailable, "审计服务未装配")
		return
	}
	events, err := h.audit.List(r.Context(), 100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取审计日志失败")
		return
	}
	out := make([]map[string]string, 0, len(events))
	for i, e := range events {
		publicID, targetID, adminID, username := e.PublicID, e.TargetPublicID, e.AdminPublicID, e.AdminUsername
		if isDemoRequest(r) {
			publicID, targetID, adminID, username = "demo-event-"+strconv.Itoa(i+1), "已隐藏", "", "店主"
		}
		out = append(out, map[string]string{
			"public_id": publicID, "action": e.Action, "target_type": e.TargetType,
			"target_public_id": targetID, "created_at": e.CreatedAt,
			"admin_public_id": adminID, "admin_username": username,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": out})
}
