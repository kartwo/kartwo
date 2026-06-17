// 数据库迁移执行器 / SQL Migration Runner
// 功能：按版本顺序应用嵌入的纯 SQL 迁移，幂等可重入（禁 AutoMigrate）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 17:05:46
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// createSchemaMigrations 记录已应用迁移版本，自身幂等建表。
const createSchemaMigrations = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY,
    applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);`

// Run 应用 fsys 中所有 *.sql 迁移（按文件名升序），仅执行未记录的版本。
// 每个迁移在独立事务内执行：要么整条成功并记录，要么回滚。返回本次新应用的数量。
func Run(ctx context.Context, db *sql.DB, fsys fs.FS) (int, error) {
	if _, err := db.ExecContext(ctx, createSchemaMigrations); err != nil {
		return 0, fmt.Errorf("migrate: 创建 schema_migrations 失败: %w", err)
	}

	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return 0, err
	}

	files, err := sqlFiles(fsys)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, name := range files {
		version := strings.TrimSuffix(name, ".sql")
		if applied[version] {
			continue
		}
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return count, fmt.Errorf("migrate: 读取 %s 失败: %w", name, err)
		}
		if err := applyOne(ctx, db, version, string(body)); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func applyOne(ctx context.Context, db *sql.DB, version, body string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migrate: 开启事务失败(%s): %w", version, err)
	}
	defer func() { _ = tx.Rollback() }() // 已提交后 Rollback 为 no-op

	if _, err := tx.ExecContext(ctx, body); err != nil {
		return fmt.Errorf("migrate: 执行迁移 %s 失败: %w", version, err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
		return fmt.Errorf("migrate: 记录迁移 %s 失败: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migrate: 提交迁移 %s 失败: %w", version, err)
	}
	return nil
}

func appliedVersions(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("migrate: 读取已应用版本失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("migrate: 扫描版本失败: %w", err)
		}
		out[v] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("migrate: 遍历版本失败: %w", err)
	}
	return out, nil
}

func sqlFiles(fsys fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("migrate: 读取迁移目录失败: %w", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files) // 文件名带版本前缀，字典序即应用顺序
	return files, nil
}
