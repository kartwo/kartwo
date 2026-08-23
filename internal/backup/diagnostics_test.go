// 备份诊断测试 / Backup Diagnostics Tests
// 功能：验证诊断只统计受保护的自动备份和升级快照
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-23 13:45:00
package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiagnosticsCountsProtectedBackups(t *testing.T) {
	dataDir := t.TempDir()
	dir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"kartwo-backup-20260823T010000Z.zip":  "auto",
		"kartwo-upgrade-20260823T020000Z.zip": "upgrade",
		"manual-export.zip":                   "ignored",
		"kartwo-backup-partial.uploading":     "ignored",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	latest := time.Date(2026, 8, 23, 2, 0, 0, 0, time.UTC)
	earlier := latest.Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "kartwo-backup-20260823T010000Z.zip"), earlier, earlier); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(dir, "kartwo-upgrade-20260823T020000Z.zip"), latest, latest); err != nil {
		t.Fatal(err)
	}
	got := Diagnostics(dataDir)
	if got.AutomaticCount != 1 || got.UpgradeCount != 1 || got.TotalBytes != int64(len("auto")+len("upgrade")) {
		t.Fatalf("诊断统计错误: %+v", got)
	}
	if got.LatestAt == nil || !got.LatestAt.Equal(latest) || got.Message != "" {
		t.Fatalf("最新时间或状态错误: %+v", got)
	}
}

func TestDiagnosticsMissingDirectoryIsEmpty(t *testing.T) {
	if got := Diagnostics(t.TempDir()); got.AutomaticCount != 0 || got.UpgradeCount != 0 || got.Message != "" {
		t.Fatalf("目录不存在应为空状态: %+v", got)
	}
}
