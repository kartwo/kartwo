// 迁移执行器测试 / Migration Runner Tests
// 功能：验证迁移按序应用、幂等可重入、失败回滚（核心逻辑必须单测）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 17:05:46
package migrate

import (
	"context"
	"database/sql"
	"testing"
	"testing/fstest"

	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	// 每个测试独立的临时文件库，避免共享内存连接的生命周期问题。
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/t.db?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("打开测试库失败: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestRun_AppliesAndIsIdempotent(t *testing.T) {
	db := newTestDB(t)
	fsys := fstest.MapFS{
		"0001_a.sql": {Data: []byte(`CREATE TABLE a (id INTEGER PRIMARY KEY);`)},
		"0002_b.sql": {Data: []byte(`CREATE TABLE b (id INTEGER PRIMARY KEY);`)},
		"notes.txt":  {Data: []byte(`忽略非 .sql 文件`)},
	}

	ctx := context.Background()
	n, err := Run(ctx, db, fsys)
	if err != nil {
		t.Fatalf("首次 Run 失败: %v", err)
	}
	if n != 2 {
		t.Fatalf("首次应用数量 = %d，期望 2", n)
	}

	// 表确实建立。
	for _, table := range []string{"a", "b"} {
		var name string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		if err != nil {
			t.Fatalf("表 %s 未建立: %v", table, err)
		}
	}

	// 再次 Run：幂等，应用 0 条且不报错。
	n2, err := Run(ctx, db, fsys)
	if err != nil {
		t.Fatalf("第二次 Run 失败: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("第二次应用数量 = %d，期望 0（幂等）", n2)
	}
}

func TestRun_AppliesNewlyAddedMigration(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	base := fstest.MapFS{"0001_a.sql": {Data: []byte(`CREATE TABLE a (id INTEGER PRIMARY KEY);`)}}
	if _, err := Run(ctx, db, base); err != nil {
		t.Fatalf("基线 Run 失败: %v", err)
	}

	// 新增一条迁移后只应用增量。
	withNew := fstest.MapFS{
		"0001_a.sql": {Data: []byte(`CREATE TABLE a (id INTEGER PRIMARY KEY);`)},
		"0002_c.sql": {Data: []byte(`CREATE TABLE c (id INTEGER PRIMARY KEY);`)},
	}
	n, err := Run(ctx, db, withNew)
	if err != nil {
		t.Fatalf("增量 Run 失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("增量应用数量 = %d，期望 1", n)
	}
}

func TestRun_FailedMigrationRollsBack(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// 第二条迁移含非法 SQL：应整体失败，且不记录版本。
	fsys := fstest.MapFS{
		"0001_ok.sql":  {Data: []byte(`CREATE TABLE ok (id INTEGER PRIMARY KEY);`)},
		"0002_bad.sql": {Data: []byte(`THIS IS NOT VALID SQL;`)},
	}
	n, err := Run(ctx, db, fsys)
	if err == nil {
		t.Fatalf("期望失败，却成功（应用 %d 条）", n)
	}
	if n != 1 {
		t.Fatalf("失败前应用数量 = %d，期望 1（0001 成功，0002 失败）", n)
	}

	// 0002 未被记录，修正后可重试。
	var cnt int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version='0002_bad'`).Scan(&cnt); err != nil {
		t.Fatalf("查询迁移记录失败: %v", err)
	}
	if cnt != 0 {
		t.Fatalf("失败迁移不应被记录，count = %d", cnt)
	}
}
