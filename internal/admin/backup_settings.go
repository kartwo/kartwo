// 自动备份设置 HTTP / Backup Settings Handlers
// 功能：后台保存本地备份周期与保留份数，环境变量覆盖时对应字段只读
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-23 14:20:00
package admin

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kartwo/kartwo/internal/config"
)

// BackupConfig 是启动时实际生效的自动备份配置；后台保存的新值在下次重启生效。
type BackupConfig struct {
	Interval     time.Duration
	Retention    int
	IntervalEnv  bool
	RetentionEnv bool
}

func (h *HTTP) getBackupSettings(w http.ResponseWriter, r *http.Request) {
	interval, intervalSource := h.backupCfg.Interval.String(), "default"
	retention, retentionSource := h.backupCfg.Retention, "default"
	if h.backupCfg.IntervalEnv {
		intervalSource = "env"
	} else if raw, err := h.settings.Get(r.Context(), config.BackupIntervalSetting); err == nil && strings.TrimSpace(raw) != "" {
		interval, intervalSource = raw, "db"
	}
	if h.backupCfg.RetentionEnv {
		retentionSource = "env"
	} else if raw, err := h.settings.Get(r.Context(), config.BackupRetentionSetting); err == nil && strings.TrimSpace(raw) != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			retention, retentionSource = n, "db"
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"interval": interval, "retention": retention,
		"interval_source": intervalSource, "retention_source": retentionSource,
		"interval_readonly": h.backupCfg.IntervalEnv, "retention_readonly": h.backupCfg.RetentionEnv,
		"restart_required": true,
	})
}

func (h *HTTP) setBackupSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Interval  string `json:"interval"`
		Retention string `json:"retention"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !h.backupCfg.IntervalEnv {
		if _, err := config.ParseBackupInterval(req.Interval); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := h.settings.SetPlain(r.Context(), config.BackupIntervalSetting, strings.TrimSpace(req.Interval)); err != nil {
			writeErr(w, http.StatusInternalServerError, "保存失败")
			return
		}
	}
	if !h.backupCfg.RetentionEnv {
		if _, err := config.ParseBackupRetention(req.Retention); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := h.settings.SetPlain(r.Context(), config.BackupRetentionSetting, strings.TrimSpace(req.Retention)); err != nil {
			writeErr(w, http.StatusInternalServerError, "保存失败")
			return
		}
	}
	if h.backupCfg.IntervalEnv && h.backupCfg.RetentionEnv {
		writeErr(w, http.StatusConflict, "自动备份设置由环境变量提供（只读）。请修改 KARTWO_BACKUP_INTERVAL / KARTWO_BACKUP_RETENTION 后重启。")
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "backup.settings_update", "settings", "backup")
	h.getBackupSettings(w, r)
}
