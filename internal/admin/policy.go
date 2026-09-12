// 店铺政策资料 HTTP 接口 / Store Policy Profile Handlers
// 功能：后台读取和保存经营资料，并按资料生成标准英文页脚内容页
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-12 15:30:00
package admin

import (
	"errors"
	"net/http"

	"github.com/kartwo/kartwo/internal/policy"
)

func (h *HTTP) getPolicyProfile(w http.ResponseWriter, r *http.Request) {
	profile, configured, err := h.policy.Get(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取店铺政策资料失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"configured": configured, "profile": profile})
}

func (h *HTTP) setPolicyProfile(w http.ResponseWriter, r *http.Request) {
	var profile policy.Profile
	if !readJSON(w, r, &profile) {
		return
	}
	if err := h.policy.Save(r.Context(), profile); err != nil {
		if errors.Is(err, policy.ErrInvalid) {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, "保存店铺政策资料失败")
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "policy_profile.update", "setting", "store.policy_profile")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *HTTP) generateFooterPages(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Publish   bool `json:"publish"`
		Overwrite bool `json:"overwrite"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	result, err := h.policy.Generate(r.Context(), req.Publish, req.Overwrite)
	if err != nil {
		if errors.Is(err, policy.ErrInvalid) {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, "生成页脚内容失败")
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "content_page.footer_generate", "content_page", "standard-footer-pages")
	writeJSON(w, http.StatusOK, result)
}
