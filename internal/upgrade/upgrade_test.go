// 升级保护测试 / Upgrade Guard Tests
// 功能：验证既有数据升级前快照、批次迁移与失败回滚边界
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-23 14:05:00
package upgrade

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/kartwo/kartwo/internal/migrate"

	_ "modernc.org/sqlite"
)

func TestApplySnapshotsExistingDatabaseBeforeMigration(t *testing.T) {
	dataDir, db := upgradeTestDB(t)
	base := fstest.MapFS{"0001_base.sql": {Data: []byte(`CREATE TABLE base (id INTEGER PRIMARY KEY);`)}}
	if _, err := migrate.Run(context.Background(), db, base); err != nil {
		t.Fatal(err)
	}
	next := fstest.MapFS{
		"0001_base.sql": {Data: []byte(`CREATE TABLE base (id INTEGER PRIMARY KEY);`)},
		"0002_next.sql": {Data: []byte(`CREATE TABLE next (id INTEGER PRIMARY KEY);`)},
	}
	result, err := Apply(context.Background(), db, dataDir, "test", true, next)
	if err != nil {
		t.Fatalf("升级失败: %v", err)
	}
	if result.Pending != 1 || result.Applied != 1 || result.SnapshotPath == "" {
		t.Fatalf("升级结果错误: %+v", result)
	}
	if _, err := os.Stat(result.SnapshotPath); err != nil {
		t.Fatalf("升级前快照不存在: %v", err)
	}
	var name string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='next'`).Scan(&name); err != nil {
		t.Fatalf("新迁移未应用: %v", err)
	}
}

func TestApplySkipsSnapshotForFirstInstall(t *testing.T) {
	dataDir, db := upgradeTestDB(t)
	fsys := fstest.MapFS{"0001_base.sql": {Data: []byte(`CREATE TABLE base (id INTEGER PRIMARY KEY);`)}}
	result, err := Apply(context.Background(), db, dataDir, "test", false, fsys)
	if err != nil {
		t.Fatalf("首次安装失败: %v", err)
	}
	if result.Applied != 1 || result.SnapshotPath != "" {
		t.Fatalf("首次安装不应创建升级快照: %+v", result)
	}
}

func TestApplyFailedBatchKeepsSnapshotAndRollsBack(t *testing.T) {
	dataDir, db := upgradeTestDB(t)
	base := fstest.MapFS{"0001_base.sql": {Data: []byte(`CREATE TABLE base (id INTEGER PRIMARY KEY);`)}}
	if _, err := migrate.Run(context.Background(), db, base); err != nil {
		t.Fatal(err)
	}
	broken := fstest.MapFS{
		"0001_base.sql":  {Data: []byte(`CREATE TABLE base (id INTEGER PRIMARY KEY);`)},
		"0002_first.sql": {Data: []byte(`CREATE TABLE first (id INTEGER PRIMARY KEY);`)},
		"0003_bad.sql":   {Data: []byte(`NOT VALID SQL;`)},
	}
	result, err := Apply(context.Background(), db, dataDir, "test", true, broken)
	if err == nil {
		t.Fatal("损坏迁移应失败")
	}
	if result.SnapshotPath == "" {
		t.Fatalf("失败前应保留快照: %+v", result)
	}
	if _, err := os.Stat(result.SnapshotPath); err != nil {
		t.Fatalf("失败后的快照不存在: %v", err)
	}
	var name string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='first'`).Scan(&name); err != sql.ErrNoRows {
		t.Fatalf("失败批次不应留下 first 表: %v", err)
	}
}

func upgradeTestDB(t *testing.T) (string, *sql.DB) {
	t.Helper()
	dataDir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "shop.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return dataDir, db
}
