// 恢复访问隔离 / Restored Access Invalidation
// 功能：仅在私有恢复阶段原子失效会话、邀请和订单凭证，保留业务事实与验证限额证据
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 23:49:46
package backup

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// A backup is a historical authorization snapshot. A token revoked or consumed
// after export must not become valid when that snapshot is restored elsewhere.
// Only the unexposed staging copy is changed; accounts, role definitions, keys,
// financial/fulfillment facts and durable business operation keys remain intact.
// Older exports are supported without applying or manufacturing migrations.
func invalidateRestoredAccess(dbPath string) error {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=rw&_pragma=foreign_keys(ON)")
	if err != nil {
		return fmt.Errorf("backup: 打开恢复访问隔离失败: %w", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("backup: 开启恢复访问隔离失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	tables, err := restoredTables(ctx, tx)
	if err != nil {
		return err
	}
	steps := []struct {
		table, query string
		args         []any
	}{
		{"session", `DELETE FROM session`, nil},
		{"admin_invite", `DELETE FROM admin_invite`, nil},
		{"order_guest_access", `DELETE FROM order_guest_access`, nil},
		// Preserve quota/attempt timestamps, including previously consumed evidence;
		// only an unconsumed code is invalidated. No fresh email is queued here.
		{"order_guest_challenge", `UPDATE order_guest_challenge SET consumed_at=? WHERE consumed_at=0`, []any{time.Now().Unix()}},
		// Codes must not be delivered or remain readable after their authorization
		// has been invalidated. Preserve sent/unknown evidence and all other mail.
		{"email_outbox", `UPDATE email_outbox SET body='',status=CASE WHEN status IN ('pending','sending') THEN 'skipped' ELSE status END WHERE kind GLOB 'guest_access:*'`, nil},
	}
	for _, step := range steps {
		if !tables[step.table] {
			continue
		}
		if _, err = tx.ExecContext(ctx, step.query, step.args...); err != nil {
			return fmt.Errorf("backup: 隔离恢复访问凭证失败: %w", err)
		}
	}
	checks := []struct{ table, query string }{
		{"session", `SELECT count(*) FROM session`},
		{"admin_invite", `SELECT count(*) FROM admin_invite`},
		{"order_guest_access", `SELECT count(*) FROM order_guest_access`},
		{"order_guest_challenge", `SELECT count(*) FROM order_guest_challenge WHERE consumed_at=0`},
		{"email_outbox", `SELECT count(*) FROM email_outbox WHERE kind GLOB 'guest_access:*' AND (body<>'' OR status IN ('pending','sending'))`},
	}
	for _, check := range checks {
		if !tables[check.table] {
			continue
		}
		var remaining int64
		if err = tx.QueryRowContext(ctx, check.query).Scan(&remaining); err != nil {
			return fmt.Errorf("backup: 复核恢复访问隔离失败: %w", err)
		}
		if remaining != 0 {
			return fmt.Errorf("backup: 恢复访问凭证未完全失效")
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("backup: 提交恢复访问隔离失败: %w", err)
	}
	return nil
}
func restoredTables(ctx context.Context, tx *sql.Tx) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT name,type FROM sqlite_master WHERE type IN ('table','view')`)
	if err != nil {
		return nil, fmt.Errorf("backup: 读取恢复数据库表失败: %w", err)
	}
	defer func() { _ = rows.Close() }()
	tables := map[string]bool{}
	for rows.Next() {
		var name, kind string
		if err = rows.Scan(&name, &kind); err != nil {
			return nil, fmt.Errorf("backup: 读取恢复表名失败: %w", err)
		}
		name = strings.ToLower(name)
		if kind != "table" {
			switch name {
			case "session", "admin_invite", "order_guest_access", "order_guest_challenge", "email_outbox":
				return nil, fmt.Errorf("backup: 恢复访问表结构无效")
			}
			continue
		}
		tables[name] = true
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("backup: 遍历恢复数据库表失败: %w", err)
	}
	return tables, nil
}
