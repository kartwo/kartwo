// 博客后台接口 / Blog Admin Handlers
// 功能：文章管理、图片上传、开关设置和操作审计
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-13 10:00:00
package admin

import (
	"errors"
	"io"
	"net/http"

	"github.com/kartwo/kartwo/internal/blog"
)

type blogPostReq struct {
	Title          string `json:"title"`
	Slug           string `json:"slug"`
	Excerpt        string `json:"excerpt"`
	BodyMarkdown   string `json:"body_markdown"`
	CoverPath      string `json:"cover_path"`
	SEODescription string `json:"seo_description"`
	Status         string `json:"status"`
}

func (p blogPostReq) post() blog.Post {
	return blog.Post{Title: p.Title, Slug: p.Slug, Excerpt: p.Excerpt, BodyMarkdown: p.BodyMarkdown, CoverPath: p.CoverPath, SEODescription: p.SEODescription, Status: p.Status}
}

func (h *HTTP) blogErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, blog.ErrNotFound):
		writeErr(w, 404, "文章不存在")
	case errors.Is(err, blog.ErrConflict):
		writeErr(w, 409, "文章链接已存在")
	case errors.Is(err, blog.ErrInvalid):
		writeErr(w, 400, err.Error())
	default:
		writeErr(w, 500, "博客操作失败")
	}
}

func (h *HTTP) listBlogPosts(w http.ResponseWriter, r *http.Request) {
	if isDemoRequest(r) {
		writeJSON(w, 200, map[string]any{"posts": []blog.Post{}})
		return
	}
	posts, err := h.blog.ListAdmin(r.Context())
	if err != nil {
		h.blogErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"posts": posts})
}
func (h *HTTP) getBlogPost(w http.ResponseWriter, r *http.Request) {
	if isDemoRequest(r) {
		writeErr(w, 404, "文章不存在")
		return
	}
	p, err := h.blog.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		h.blogErr(w, err)
		return
	}
	writeJSON(w, 200, p)
}
func (h *HTTP) createBlogPost(w http.ResponseWriter, r *http.Request) {
	if isDemoRequest(r) {
		writeErr(w, 403, "公开演示不可修改文章")
		return
	}
	var req blogPostReq
	if !readJSON(w, r, &req) {
		return
	}
	id, err := h.blog.Create(r.Context(), req.post())
	if err != nil {
		h.blogErr(w, err)
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "blog.create", "blog_post", id)
	writeJSON(w, 201, map[string]string{"public_id": id})
}
func (h *HTTP) updateBlogPost(w http.ResponseWriter, r *http.Request) {
	if isDemoRequest(r) {
		writeErr(w, 403, "公开演示不可修改文章")
		return
	}
	var req blogPostReq
	if !readJSON(w, r, &req) {
		return
	}
	id := r.PathValue("id")
	if err := h.blog.Update(r.Context(), id, req.post()); err != nil {
		h.blogErr(w, err)
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "blog.update", "blog_post", id)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (h *HTTP) deleteBlogPost(w http.ResponseWriter, r *http.Request) {
	if isDemoRequest(r) {
		writeErr(w, 403, "公开演示不可修改文章")
		return
	}
	id := r.PathValue("id")
	if err := h.blog.Delete(r.Context(), id); err != nil {
		h.blogErr(w, err)
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "blog.delete", "blog_post", id)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (h *HTTP) getBlogSettings(w http.ResponseWriter, r *http.Request) {
	enabled, err := h.blog.Enabled(r.Context())
	if err != nil {
		h.blogErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"enabled": enabled})
}
func (h *HTTP) setBlogSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Enabled == nil {
		writeErr(w, 400, "请选择开启或关闭")
		return
	}
	if err := h.blog.SetEnabled(r.Context(), *req.Enabled); err != nil {
		h.blogErr(w, err)
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "blog.settings_update", "setting", "blog.enabled")
	h.getBlogSettings(w, r)
}
func (h *HTTP) uploadBlogImage(w http.ResponseWriter, r *http.Request) {
	if isDemoRequest(r) {
		writeErr(w, 403, "公开演示不可上传图片")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, uploadMaxBytes)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		writeErr(w, 413, "上传体过大或格式非法")
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, "缺少图片文件")
		return
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	if err != nil {
		writeErr(w, 400, "读取图片失败")
		return
	}
	img, err := h.media.StoreBlogImage(data)
	if err != nil {
		h.writeMediaErr(w, err)
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "blog.image_upload", "blog_image", "uploaded")
	writeJSON(w, 201, map[string]any{"path": img.Path, "url": img.URL, "width": img.Width, "height": img.Height})
}

func (h *HTTP) previewBlog(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BodyMarkdown string `json:"body_markdown"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if len(req.BodyMarkdown) > 100000 {
		writeErr(w, 400, "正文过长")
		return
	}
	writeJSON(w, 200, map[string]string{"html": string(blog.RenderMarkdown(req.BodyMarkdown))})
}
