// 媒体上传 HTTP 接口 / Media Upload Handlers
// 功能：商品图片上传/列表/删除（需鉴权，写操作经 CSRF 中间件）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-18 09:48:36
package admin

import (
	"errors"
	"io"
	"net/http"

	"github.com/kartwo/kartwo/internal/catalog"
	"github.com/kartwo/kartwo/internal/media"
)

// uploadMaxBytes 限制 multipart 请求体上限（略大于单文件上限，留 multipart 头余量）。
const uploadMaxBytes = 12 << 20 // 12 MiB

func (h *HTTP) uploadMedia(w http.ResponseWriter, r *http.Request) {
	ac := authFrom(r.Context())
	if ac.Role == "demo" {
		owned, err := h.demoOwnsProduct(r.Context(), ac.SessionToken, r.PathValue("id"))
		if err != nil || !owned {
			writeErr(w, http.StatusForbidden, "公开演示只能给自己新建的商品上传图片")
			return
		}
	}
	productID, err := h.cat.ProductIDByPublicID(r.Context(), r.PathValue("id"))
	if err != nil {
		h.writeCatalogErr(w, err)
		return
	}

	bodyLimit := int64(uploadMaxBytes)
	if ac.Role == "demo" {
		bodyLimit = h.demo.MaxImageBytes + (1 << 20)
	}
	r.Body = http.MaxBytesReader(w, r.Body, bodyLimit)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "上传体过大或格式非法")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "缺少 file 字段")
		return
	}
	defer func() { _ = file.Close() }()
	if ac.Role == "demo" {
		if header.Size > h.demo.MaxImageBytes {
			writeErr(w, http.StatusRequestEntityTooLarge, "演示商品图片最大 2MB")
			return
		}
		existing, listErr := h.media.ListByProduct(r.Context(), productID)
		if listErr != nil {
			writeErr(w, http.StatusInternalServerError, "内部错误")
			return
		}
		if len(existing) >= 1 {
			writeErr(w, http.StatusConflict, "每个演示商品最多上传 1 张图片")
			return
		}
	}
	data, err := io.ReadAll(file)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "读取上传失败")
		return
	}
	if ac.Role == "demo" && int64(len(data)) > h.demo.MaxImageBytes {
		writeErr(w, http.StatusRequestEntityTooLarge, "演示商品图片最大 2MB")
		return
	}

	asset, err := h.media.Upload(r.Context(), productID, data)
	if err != nil {
		h.writeMediaErr(w, err)
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "media.upload", "media", asset.PublicID)
	writeJSON(w, http.StatusCreated, mediaAssetJSON(asset))
}

func (h *HTTP) listMedia(w http.ResponseWriter, r *http.Request) {
	if ac := authFrom(r.Context()); ac.Role == "demo" {
		allowed, err := h.demoCanViewProduct(r.Context(), ac.SessionToken, r.PathValue("id"))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "内部错误")
			return
		}
		if !allowed {
			writeErr(w, http.StatusNotFound, "商品不存在")
			return
		}
	}
	productID, err := h.cat.ProductIDByPublicID(r.Context(), r.PathValue("id"))
	if err != nil {
		h.writeCatalogErr(w, err)
		return
	}
	assets, err := h.media.ListByProduct(r.Context(), productID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "内部错误")
		return
	}
	out := make([]map[string]any, 0, len(assets))
	for i := range assets {
		out = append(out, mediaAssetJSON(&assets[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"media": out})
}

func (h *HTTP) deleteMedia(w http.ResponseWriter, r *http.Request) {
	mediaID := r.PathValue("id")
	if ac := authFrom(r.Context()); ac.Role == "demo" {
		owned, err := h.demoOwnsMedia(r.Context(), ac.SessionToken, mediaID)
		if err != nil || !owned {
			writeErr(w, http.StatusForbidden, "公开演示不能修改示例商品图片")
			return
		}
	}
	if err := h.media.Delete(r.Context(), mediaID); err != nil {
		h.writeMediaErr(w, err)
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "media.delete", "media", mediaID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *HTTP) updateMediaAlt(w http.ResponseWriter, r *http.Request) {
	if ac := authFrom(r.Context()); ac.Role == "demo" {
		owned, err := h.demoOwnsMedia(r.Context(), ac.SessionToken, r.PathValue("id"))
		if err != nil || !owned {
			writeErr(w, http.StatusForbidden, "公开演示不能修改示例商品图片")
			return
		}
	}
	var req struct {
		AltText string `json:"alt_text"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := h.media.UpdateAlt(r.Context(), r.PathValue("id"), req.AltText); err != nil {
		h.writeMediaErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func mediaAssetJSON(a *media.Asset) map[string]any {
	derivs := make([]map[string]any, 0, len(a.Derivatives))
	for _, d := range a.Derivatives {
		derivs = append(derivs, map[string]any{"label": d.Label, "url": d.URL, "width": d.Width, "height": d.Height})
	}
	return map[string]any{
		"public_id": a.PublicID, "mime": a.Mime, "width": a.Width, "height": a.Height,
		"size_bytes": a.SizeBytes, "alt_text": a.AltText, "original_url": a.OriginalURL, "derivatives": derivs,
	}
}

// writeMediaErr 映射媒体领域错误到 HTTP 状态。
func (h *HTTP) writeMediaErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, media.ErrUnsupportedType):
		writeErr(w, http.StatusUnsupportedMediaType, "仅支持 JPEG/PNG/WebP 图片")
	case errors.Is(err, media.ErrFileTooLarge):
		writeErr(w, http.StatusRequestEntityTooLarge, "文件过大")
	case errors.Is(err, media.ErrTooManyPerProduct):
		writeErr(w, http.StatusConflict, "该商品图片数量已达上限")
	case errors.Is(err, media.ErrDiskFull):
		writeErr(w, http.StatusInsufficientStorage, "磁盘空间不足，暂停新上传")
	case errors.Is(err, media.ErrNotFound):
		writeErr(w, http.StatusNotFound, "资源不存在")
	case errors.Is(err, media.ErrAltTooLong):
		writeErr(w, http.StatusBadRequest, "图片 alt 文本最多 250 个字符")
	case errors.Is(err, catalog.ErrNotFound):
		writeErr(w, http.StatusNotFound, "商品不存在")
	default:
		writeErr(w, http.StatusInternalServerError, "内部错误")
	}
}
