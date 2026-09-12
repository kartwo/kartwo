// 内容与精选 HTTP 接口 / Content and Merchandising Handlers
// 功能：管理员管理首页精选和受限 Markdown 内容页
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-07 12:00:00
package admin

import (
	"net/http"

	"github.com/kartwo/kartwo/internal/catalog"
)

func (h *HTTP) setProductFeatured(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Featured bool `json:"featured"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := h.cat.SetProductFeatured(r.Context(), r.PathValue("id"), req.Featured); err != nil {
		h.writeCatalogErr(w, err)
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "product.featured_update", "product", r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type contentPageReq struct {
	Title          string `json:"title"`
	Slug           string `json:"slug"`
	BodyMarkdown   string `json:"body_markdown"`
	SEODescription string `json:"seo_description"`
	Status         string `json:"status"`
}

func contentJSON(p catalog.ContentPage) map[string]any {
	return map[string]any{"public_id": p.PublicID, "title": p.Title, "slug": p.Slug, "body_markdown": p.BodyMarkdown, "seo_description": p.SEODescription, "status": p.Status, "updated_at": p.UpdatedAt}
}
func (h *HTTP) listContentPages(w http.ResponseWriter, r *http.Request) {
	ps, err := h.cat.ListContentPages(r.Context())
	if err != nil {
		writeErr(w, 500, "内部错误")
		return
	}
	out := make([]map[string]any, 0, len(ps))
	for _, p := range ps {
		out = append(out, contentJSON(p))
	}
	writeJSON(w, 200, map[string]any{"pages": out})
}
func (h *HTTP) getContentPage(w http.ResponseWriter, r *http.Request) {
	p, err := h.cat.GetContentPage(r.Context(), r.PathValue("id"))
	if err != nil {
		h.writeCatalogErr(w, err)
		return
	}
	writeJSON(w, 200, contentJSON(*p))
}
func (h *HTTP) createContentPage(w http.ResponseWriter, r *http.Request) {
	var req contentPageReq
	if !readJSON(w, r, &req) {
		return
	}
	id, err := h.cat.CreateContentPage(r.Context(), req.Title, req.Slug, req.BodyMarkdown, req.SEODescription, req.Status)
	if err != nil {
		h.writeCatalogErr(w, err)
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "content_page.create", "content_page", id)
	writeJSON(w, 201, map[string]any{"public_id": id})
}
func (h *HTTP) updateContentPage(w http.ResponseWriter, r *http.Request) {
	var req contentPageReq
	if !readJSON(w, r, &req) {
		return
	}
	if err := h.cat.UpdateContentPage(r.Context(), r.PathValue("id"), req.Title, req.BodyMarkdown, req.SEODescription, req.Status); err != nil {
		h.writeCatalogErr(w, err)
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "content_page.update", "content_page", r.PathValue("id"))
	writeJSON(w, 200, map[string]any{"ok": true})
}
func (h *HTTP) deleteContentPage(w http.ResponseWriter, r *http.Request) {
	if err := h.cat.DeleteContentPage(r.Context(), r.PathValue("id")); err != nil {
		h.writeCatalogErr(w, err)
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "content_page.delete", "content_page", r.PathValue("id"))
	writeJSON(w, 200, map[string]any{"ok": true})
}
