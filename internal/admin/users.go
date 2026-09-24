// 后台用户与角色管理 / Admin Users and Roles
// 功能：店主管理员工、角色、一次性邀请和会话失效
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-24 11:00:00
package admin

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kartwo/kartwo/internal/auth"
)

type managedUser struct {
	PublicID    string `json:"public_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	RoleID      string `json:"role_id"`
	RoleName    string `json:"role_name"`
	Status      string `json:"status"`
	LastLoginAt string `json:"last_login_at"`
	CreatedAt   string `json:"created_at"`
}

type managedRole struct {
	PublicID    string   `json:"public_id"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
	SystemKey   string   `json:"system_key"`
	UserCount   int      `json:"user_count"`
}

func (h *HTTP) listUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.db.QueryContext(r.Context(), `SELECT a.public_id, a.username, a.display_name, COALESCE(ro.public_id,''), COALESCE(ro.name,'店主'),
	CASE WHEN a.disabled_at IS NOT NULL THEN 'disabled' WHEN a.password_hash = 'invite-pending' THEN 'invited' ELSE 'active' END,
	COALESCE(a.last_login_at,''), a.created_at
	FROM admin_user a LEFT JOIN admin_role ro ON ro.id = a.role_id WHERE a.account_type <> 'demo' ORDER BY a.id`)
	if err != nil {
		writeErr(w, 500, "读取用户失败")
		return
	}
	defer func() { _ = rows.Close() }()
	out := []managedUser{}
	for rows.Next() {
		var u managedUser
		if err := rows.Scan(&u.PublicID, &u.Username, &u.DisplayName, &u.RoleID, &u.RoleName, &u.Status, &u.LastLoginAt, &u.CreatedAt); err != nil {
			writeErr(w, 500, "读取用户失败")
			return
		}
		out = append(out, u)
	}
	if rows.Err() != nil {
		writeErr(w, 500, "读取用户失败")
		return
	}
	writeJSON(w, 200, out)
}

func (h *HTTP) listRoles(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.db.QueryContext(r.Context(), `SELECT ro.public_id, ro.name, ro.permissions, COALESCE(ro.system_key,''), COUNT(a.id)
	FROM admin_role ro LEFT JOIN admin_user a ON a.role_id = ro.id AND a.account_type = 'staff'
	GROUP BY ro.id ORDER BY ro.id`)
	if err != nil {
		writeErr(w, 500, "读取角色失败")
		return
	}
	defer func() { _ = rows.Close() }()
	out := []managedRole{}
	for rows.Next() {
		var role managedRole
		var raw string
		if err := rows.Scan(&role.PublicID, &role.Name, &raw, &role.SystemKey, &role.UserCount); err != nil {
			writeErr(w, 500, "读取角色失败")
			return
		}
		if err := json.Unmarshal([]byte(raw), &role.Permissions); err != nil {
			writeErr(w, 500, "角色权限格式非法")
			return
		}
		out = append(out, role)
	}
	if rows.Err() != nil {
		writeErr(w, 500, "读取角色失败")
		return
	}
	writeJSON(w, 200, out)
}

func cleanRoleInput(name string, permissions []string) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 60 {
		return "", "", errors.New("角色名称不能为空且最多 60 字")
	}
	set := map[string]bool{}
	for _, permission := range permissions {
		if !validPermission(permission) || ownerOnlyPermissions[permission] {
			return "", "", fmt.Errorf("不可分配权限 %q", permission)
		}
		set[permission] = true
	}
	if len(set) == 0 {
		return "", "", errors.New("至少选择一项权限")
	}
	for _, permission := range []string{"products.create", "products.edit", "products.publish", "products.delete", "products.inventory", "products.price", "products.media", "categories.manage"} {
		if set[permission] && !set["products.view"] {
			return "", "", errors.New("商品操作权限需要同时勾选「查看商品」")
		}
	}
	if set["orders.customer_data"] && !set["orders.view"] {
		return "", "", errors.New("客户信息权限需要同时勾选「查看订单」")
	}
	if set["orders.fulfill"] && (!set["orders.view"] || !set["orders.customer_data"]) {
		return "", "", errors.New("发货权限需要同时勾选「查看订单」和「查看客户信息」")
	}
	raw, err := json.Marshal(permissionsList(set))
	return name, string(raw), err
}

func (h *HTTP) createRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	name, raw, err := cleanRoleInput(req.Name, req.Permissions)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	id := uuid.Must(uuid.NewV7()).String()
	if _, err := h.svc.db.ExecContext(r.Context(), "INSERT INTO admin_role (public_id,name,permissions) VALUES (?,?,?)", id, name, raw); err != nil {
		writeErr(w, 409, "角色名已存在或保存失败")
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "role.create", "role", id)
	writeJSON(w, 201, map[string]string{"public_id": id})
}

func (h *HTTP) updateRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	name, raw, err := cleanRoleInput(req.Name, req.Permissions)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	id := r.PathValue("id")
	tx, err := h.svc.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, 500, "保存角色失败")
		return
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(r.Context(), "UPDATE admin_role SET name=?, permissions=?, updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id=? AND system_key IS NULL", name, raw, id)
	if err != nil {
		writeErr(w, 409, "角色名已存在或保存失败")
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		writeErr(w, 404, "自定义角色不存在；预设角色不可修改")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "DELETE FROM session WHERE admin_id IN (SELECT a.id FROM admin_user a JOIN admin_role ro ON ro.id=a.role_id WHERE ro.public_id=?)", id); err != nil {
		writeErr(w, 500, "撤销旧会话失败")
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, 500, "保存角色失败")
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "role.update", "role", id)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *HTTP) deleteRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	result, err := h.svc.db.ExecContext(r.Context(), `DELETE FROM admin_role WHERE public_id=? AND system_key IS NULL
	AND NOT EXISTS (SELECT 1 FROM admin_user WHERE role_id=admin_role.id)`, id)
	if err != nil {
		writeErr(w, 500, "删除角色失败")
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		writeErr(w, 409, "角色不存在、为预设角色或仍有用户")
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "role.delete", "role", id)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func inviteHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (h *HTTP) issueInvite(ctx context.Context, adminID int64) (string, error) {
	token, err := randToken()
	if err != nil {
		return "", err
	}
	tx, err := h.svc.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, "DELETE FROM admin_invite WHERE admin_id=?", adminID); err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO admin_invite (token_hash,admin_id,expires_at) VALUES (?,?,?)", inviteHash(token), adminID, time.Now().UTC().Add(24*time.Hour).Format(timeLayout))
	if err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return "/admin/#/accept-invite?token=" + token, nil
}

func (h *HTTP) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		RoleID      string `json:"role_id"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	username := strings.ToLower(strings.TrimSpace(req.Username))
	if !validStaffUsername(username) || len([]rune(req.DisplayName)) > 100 {
		writeErr(w, 400, "用户名或显示名称非法")
		return
	}
	var duplicate int
	if err := h.svc.db.QueryRowContext(r.Context(), "SELECT 1 FROM admin_user WHERE lower(username)=?", username).Scan(&duplicate); err == nil {
		writeErr(w, 409, "用户名已存在")
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 500, "检查用户名失败")
		return
	}
	var roleID int64
	if err := h.svc.db.QueryRowContext(r.Context(), "SELECT id FROM admin_role WHERE public_id=?", req.RoleID).Scan(&roleID); err != nil {
		writeErr(w, 400, "角色不存在")
		return
	}
	id := uuid.Must(uuid.NewV7()).String()
	result, err := h.svc.db.ExecContext(r.Context(), "INSERT INTO admin_user (public_id,username,password_hash,account_type,role_id,display_name) VALUES (?,?,'invite-pending','staff',?,?)", id, username, roleID, strings.TrimSpace(req.DisplayName))
	if err != nil {
		writeErr(w, 409, "用户名已存在")
		return
	}
	internalID, _ := result.LastInsertId()
	link, err := h.issueInvite(r.Context(), internalID)
	if err != nil {
		_, _ = h.svc.db.ExecContext(r.Context(), "DELETE FROM admin_user WHERE id=?", internalID)
		writeErr(w, 500, "生成邀请失败")
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "user.invite", "admin", id)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 201, map[string]string{"public_id": id, "invite_path": link})
}

func validStaffUsername(name string) bool {
	if len(name) < 3 || len(name) > 64 || name == demoUsername {
		return false
	}
	for _, ch := range name {
		if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '.' && ch != '_' && ch != '-' {
			return false
		}
	}
	return true
}

func (h *HTTP) updateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DisplayName string `json:"display_name"`
		RoleID      string `json:"role_id"`
		Disabled    bool   `json:"disabled"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if len([]rune(req.DisplayName)) > 100 {
		writeErr(w, 400, "显示名称过长")
		return
	}
	var roleID int64
	if err := h.svc.db.QueryRowContext(r.Context(), "SELECT id FROM admin_role WHERE public_id=?", req.RoleID).Scan(&roleID); err != nil {
		writeErr(w, 400, "角色不存在")
		return
	}
	id := r.PathValue("id")
	tx, err := h.svc.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, 500, "保存用户失败")
		return
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(r.Context(), `UPDATE admin_user SET display_name=?, role_id=?, disabled_at=CASE WHEN ? THEN strftime('%Y-%m-%dT%H:%M:%fZ','now') ELSE NULL END,
	updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id=? AND account_type='staff'`, strings.TrimSpace(req.DisplayName), roleID, req.Disabled, id)
	if err != nil {
		writeErr(w, 500, "保存用户失败")
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		writeErr(w, 404, "员工账号不存在")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "DELETE FROM session WHERE admin_id=(SELECT id FROM admin_user WHERE public_id=?)", id); err != nil {
		writeErr(w, 500, "撤销旧会话失败")
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, 500, "保存用户失败")
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "user.update", "admin", id)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *HTTP) resetUserPassword(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var internalID int64
	if err := h.svc.db.QueryRowContext(r.Context(), "SELECT id FROM admin_user WHERE public_id=? AND account_type='staff' AND disabled_at IS NULL", id).Scan(&internalID); err != nil {
		writeErr(w, 404, "可用员工账号不存在")
		return
	}
	link, err := h.issueInvite(r.Context(), internalID)
	if err != nil {
		writeErr(w, 500, "生成重置链接失败")
		return
	}
	if _, err := h.svc.db.ExecContext(r.Context(), "DELETE FROM session WHERE admin_id=?", internalID); err != nil {
		writeErr(w, 500, "撤销旧会话失败")
		return
	}
	h.recordAudit(r, authFrom(r.Context()).AdminID, "user.password_reset", "admin", id)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]string{"invite_path": link})
}

func (h *HTTP) revokeUserSessions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var exists int
	if err := h.svc.db.QueryRowContext(r.Context(), "SELECT 1 FROM admin_user WHERE public_id=? AND account_type='staff'", id).Scan(&exists); err != nil {
		writeErr(w, 404, "员工账号不存在")
		return
	}
	result, err := h.svc.db.ExecContext(r.Context(), "DELETE FROM session WHERE admin_id=(SELECT id FROM admin_user WHERE public_id=? AND account_type='staff')", id)
	if err != nil {
		writeErr(w, 500, "撤销会话失败")
		return
	}
	n, _ := result.RowsAffected()
	h.recordAudit(r, authFrom(r.Context()).AdminID, "user.sessions_revoke", "admin", id)
	writeJSON(w, 200, map[string]any{"revoked": n})
}

func (h *HTTP) acceptInvite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if len(req.Password) < minPasswordLen || len(req.Password) > 1024 {
		writeErr(w, 400, "密码长度须为 8–1024 位")
		return
	}
	if len(req.Token) < 32 || len(req.Token) > 256 {
		writeErr(w, 400, "邀请链接无效或已过期")
		return
	}
	if !h.limiter.allow(clientIP(r) + "|invite") {
		writeErr(w, http.StatusTooManyRequests, "尝试过于频繁，请稍后重试")
		return
	}
	var tokenValid int
	err := h.svc.db.QueryRowContext(r.Context(), `SELECT 1 FROM admin_invite i JOIN admin_user a ON a.id=i.admin_id
	WHERE i.token_hash=? AND i.expires_at>? AND a.account_type='staff' AND a.disabled_at IS NULL`, inviteHash(req.Token), time.Now().UTC().Format(timeLayout)).Scan(&tokenValid)
	if err != nil {
		writeErr(w, 400, "邀请链接无效或已过期")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, 500, "设置密码失败")
		return
	}
	tx, err := h.svc.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, 500, "设置密码失败")
		return
	}
	defer func() { _ = tx.Rollback() }()
	var adminID int64
	err = tx.QueryRowContext(r.Context(), `SELECT i.admin_id FROM admin_invite i JOIN admin_user a ON a.id=i.admin_id
	WHERE i.token_hash=? AND i.consumed_at IS NULL AND i.expires_at>? AND a.account_type='staff' AND a.disabled_at IS NULL`, inviteHash(req.Token), time.Now().UTC().Format(timeLayout)).Scan(&adminID)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 400, "邀请链接无效或已过期")
		return
	}
	if err != nil {
		writeErr(w, 500, "设置密码失败")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "UPDATE admin_user SET password_hash=?, updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?", hash, adminID); err != nil {
		writeErr(w, 500, "设置密码失败")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "DELETE FROM session WHERE admin_id=?", adminID); err != nil {
		writeErr(w, 500, "设置密码失败")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "DELETE FROM admin_invite WHERE admin_id=?", adminID); err != nil {
		writeErr(w, 500, "设置密码失败")
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, 500, "设置密码失败")
		return
	}
	h.limiter.reset(clientIP(r) + "|invite")
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *HTTP) updateStaffPassword(w http.ResponseWriter, r *http.Request) {
	ac := authFrom(r.Context())
	if ac.Role != "staff" {
		writeErr(w, 403, "仅员工账号使用此入口")
		return
	}
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if len(req.NewPassword) < minPasswordLen || len(req.NewPassword) > 1024 || req.NewPassword == req.CurrentPassword {
		writeErr(w, 400, "新密码至少 8 位且不能与旧密码相同")
		return
	}
	var oldHash string
	if err := h.svc.db.QueryRowContext(r.Context(), "SELECT password_hash FROM admin_user WHERE id=? AND account_type='staff' AND disabled_at IS NULL", ac.AdminID).Scan(&oldHash); err != nil {
		writeErr(w, 401, "账号不可用")
		return
	}
	ok, err := auth.VerifyPassword(oldHash, req.CurrentPassword)
	if err != nil || !ok {
		writeErr(w, 400, "当前密码错误")
		return
	}
	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeErr(w, 500, "更新密码失败")
		return
	}
	tx, err := h.svc.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, 500, "更新密码失败")
		return
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(r.Context(), "UPDATE admin_user SET password_hash=?, updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=? AND password_hash=?", newHash, ac.AdminID, oldHash)
	if err != nil {
		writeErr(w, 500, "更新密码失败")
		return
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		writeErr(w, 409, "密码已变更，请重新登录")
		return
	}
	if _, err := tx.ExecContext(r.Context(), "DELETE FROM session WHERE admin_id=?", ac.AdminID); err != nil {
		writeErr(w, 500, "更新密码失败")
		return
	}
	if err := tx.Commit(); err != nil {
		writeErr(w, 500, "更新密码失败")
		return
	}
	h.recordAudit(r, ac.AdminID, "user.password_change", "admin", ac.AdminPublicID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
