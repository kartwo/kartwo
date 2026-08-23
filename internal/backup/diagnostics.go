// 备份诊断 / Backup Diagnostics
// 功能：统计本地自动备份与升级快照，供后台只读诊断页展示
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-23 13:45:00
package backup

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Usage 是本地保护点的可观测状态；只统计程序明确命名的 ZIP。
type Usage struct {
	AutomaticCount int        `json:"automatic_count"`
	UpgradeCount   int        `json:"upgrade_count"`
	TotalBytes     int64      `json:"total_bytes"`
	LatestAt       *time.Time `json:"latest_at"`
	Message        string     `json:"message"`
}

// Diagnostics 读取 dataDir/backups 下的自动备份和升级快照。目录尚不存在并非异常。
func Diagnostics(dataDir string) Usage {
	entries, err := os.ReadDir(filepath.Join(dataDir, "backups"))
	if os.IsNotExist(err) {
		return Usage{}
	}
	if err != nil {
		return Usage{Message: "暂时无法读取本地备份目录"}
	}
	var out Usage
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !isProtectedBackup(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			out.Message = "部分本地备份文件无法读取"
			continue
		}
		out.TotalBytes += info.Size()
		if strings.HasPrefix(entry.Name(), "kartwo-backup-") {
			out.AutomaticCount++
		} else {
			out.UpgradeCount++
		}
		modified := info.ModTime().UTC()
		if out.LatestAt == nil || modified.After(*out.LatestAt) {
			out.LatestAt = &modified
		}
	}
	return out
}

func isProtectedBackup(name string) bool {
	return strings.HasSuffix(name, ".zip") &&
		(strings.HasPrefix(name, "kartwo-backup-") || strings.HasPrefix(name, "kartwo-upgrade-"))
}
