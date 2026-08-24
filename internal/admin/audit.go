// 审计日志 HTTP 接口 / Audit Log HTTP Handler
// 功能：向已登录管理员提供最近关键后台操作的只读追溯列表
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-24 00:20:00
package admin

import "net/http"

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
	for _, e := range events {
		out = append(out, map[string]string{
			"public_id": e.PublicID, "action": e.Action, "target_type": e.TargetType,
			"target_public_id": e.TargetPublicID, "created_at": e.CreatedAt,
			"admin_public_id": e.AdminPublicID, "admin_username": e.AdminUsername,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": out})
}
