// 订单访问撤销 / Order Access Revocation
// 功能：店主可撤销指定订单的所有顾客凭证与验证码，保留审计且不改订单资金状态
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 17:18:40
package admin

import (
	"database/sql"
	"errors"
	"net/http"
)

func (h *HTTP) revokeOrderAccess(w http.ResponseWriter, r *http.Request) {
	// Require the actual owner as well as the normal authentication/CSRF guard.
	ac := authFrom(r.Context())
	if ac == nil || ac.Role != "owner" {
		writeErr(w, 403, "此操作仅店主可用")
		return
	}
	err := h.orders.RevokeAllGuestAccess(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "订单不存在")
		return
	}
	if err != nil {
		writeErr(w, 500, "撤销订单访问失败")
		return
	}
	h.recordAudit(r, ac.AdminID, "order.guest_access_revoked", "order", r.PathValue("id"))
	writeJSON(w, 200, map[string]bool{"ok": true})
}
