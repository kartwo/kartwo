// 博客店面 / Blog Storefront
// 功能：按开关展示已发布文章列表与详情，并生成 SEO 信息
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-13 10:00:00
package storefront

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/kartwo/kartwo/internal/blog"
)

func (h *HTTP) blogList(w http.ResponseWriter, r *http.Request) {
	enabled, err := h.blog.Enabled(r.Context())
	if err != nil {
		http.Error(w, "Something went wrong", 500)
		return
	}
	if !enabled {
		http.NotFound(w, r)
		return
	}
	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		page, err = strconv.Atoi(raw)
		if err != nil || page < 1 || page > 100000 {
			http.NotFound(w, r)
			return
		}
	}
	posts, total, err := h.blog.ListPublished(r.Context(), page)
	if err != nil {
		http.Error(w, "Something went wrong", 500)
		return
	}
	if page > 1 && len(posts) == 0 {
		http.NotFound(w, r)
		return
	}
	shop := h.shopName(r.Context())
	canonical := h.base(r) + "/blog"
	if page > 1 {
		canonical += "?page=" + strconv.Itoa(page)
	}
	h.render(w, r, h.blogListTmpl, map[string]any{"ShopName": shop, "Posts": posts, "PageNumber": page, "Previous": page - 1, "Next": page + 1, "HasPrevious": page > 1, "HasNext": page*10 < total,
		"SEO": seo{Title: "Blog — " + shop, Description: "Stories and updates from " + shop, Canonical: canonical, OGType: "website", NoIndex: total == 0}})
}

func (h *HTTP) blogPost(w http.ResponseWriter, r *http.Request) {
	enabled, err := h.blog.Enabled(r.Context())
	if err != nil {
		http.Error(w, "Something went wrong", 500)
		return
	}
	if !enabled {
		http.NotFound(w, r)
		return
	}
	p, err := h.blog.GetPublished(r.Context(), r.PathValue("slug"))
	if errors.Is(err, blog.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "Something went wrong", 500)
		return
	}
	shop := h.shopName(r.Context())
	image := ""
	if p.CoverURL() != "" {
		image = h.base(r) + p.CoverURL()
	}
	desc := p.SEODescription
	if desc == "" {
		desc = p.Excerpt
	}
	if desc == "" {
		desc = p.Title
	}
	body := blog.RenderMarkdown(p.BodyMarkdown)
	if p.BodyHTML != "" {
		body = blog.RenderHTML(p.BodyHTML)
	}
	h.render(w, r, h.blogPostTmpl, map[string]any{"ShopName": shop, "Post": p, "Body": body,
		"SEO": seo{Title: p.Title + " — " + shop, Description: desc, Canonical: h.base(r) + "/blog/" + p.Slug, OGType: "article", OGImage: image}})
}
