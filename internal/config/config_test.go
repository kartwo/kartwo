// 配置加载测试 / Configuration Loader Tests
// 功能：验证本地自动备份的默认值与环境变量校验
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-23 10:40:00
package config

import (
	"testing"
	"time"
)

func TestLoadBackupDefaultsAndOverrides(t *testing.T) {
	t.Setenv("KARTWO_BACKUP_INTERVAL", "90m")
	t.Setenv("KARTWO_BACKUP_RETENTION", "12")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if cfg.BackupInterval != 90*time.Minute || cfg.BackupRetention != 12 {
		t.Fatalf("备份配置错误: interval=%s retention=%d", cfg.BackupInterval, cfg.BackupRetention)
	}
}

func TestLoadRejectsInvalidBackupConfig(t *testing.T) {
	t.Setenv("KARTWO_BACKUP_INTERVAL", "30s")
	if _, err := Load(); err == nil {
		t.Fatal("小于一分钟的备份周期应被拒绝")
	}
	t.Setenv("KARTWO_BACKUP_INTERVAL", "1h")
	t.Setenv("KARTWO_BACKUP_RETENTION", "0")
	if _, err := Load(); err == nil {
		t.Fatal("保留数为零应被拒绝")
	}
}

func TestLoadRejectsUnsafeWebDAVURL(t *testing.T) {
	t.Setenv("KARTWO_BACKUP_WEBDAV_URL", "http://example.com/backup")
	if _, err := Load(); err == nil {
		t.Fatal("明文 WebDAV 地址应被拒绝")
	}
}
