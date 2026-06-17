// 数据层 / Data Store
// 功能：打开并持有数据库连接（M0 默认 SQLite 实现），为上层提供 *sql.DB 接缝
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 17:05:46
package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/kartwo/kartwo/internal/config"

	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动（无 CGO），保证单静态二进制可交叉编译
)

// Store 持有数据库句柄。
// 数据层选型 = sqlc（纯 SQL→生成类型安全代码，见 internal/store/sqlcgen）；
// 此处只负责连接与方言接缝，业务查询由 sqlc 生成代码承载。
// 双模式纪律：Engine 为分叉点，M0 仅落地 sqlite 默认实现，postgres 为升级项接口占位。
type Store struct {
	DB     *sql.DB
	Engine string
}

// Open 按配置打开数据库。M0 仅支持 sqlite。
func Open(cfg *config.Config) (*Store, error) {
	if cfg.DBEngine != "sqlite" {
		return nil, fmt.Errorf("store: 引擎 %q 暂未实现（M0 仅支持 sqlite）", cfg.DBEngine)
	}

	// 确保数据目录存在（数据即文件夹）。
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return nil, fmt.Errorf("store: 创建数据目录失败: %w", err)
	}

	db, err := openSQLite(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	return &Store{DB: db, Engine: "sqlite"}, nil
}

// openSQLite 以可移植、安全的 PRAGMA 打开 SQLite：
// WAL（并发读不阻塞写）、busy_timeout（避免瞬时 locked）、foreign_keys（约束生效）。
func openSQLite(path string) (*sql.DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("store: 解析数据库路径失败: %w", err)
	}
	dsn := "file:" + abs + "?" + url.Values{
		"_pragma": {"journal_mode(WAL)", "busy_timeout(5000)", "foreign_keys(ON)"},
	}.Encode()

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: 打开 SQLite 失败: %w", err)
	}
	// SQLite 单写者：限制连接数避免写锁竞争。
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: 连接 SQLite 失败: %w", err)
	}
	return db, nil
}

// Close 关闭数据库连接。
func (s *Store) Close() error {
	if s == nil || s.DB == nil {
		return nil
	}
	return s.DB.Close()
}
