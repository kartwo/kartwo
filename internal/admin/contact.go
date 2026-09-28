// 后台客服留言 / Admin Contact Inbox
// 功能：受权限保护的客服收件箱、处理状态与店主留言配置
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-28 14:20:00
package admin

import (
	"errors"
	"net/http"

	"github.com/kartwo/kartwo/internal/contact"
)

func (h *HTTP) getContactSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.contact.Config(r.Context())
	if err != nil {
		writeErr(w, 500, "读取客服设置失败")
		return
	}
	writeJSON(w, 200, cfg)
}

func (h *HTTP) setContactSettings(w http.ResponseWriter, r *http.Request) {
	var cfg contact.Config
	if !readJSON(w, r, &cfg) {
		return
	}
	if err := h.contact.SaveConfig(r.Context(), cfg); err != nil {
		if errors.Is(err, contact.ErrInvalid) {
			writeErr(w, 400, "完整留言保留月数须为 1–24；IP 保留天数须为 7–90")
			return
		}
		writeErr(w, 500, "保存客服设置失败")
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "contact.settings_update", "setting", "contact")
	writeJSON(w, 200, cfg)
}

func (h *HTTP) listContactMessages(w http.ResponseWriter, r *http.Request) {
	if isDemoRequest(r) {
		writeJSON(w, 200, map[string]any{"messages": []contact.Message{}, "demo_hidden": true})
		return
	}
	rows, err := h.contact.List(r.Context(), r.URL.Query().Get("status"), 100)
	if err != nil {
		if errors.Is(err, contact.ErrInvalid) {
			writeErr(w, 400, "状态筛选值无效")
			return
		}
		writeErr(w, 500, "读取留言失败")
		return
	}
	writeJSON(w, 200, map[string]any{"messages": rows})
}

func (h *HTTP) getContactMessage(w http.ResponseWriter, r *http.Request) {
	if isDemoRequest(r) {
		writeErr(w, 403, "公开演示不提供真实留言")
		return
	}
	m, err := h.contact.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, contact.ErrNotFound) {
			writeErr(w, 404, "留言不存在")
			return
		}
		writeErr(w, 500, "读取留言失败")
		return
	}
	writeJSON(w, 200, m)
}

func (h *HTTP) setContactMessageStatus(w http.ResponseWriter, r *http.Request) {
	if isDemoRequest(r) {
		writeErr(w, 403, "公开演示不可修改留言")
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	ac := authFrom(r.Context())
	if err := h.contact.SetStatus(r.Context(), r.PathValue("id"), req.Status, contact.Actor{PublicID: ac.AdminPublicID, Username: ac.Username}); err != nil {
		if errors.Is(err, contact.ErrInvalid) {
			writeErr(w, 400, "留言状态无效")
			return
		}
		if errors.Is(err, contact.ErrUnchanged) {
			writeErr(w, 409, "留言已是该状态，请刷新页面")
			return
		}
		if errors.Is(err, contact.ErrNotFound) {
			writeErr(w, 404, "留言不存在或已匿名化")
			return
		}
		writeErr(w, 500, "更新留言失败")
		return
	}
	h.recordAudit(r, ac.AdminID, "contact.status_update", "contact_message", r.PathValue("id"))
	h.getContactMessage(w, r)
}
