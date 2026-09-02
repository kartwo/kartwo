// 自动备份设置 HTTP / Backup Settings Handlers
// 功能：后台保存本地备份和 WebDAV 异地备份配置，环境变量覆盖字段只读
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-25 16:42:00
package admin

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kartwo/kartwo/internal/backup"
	"github.com/kartwo/kartwo/internal/config"
	"github.com/kartwo/kartwo/internal/settings"
)

// BackupConfig 是启动时实际生效的备份配置。
// 本地备份的 interval/retention 与 WebDAV 字段都允许部分环境变量覆盖（env 覆盖 -> 该字段只读）。
type BackupConfig struct {
	Interval    time.Duration
	Retention   int
	IntervalEnv bool
	RetentionEnv bool

	WebDAVEnabled     bool
	WebDAVEnabledEnv  bool
	WebDAVURLEnv      bool
	WebDAVPathEnv     bool
	WebDAVUsernameEnv bool
	WebDAVPasswordEnv bool
	WebDAVPassword    string
	WebDAVURL         string
	WebDAVPath        string
	WebDAVUsername    string
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

	webDAVEnabled := strconv.FormatBool(h.backupCfg.WebDAVEnabled)
	webDAVEnabledSource := "default"
	if h.backupCfg.WebDAVEnabledEnv {
		webDAVEnabledSource = "env"
	} else if raw, err := h.settings.Get(r.Context(), config.BackupWebDAVEnabledSetting); err == nil && strings.TrimSpace(raw) != "" {
		if enabled, err := config.ParseBackupWebDAVEnabled(raw); err == nil {
			webDAVEnabled = strconv.FormatBool(enabled)
			webDAVEnabledSource = "db"
		}
	}

	webDAVURL := h.backupCfg.WebDAVURL
	webDAVURLSource := "default"
	if h.backupCfg.WebDAVURLEnv {
		webDAVURLSource = "env"
	} else if raw, err := h.settings.Get(r.Context(), config.BackupWebDAVURLSetting); err == nil && strings.TrimSpace(raw) != "" {
		if parsed, err := config.ParseBackupWebDAVURL(raw); err == nil {
			webDAVURL, webDAVURLSource = parsed, "db"
		}
	}

	webDAVPath := h.backupCfg.WebDAVPath
	webDAVPathSource := "default"
	if h.backupCfg.WebDAVPathEnv {
		webDAVPathSource = "env"
	} else if raw, err := h.settings.Get(r.Context(), config.BackupWebDAVPathSetting); err == nil && strings.TrimSpace(raw) != "" {
		if parsed, err := config.ParseBackupWebDAVPath(raw); err == nil {
			webDAVPath, webDAVPathSource = parsed, "db"
		}
	}

	webDAVUsername := h.backupCfg.WebDAVUsername
	webDAVUsernameSource := "default"
	if h.backupCfg.WebDAVUsernameEnv {
		webDAVUsernameSource = "env"
	} else if raw, err := h.settings.Get(r.Context(), config.BackupWebDAVUsernameSetting); err == nil {
		webDAVUsername = strings.TrimSpace(raw)
		webDAVUsernameSource = "db"
	}

	passwordSet := h.settingExists(r.Context(), config.BackupWebDAVPasswordSetting)
	if h.backupCfg.WebDAVPasswordEnv {
		passwordSet = h.backupCfg.WebDAVPassword != ""
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"interval":             interval,
		"retention":            retention,
		"interval_source":      intervalSource,
		"retention_source":     retentionSource,
		"interval_readonly":    h.backupCfg.IntervalEnv,
		"retention_readonly":   h.backupCfg.RetentionEnv,
		"webdav_enabled":       webDAVEnabled,
		"webdav_enabled_source": webDAVEnabledSource,
		"webdav_enabled_readonly": h.backupCfg.WebDAVEnabledEnv,
		"webdav_url":           webDAVURL,
		"webdav_url_source":    webDAVURLSource,
		"webdav_url_readonly":  h.backupCfg.WebDAVURLEnv,
		"webdav_path":          webDAVPath,
		"webdav_path_source":   webDAVPathSource,
		"webdav_path_readonly": h.backupCfg.WebDAVPathEnv,
		"webdav_username":      webDAVUsername,
		"webdav_username_source": webDAVUsernameSource,
		"webdav_username_readonly": h.backupCfg.WebDAVUsernameEnv,
		"webdav_password_set":   passwordSet,
		"webdav_password_readonly": h.backupCfg.WebDAVPasswordEnv,
		"restart_required":     true,
	})
}

func (h *HTTP) setBackupSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Interval       string  `json:"interval"`
		Retention      string  `json:"retention"`
		WebDAVEnabled  *bool   `json:"webdav_enabled"`
		WebDAVURL      *string `json:"webdav_url"`
		WebDAVPath     *string `json:"webdav_path"`
		WebDAVUsername *string `json:"webdav_username"`
		WebDAVPassword *string `json:"webdav_password"`
	}
	if !readJSON(w, r, &req) {
		return
	}

	if h.backupCfg.IntervalEnv && strings.TrimSpace(req.Interval) != "" {
		writeErr(w, http.StatusConflict, "自动备份周期由环境变量提供（只读）。请修改 KARTWO_BACKUP_INTERVAL 后重启。")
		return
	}
	if h.backupCfg.RetentionEnv && strings.TrimSpace(req.Retention) != "" {
		writeErr(w, http.StatusConflict, "自动备份保留份数由环境变量提供（只读）。请修改 KARTWO_BACKUP_RETENTION 后重启。")
		return
	}
	if h.backupCfg.WebDAVEnabledEnv && req.WebDAVEnabled != nil {
		writeErr(w, http.StatusConflict, "WebDAV 启用开关由环境变量提供（只读）。请修改 KARTWO_BACKUP_WEBDAV_ENABLED 后重启。")
		return
	}
	if h.backupCfg.WebDAVURLEnv && req.WebDAVURL != nil {
		writeErr(w, http.StatusConflict, "WebDAV 地址由环境变量提供（只读）。请修改 KARTWO_BACKUP_WEBDAV_URL 后重启。")
		return
	}
	if h.backupCfg.WebDAVPathEnv && req.WebDAVPath != nil {
		writeErr(w, http.StatusConflict, "WebDAV 目录由环境变量提供（只读）。请修改 KARTWO_BACKUP_WEBDAV_PATH 后重启。")
		return
	}
	if h.backupCfg.WebDAVUsernameEnv && req.WebDAVUsername != nil {
		writeErr(w, http.StatusConflict, "WebDAV 用户名由环境变量提供（只读）。请修改 KARTWO_BACKUP_WEBDAV_USERNAME 后重启。")
		return
	}
	if h.backupCfg.WebDAVPasswordEnv && req.WebDAVPassword != nil {
		writeErr(w, http.StatusConflict, "WebDAV 密码由环境变量提供（只读）。请修改 KARTWO_BACKUP_WEBDAV_PASSWORD 后重启。")
		return
	}

	ctx := r.Context()
	ac := authFrom(ctx)

	wrote := false
	if strings.TrimSpace(req.Interval) != "" && !h.backupCfg.IntervalEnv {
		if _, err := config.ParseBackupInterval(req.Interval); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := h.settings.SetPlain(ctx, config.BackupIntervalSetting, strings.TrimSpace(req.Interval)); err != nil {
			writeErr(w, http.StatusInternalServerError, "保存失败")
			return
		}
		wrote = true
	}
	if strings.TrimSpace(req.Retention) != "" && !h.backupCfg.RetentionEnv {
		if _, err := config.ParseBackupRetention(req.Retention); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := h.settings.SetPlain(ctx, config.BackupRetentionSetting, strings.TrimSpace(req.Retention)); err != nil {
			writeErr(w, http.StatusInternalServerError, "保存失败")
			return
		}
		wrote = true
	}
	if req.WebDAVEnabled != nil && !h.backupCfg.WebDAVEnabledEnv {
		if err := h.settings.SetPlain(ctx, config.BackupWebDAVEnabledSetting, strconv.FormatBool(*req.WebDAVEnabled)); err != nil {
			writeErr(w, http.StatusInternalServerError, "保存失败")
			return
		}
		wrote = true
	}
	if req.WebDAVURL != nil && strings.TrimSpace(*req.WebDAVURL) != "" && !h.backupCfg.WebDAVURLEnv {
		if _, err := config.ParseBackupWebDAVURL(*req.WebDAVURL); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := h.settings.SetPlain(ctx, config.BackupWebDAVURLSetting, strings.TrimSpace(*req.WebDAVURL)); err != nil {
			writeErr(w, http.StatusInternalServerError, "保存失败")
			return
		}
		wrote = true
	}
	if req.WebDAVPath != nil && strings.TrimSpace(*req.WebDAVPath) != "" && !h.backupCfg.WebDAVPathEnv {
		if _, err := config.ParseBackupWebDAVPath(*req.WebDAVPath); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := h.settings.SetPlain(ctx, config.BackupWebDAVPathSetting, strings.TrimSpace(*req.WebDAVPath)); err != nil {
			writeErr(w, http.StatusInternalServerError, "保存失败")
			return
		}
		wrote = true
	}
	if req.WebDAVUsername != nil && !h.backupCfg.WebDAVUsernameEnv {
		if err := h.settings.SetPlain(ctx, config.BackupWebDAVUsernameSetting, strings.TrimSpace(*req.WebDAVUsername)); err != nil {
			writeErr(w, http.StatusInternalServerError, "保存失败")
			return
		}
		wrote = true
	}
	if req.WebDAVPassword != nil && !h.backupCfg.WebDAVPasswordEnv {
		if *req.WebDAVPassword != "" {
			kek, ok := h.svc.Key(ac.SessionToken)
			if !ok {
				writeErr(w, http.StatusInternalServerError, "会话密钥不可用，请重新登录")
				return
			}
			if err := h.settings.SetEncrypted(ctx, config.BackupWebDAVPasswordSetting, []byte(*req.WebDAVPassword), kek); err != nil {
				writeErr(w, http.StatusInternalServerError, "保存失败")
				return
			}
			wrote = true
		}
	}

	if !wrote {
		h.getBackupSettings(w, r)
		return
	}
	h.recordAudit(r, ac.AdminID, "backup.settings_update", "settings", "backup")
	h.getBackupSettings(w, r)
}

func (h *HTTP) testBackupRemote(w http.ResponseWriter, r *http.Request) {
	enabled, endpoint, remotePath, username, err := h.effectiveWebDAV(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadRequest, "WebDAV 配置无效")
		return
	}
	if !enabled {
		writeErr(w, http.StatusBadRequest, "未启用 WebDAV 异地备份")
		return
	}
	password, err := h.webDAVPassword(r.Context())
	if err != nil {
		if errors.Is(err, settings.ErrNotFound) {
			writeErr(w, http.StatusBadRequest, "未配置 WebDAV 密码")
			return
		}
		writeErr(w, http.StatusInternalServerError, "读取 WebDAV 配置失败")
		return
	}
	u, err := backup.NewWebDAVUploader(endpoint, username, password, remotePath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := u.Test(r.Context(), ""); err != nil {
		writeErr(w, http.StatusBadGateway, "WebDAV 测试失败："+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// effectiveWebDAV 读取本次管理员测试应使用的有效配置。环境变量逐字段优先；
// 数据库值仅在已登录会话中配合加密密码参与连通测试，不用于冷启动后台任务。
func (h *HTTP) effectiveWebDAV(ctx context.Context) (bool, string, string, string, error) {
	enabled := h.backupCfg.WebDAVEnabled
	endpoint := h.backupCfg.WebDAVURL
	remotePath := h.backupCfg.WebDAVPath
	username := h.backupCfg.WebDAVUsername
	if !h.backupCfg.WebDAVEnabledEnv {
		if raw, err := h.settings.Get(ctx, config.BackupWebDAVEnabledSetting); err == nil && strings.TrimSpace(raw) != "" {
			var err error
			enabled, err = config.ParseBackupWebDAVEnabled(raw)
			if err != nil {
				return false, "", "", "", err
			}
		}
	}
	if !h.backupCfg.WebDAVURLEnv {
		if raw, err := h.settings.Get(ctx, config.BackupWebDAVURLSetting); err == nil && strings.TrimSpace(raw) != "" {
			var err error
			endpoint, err = config.ParseBackupWebDAVURL(raw)
			if err != nil {
				return false, "", "", "", err
			}
		}
	}
	if !h.backupCfg.WebDAVPathEnv {
		if raw, err := h.settings.Get(ctx, config.BackupWebDAVPathSetting); err == nil && strings.TrimSpace(raw) != "" {
			var err error
			remotePath, err = config.ParseBackupWebDAVPath(raw)
			if err != nil {
				return false, "", "", "", err
			}
		}
	}
	if !h.backupCfg.WebDAVUsernameEnv {
		if raw, err := h.settings.Get(ctx, config.BackupWebDAVUsernameSetting); err == nil {
			username = strings.TrimSpace(raw)
		}
	}
	return enabled, endpoint, remotePath, username, nil
}

func (h *HTTP) webDAVPassword(ctx context.Context) (string, error) {
	if h.backupCfg.WebDAVPasswordEnv {
		return h.backupCfg.WebDAVPassword, nil
	}
	ac := authFrom(ctx)
	kek, ok := h.svc.Key(ac.SessionToken)
	if !ok {
		return "", errors.New("会话密钥不可用，请重新登录")
	}
	b, err := h.settings.GetEncrypted(ctx, config.BackupWebDAVPasswordSetting, kek)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
