// 管理员账号安全 / Administrator Account Security
// 功能：校验当前主口令后修改店主用户名，并在修改主口令时原子轮换全部加密设置
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-14 09:30:00
package admin

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kartwo/kartwo/internal/auth"
	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

var (
	ErrInvalidUsername   = errors.New("admin: 管理员用户名不合法")
	ErrUsernameTaken     = errors.New("admin: 管理员用户名已被使用")
	ErrPasswordTooShort  = errors.New("admin: 新口令至少 8 位")
	ErrPasswordUnchanged = errors.New("admin: 新口令不能与当前口令相同")
	ErrNoAccountChange   = errors.New("admin: 用户名和口令均未修改")
)

// UpdateOwnerCredentials 在一个事务内更新店主账号；newPassword 为空表示只修改用户名。
// 主口令变化时，所有 encrypted=1 设置必须先用旧 KEK 解密、再用新 KEK 加密，任一步失败即回滚。
func (s *Service) UpdateOwnerCredentials(ctx context.Context, adminID int64, username, currentPassword, newPassword string) (bool, error) {
	username = strings.TrimSpace(username)
	if !validOwnerUsername(username) {
		return false, ErrInvalidUsername
	}
	if currentPassword == "" {
		return false, ErrInvalidCredentials
	}
	if newPassword != "" && len(newPassword) < minPasswordLen {
		return false, ErrPasswordTooShort
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("admin: 开启账号更新事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := s.q.WithTx(tx)

	owner, err := q.GetAdminUserByID(ctx, adminID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && owner.Role != "owner") {
		return false, ErrUnauthorized
	}
	if err != nil {
		return false, fmt.Errorf("admin: 读取店主账号失败: %w", err)
	}
	verified, err := auth.VerifyPassword(owner.PasswordHash, currentPassword)
	if err != nil {
		return false, fmt.Errorf("admin: 校验当前口令失败: %w", err)
	}
	if !verified {
		return false, ErrInvalidCredentials
	}
	if username == owner.Username && newPassword == "" {
		return false, ErrNoAccountChange
	}
	if existing, lookupErr := q.GetAdminUserByUsername(ctx, username); lookupErr == nil && existing.ID != owner.ID {
		return false, ErrUsernameTaken
	} else if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
		return false, fmt.Errorf("admin: 检查用户名失败: %w", lookupErr)
	}

	passwordHash := owner.PasswordHash
	passwordChanged := newPassword != ""
	if passwordChanged {
		if newPassword == currentPassword {
			return false, ErrPasswordUnchanged
		}
		passwordHash, err = auth.HashPassword(newPassword)
		if err != nil {
			return false, err
		}
		meta, metaErr := q.GetMeta(ctx, metaKeyKEKalt)
		if metaErr != nil {
			return false, fmt.Errorf("admin: 读取 KEK 盐失败: %w", metaErr)
		}
		salt, decodeErr := base64.RawStdEncoding.DecodeString(meta.Value)
		if decodeErr != nil {
			return false, fmt.Errorf("admin: 解析 KEK 盐失败: %w", decodeErr)
		}
		keySource := auth.NewMasterPasswordKeySource(salt)
		oldKEK, deriveErr := keySource.DeriveKey(currentPassword)
		if deriveErr != nil {
			return false, deriveErr
		}
		newKEK, deriveErr := keySource.DeriveKey(newPassword)
		if deriveErr != nil {
			return false, deriveErr
		}
		rows, listErr := q.ListEncryptedSettings(ctx)
		if listErr != nil {
			return false, fmt.Errorf("admin: 读取加密设置失败: %w", listErr)
		}
		for _, row := range rows {
			plaintext, decryptErr := auth.Decrypt(oldKEK, row.Value)
			if decryptErr != nil {
				return false, fmt.Errorf("admin: 加密设置 %q 无法用当前口令解锁: %w", row.Key, decryptErr)
			}
			ciphertext, encryptErr := auth.Encrypt(newKEK, plaintext)
			if encryptErr != nil {
				return false, fmt.Errorf("admin: 重新加密设置 %q 失败: %w", row.Key, encryptErr)
			}
			updated, updateErr := q.UpdateEncryptedSettingValue(ctx, sqlcgen.UpdateEncryptedSettingValueParams{Value: ciphertext, Key: row.Key})
			if updateErr != nil {
				return false, fmt.Errorf("admin: 写回加密设置 %q 失败: %w", row.Key, updateErr)
			}
			if updated != 1 {
				return false, fmt.Errorf("admin: 写回加密设置 %q 失败: 记录不存在", row.Key)
			}
		}
	}

	updated, err := q.UpdateOwnerCredentials(ctx, sqlcgen.UpdateOwnerCredentialsParams{Username: username, PasswordHash: passwordHash, ID: owner.ID})
	if err != nil {
		return false, fmt.Errorf("admin: 更新账号失败: %w", err)
	}
	if updated != 1 {
		return false, ErrUnauthorized
	}
	if err := q.DeleteSessionsByAdmin(ctx, owner.ID); err != nil {
		return false, fmt.Errorf("admin: 注销旧会话失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("admin: 提交账号更新失败: %w", err)
	}

	s.vault.clear()
	if s.keys != nil {
		s.keys.Lock()
	}
	if s.mailKeys != nil {
		s.mailKeys.Lock()
	}
	return passwordChanged, nil
}

func validOwnerUsername(username string) bool {
	if username == "" || username == demoUsername || utf8.RuneCountInString(username) > 64 {
		return false
	}
	for _, r := range username {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func (h *HTTP) getAccount(w http.ResponseWriter, r *http.Request) {
	ac := authFrom(r.Context())
	if ac.Role == "demo" {
		writeJSON(w, http.StatusOK, map[string]any{"username": "店主账号（已隐藏）", "readonly": true})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"username": ac.Username, "readonly": false})
}

func (h *HTTP) updateAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username        string `json:"username"`
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	ac := authFrom(r.Context())
	passwordChanged, err := h.svc.UpdateOwnerCredentials(r.Context(), ac.AdminID, req.Username, req.CurrentPassword, req.NewPassword)
	switch {
	case err == nil:
		h.recordAudit(r, ac.AdminID, "account.credentials_update", "admin", ac.AdminPublicID)
		h.clearCookie(w, r, sessionCookie, true)
		h.clearCookie(w, r, csrfCookie, false)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "username": strings.TrimSpace(req.Username), "password_changed": passwordChanged, "signed_out": true})
	case errors.Is(err, ErrInvalidCredentials):
		writeErr(w, http.StatusForbidden, "当前密码错误")
	case errors.Is(err, ErrUsernameTaken):
		writeErr(w, http.StatusConflict, "该用户名已被使用")
	case errors.Is(err, ErrInvalidUsername):
		writeErr(w, http.StatusBadRequest, "用户名不能为空、不能使用保留名称或控制字符，且最多 64 个字符")
	case errors.Is(err, ErrPasswordTooShort):
		writeErr(w, http.StatusBadRequest, "新密码至少 8 位")
	case errors.Is(err, ErrPasswordUnchanged):
		writeErr(w, http.StatusBadRequest, "新密码不能与当前密码相同")
	case errors.Is(err, ErrNoAccountChange):
		writeErr(w, http.StatusBadRequest, "用户名和密码均未修改")
	default:
		writeErr(w, http.StatusInternalServerError, "更新账号失败；原账号和加密配置保持不变")
	}
}
