// CSV 导入 HTTP 接口 / CSV Import HTTP Handlers
// 功能：受鉴权与 CSRF 保护的 CSV 干跑预览、任务查询与确认执行
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-21 12:23:42
package admin

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/kartwo/kartwo/internal/importer"
)

func (h *HTTP) previewCSVImport(w http.ResponseWriter, r *http.Request) {
	if h.importer == nil {
		writeErr(w, http.StatusServiceUnavailable, "导入服务不可用")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20+1024)
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "CSV 文件不能超过 5MB")
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "请选择 CSV 文件")
		return
	}
	defer func() { _ = f.Close() }()
	var p importer.Preview
	if r.FormValue("format") == importer.FormatShopify {
		p, err = h.importer.PreviewShopifyCSV(r.Context(), f)
	} else {
		p, err = h.importer.PreviewCSV(r.Context(), f)
	}
	if err != nil {
		h.writeImportErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}
func (h *HTTP) executeImport(w http.ResponseWriter, r *http.Request) {
	p, err := h.importer.Execute(r.Context(), r.PathValue("id"))
	if err != nil {
		h.writeImportErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}
func (h *HTTP) getImport(w http.ResponseWriter, r *http.Request) {
	p, err := h.importer.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		h.writeImportErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}
func (h *HTTP) writeImportErr(w http.ResponseWriter, err error) {
	var ve *importer.ValidationError
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeErr(w, http.StatusNotFound, "导入任务不存在")
	case errors.Is(err, http.ErrMissingFile):
		writeErr(w, http.StatusBadRequest, "请选择 CSV 文件")
	case errors.As(err, &ve):
		writeErr(w, http.StatusBadRequest, ve.Message)
	default:
		writeErr(w, http.StatusInternalServerError, "内部错误")
	}
}
