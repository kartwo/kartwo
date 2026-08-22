// 后台诊断接口 / Admin Diagnostics Handler
// 功能：返回数据库连通、媒体资产占用与媒体根目录磁盘容量（需鉴权，只读）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-22 09:10:00
package admin

import "net/http"

// diagnostics 返回本机可即时探测的健康项；外部服务探测将在各自能力到位后追加。
func (h *HTTP) diagnostics(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.db.PingContext(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "数据库暂不可用")
		return
	}
	usage, err := h.media.Diagnostics(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "媒体统计暂不可用")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"database": map[string]any{"status": "ok"},
		"media": map[string]any{
			"asset_count": usage.AssetCount, "original_bytes": usage.OriginalBytes,
			"derivative_bytes": usage.DerivativeBytes, "total_bytes": usage.TotalBytes,
		},
		"disk": map[string]any{
			"available": usage.Disk.Available, "total_bytes": usage.Disk.TotalBytes,
			"free_bytes": usage.Disk.FreeBytes, "used_bytes": usage.Disk.UsedBytes,
			"message": usage.Disk.Message,
		},
	})
}
