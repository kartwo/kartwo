// 本地备份调度器测试 / Local Backup Scheduler Tests
// 功能：验证持久备份原子落位与只清理程序命名的旧备份
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-23 10:40:00
package backup

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/migrations"

	_ "modernc.org/sqlite"
)

func TestCreatePersistentAndPrune(t *testing.T) {
	dataDir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "shop.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := migrate.Run(context.Background(), db, migrations.FS); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	exporter := New(db, dataDir, "test-version")
	firstAt := time.Date(2026, 8, 23, 1, 0, 0, 0, time.UTC)
	exporter.now = func() time.Time { return firstAt }
	first, manifest, err := exporter.CreatePersistent(context.Background())
	if err != nil {
		t.Fatalf("创建持久备份失败: %v", err)
	}
	if manifest.CreatedAt != firstAt || filepath.Base(first) != persistentName(firstAt) {
		t.Fatalf("持久备份结果错误: path=%s manifest=%+v", first, manifest)
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatalf("持久备份未落盘: %v", err)
	}

	backupDir := filepath.Join(dataDir, "backups")
	second := filepath.Join(backupDir, persistentName(firstAt.Add(time.Hour)))
	third := filepath.Join(backupDir, persistentName(firstAt.Add(2*time.Hour)))
	manual := filepath.Join(backupDir, "operator-copy.zip")
	for _, path := range []string{second, third, manual} {
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatalf("写入备份夹具失败: %v", err)
		}
	}
	if err := PrunePersistent(dataDir, 2); err != nil {
		t.Fatalf("清理旧备份失败: %v", err)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("最旧自动备份应被清理: %v", err)
	}
	for _, path := range []string{second, third, manual} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("不应删除 %s: %v", filepath.Base(path), err)
		}
	}
}

func TestPrunePersistentRejectsZeroRetention(t *testing.T) {
	if err := PrunePersistent(t.TempDir(), 0); err == nil {
		t.Fatal("保留数为零应被拒绝")
	}
}
