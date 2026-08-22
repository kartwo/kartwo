// 后台数据导出 / Admin Data Export
// 功能：鉴权后下载一次性全量 ZIP 导出包
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-22 13:10:00
package admin

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

func (h *HTTP) exportData(w http.ResponseWriter, r *http.Request) {
	if h.exporter == nil {
		writeErr(w, http.StatusNotImplemented, "导出暂不可用")
		return
	}
	path, _, err := h.exporter.Create(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "创建导出包失败")
		return
	}
	defer func() { _ = os.Remove(path) }()
	f, err := os.Open(path) //nolint:gosec // path 由导出服务在受控数据目录中生成。
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取导出包失败")
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取导出包失败")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=kartwo-export-%s.zip", time.Now().UTC().Format("20060102T150405Z")))
	http.ServeContent(w, r, "export.zip", info.ModTime(), f)
}
