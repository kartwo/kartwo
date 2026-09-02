// 本地备份调度器测试 / Local Backup Scheduler Tests
// 功能：验证持久备份原子落位与只清理程序命名的旧备份
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-23 10:40:00
package backup

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
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

type fakeUploader struct {
	paths []string
	err   error

	tested bool
}

func (u *fakeUploader) Upload(_ context.Context, sourcePath string) error {
	u.paths = append(u.paths, sourcePath)
	return u.err
}

func (u *fakeUploader) Test(context.Context, string) error {
	u.tested = true
	return u.err
}

func TestSchedulerUploadFailureRecords(t *testing.T) {
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
	exporter.now = func() time.Time { return time.Date(2026, 8, 25, 1, 0, 0, 0, time.UTC) }
	uploader := &fakeUploader{err: errors.New("broken")}
	s := NewScheduler(exporter, time.Minute, 7, slog.Default(), uploader)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.tick(ctx)

	if len(uploader.paths) != 1 {
		t.Fatalf("应尝试上传 1 次，得 %d", len(uploader.paths))
	}
	if uploader.paths[0] == "" {
		t.Fatal("上传源路径不能为空")
	}
	if _, _, msg := s.RemoteStatus(); msg != "broken" {
		t.Fatalf("错误上报应保留原文，得 %q", msg)
	}
}

func TestSchedulerUploadSuccess(t *testing.T) {
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
	exporter.now = func() time.Time { return time.Date(2026, 8, 25, 1, 0, 0, 0, time.UTC) }
	uploader := &fakeUploader{}
	s := NewScheduler(exporter, time.Minute, 7, slog.Default(), uploader)
	s.tick(context.Background())

	if len(uploader.paths) != 1 {
		t.Fatalf("应尝试上传 1 次，得 %d", len(uploader.paths))
	}
	if _, _, msg := s.RemoteStatus(); msg != "" {
		t.Fatalf("成功应不保留错误消息: %q", msg)
	}
}

func TestPrunePersistentRejectsZeroRetention(t *testing.T) {
	if err := PrunePersistent(t.TempDir(), 0); err == nil {
		t.Fatal("保留数为零应被拒绝")
	}
}

func TestPrunePersistentAllowsFewerFilesThanRetention(t *testing.T) {
	dataDir := t.TempDir()
	backupDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupDir, 0o750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(backupDir, "kartwo-backup-20260823T010000Z.zip")
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := PrunePersistent(dataDir, 7); err != nil {
		t.Fatalf("备份数少于保留数不应失败: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("现有备份不应被删除: %v", err)
	}
}
