// 店铺设置 HTTP / Shop Settings Handlers
// 功能：后台读取与保存店铺名称，并上传或删除店面 Logo
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-07 12:00:00
package admin

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/kartwo/kartwo/internal/settings"
)

func (h *HTTP) getShop(w http.ResponseWriter, r *http.Request) {
	fallback := "Kartwo Store"
	if strings.TrimSpace(h.envShopName) != "" {
		fallback = strings.TrimSpace(h.envShopName)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name": h.settings.ShopName(r.Context(), fallback), "source": "db", "readonly": false,
		"logo_url": h.settings.ShopLogoURL(r.Context()),
	})
}

func (h *HTTP) setShop(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := h.settings.SetShopName(r.Context(), strings.TrimSpace(req.Name)); err != nil {
		if errors.Is(err, settings.ErrInvalidShopName) {
			writeErr(w, http.StatusBadRequest, "店铺名称不能为空、不能含控制字符，且最多 120 个字符")
			return
		}
		writeErr(w, http.StatusInternalServerError, "保存店铺名称失败")
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "shop.settings_update", "settings", "shop")
	h.getShop(w, r)
}

func (h *HTTP) uploadShopLogo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, uploadMaxBytes)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "上传体过大或格式非法")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "缺少 file 字段")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(file)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "读取上传失败")
		return
	}
	logo, err := h.media.StoreBrandLogo(data)
	if err != nil {
		h.writeMediaErr(w, err)
		return
	}
	oldPath := h.settings.ShopLogoPath(r.Context())
	if err := h.settings.SetShopLogoPath(r.Context(), logo.Path); err != nil {
		_ = h.media.DeleteBrandLogo(logo.Path)
		writeErr(w, http.StatusInternalServerError, "保存 Logo 失败")
		return
	}
	if oldPath != "" && oldPath != logo.Path {
		_ = h.media.DeleteBrandLogo(oldPath)
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "shop.logo_upload", "settings", "shop-logo")
	h.getShop(w, r)
}

func (h *HTTP) deleteShopLogo(w http.ResponseWriter, r *http.Request) {
	oldPath := h.settings.ShopLogoPath(r.Context())
	if err := h.settings.SetShopLogoPath(r.Context(), ""); err != nil {
		writeErr(w, http.StatusInternalServerError, "删除 Logo 失败")
		return
	}
	if oldPath != "" {
		_ = h.media.DeleteBrandLogo(oldPath)
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "shop.logo_delete", "settings", "shop-logo")
	h.getShop(w, r)
}
