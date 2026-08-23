// 数据恢复 / Data Restore
// 功能：校验全量导出 ZIP 并原子恢复到一个全新的数据目录
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-23 09:20:00
package backup

import (
	"archive/zip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const (
	maxRestoreEntries = 100000
	maxRestoreFile    = 1 << 30
	maxRestoreTotal   = 4 << 30
)

// RestoreResult 描述一次成功恢复的结果。
type RestoreResult struct {
	Manifest   Manifest
	MediaFiles int
}

// Restore 将 ZIP 恢复到不存在的 dataDir。它绝不覆盖既有目录，所有文件先解压到同盘临时目录，
// 通过校验后才原子改名为目标目录。
func Restore(zipPath, dataDir string) (RestoreResult, error) {
	if err := ensureNewDataDir(dataDir); err != nil {
		return RestoreResult{}, err
	}
	zr, err := zip.OpenReader(zipPath) //nolint:gosec // ZIP 路径由本机运维显式传入，后续严格校验条目。
	if err != nil {
		return RestoreResult{}, fmt.Errorf("backup: 打开导出包失败: %w", err)
	}
	defer func() { _ = zr.Close() }()
	manifest, err := validateRestoreArchive(zr.File)
	if err != nil {
		return RestoreResult{}, err
	}

	parent := filepath.Dir(filepath.Clean(dataDir))
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return RestoreResult{}, fmt.Errorf("backup: 创建恢复目录失败: %w", err)
	}
	stage, err := os.MkdirTemp(parent, ".kartwo-restore-")
	if err != nil {
		return RestoreResult{}, fmt.Errorf("backup: 创建恢复临时目录失败: %w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }()
	if err := os.Chmod(stage, 0o700); err != nil { //nolint:gosec // stage 是仅当前进程可访问的目录，目录需保留执行位以访问内部恢复文件。
		return RestoreResult{}, fmt.Errorf("backup: 设置恢复临时目录权限失败: %w", err)
	}

	mediaFiles, err := extractRestoreArchive(zr.File, stage)
	if err != nil {
		return RestoreResult{}, err
	}
	if err := verifyRestoredDatabase(filepath.Join(stage, "shop.db")); err != nil {
		return RestoreResult{}, err
	}
	if err := os.Rename(stage, dataDir); err != nil {
		return RestoreResult{}, fmt.Errorf("backup: 落位恢复数据失败: %w", err)
	}
	return RestoreResult{Manifest: manifest, MediaFiles: mediaFiles}, nil
}

func ensureNewDataDir(dataDir string) error {
	clean := filepath.Clean(dataDir)
	if clean == "." || clean == string(filepath.Separator) {
		return fmt.Errorf("backup: 恢复目标必须是一个新的数据目录")
	}
	if _, err := os.Lstat(clean); err == nil {
		return fmt.Errorf("backup: 恢复目标已存在，拒绝覆盖: %s", clean)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("backup: 检查恢复目标失败: %w", err)
	}
	return nil
}

func validateRestoreArchive(files []*zip.File) (Manifest, error) {
	if len(files) == 0 || len(files) > maxRestoreEntries {
		return Manifest{}, fmt.Errorf("backup: 导出包条目数量不合法")
	}
	var (
		manifest     Manifest
		manifestSeen bool
		databaseSeen bool
		total        uint64
		seen         = make(map[string]struct{}, len(files))
	)
	for _, file := range files {
		name, err := restoreEntryName(file)
		if err != nil {
			return Manifest{}, err
		}
		if _, duplicate := seen[name]; duplicate {
			return Manifest{}, fmt.Errorf("backup: 导出包含重复条目: %s", name)
		}
		seen[name] = struct{}{}
		if file.UncompressedSize64 > maxRestoreFile {
			return Manifest{}, fmt.Errorf("backup: 导出包条目过大: %s", name)
		}
		total += file.UncompressedSize64
		if total > maxRestoreTotal {
			return Manifest{}, fmt.Errorf("backup: 导出包解压后过大")
		}
		switch {
		case name == "manifest.json":
			body, err := readRestoreEntry(file)
			if err != nil {
				return Manifest{}, err
			}
			if err := json.Unmarshal(body, &manifest); err != nil {
				return Manifest{}, fmt.Errorf("backup: manifest 格式错误: %w", err)
			}
			manifestSeen = true
		case name == "shop.db":
			databaseSeen = true
		case strings.HasPrefix(name, "media/"):
		default:
			return Manifest{}, fmt.Errorf("backup: 导出包包含不支持的条目: %s", name)
		}
	}
	if !manifestSeen || !databaseSeen {
		return Manifest{}, fmt.Errorf("backup: 导出包缺少 manifest.json 或 shop.db")
	}
	if manifest.FormatVersion != exportFormatVersion {
		return Manifest{}, fmt.Errorf("backup: 不支持的导出格式版本: %d", manifest.FormatVersion)
	}
	return manifest, nil
}

func restoreEntryName(file *zip.File) (string, error) {
	if !file.Mode().IsRegular() {
		return "", fmt.Errorf("backup: 导出包包含非普通文件: %s", file.Name)
	}
	name := file.Name
	if name == "" || strings.HasPrefix(name, "/") || path.Clean(name) != name || strings.Contains(name, "\\") {
		return "", fmt.Errorf("backup: 导出包路径不安全: %s", file.Name)
	}
	return name, nil
}

func extractRestoreArchive(files []*zip.File, stage string) (int, error) {
	mediaFiles := 0
	for _, file := range files {
		name, err := restoreEntryName(file)
		if err != nil {
			return 0, err
		}
		target := filepath.Join(stage, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return 0, fmt.Errorf("backup: 创建恢复文件目录失败: %w", err)
		}
		if err := writeRestoreEntry(file, target); err != nil {
			return 0, err
		}
		if strings.HasPrefix(name, "media/") {
			mediaFiles++
		}
	}
	return mediaFiles, nil
}

func readRestoreEntry(file *zip.File) ([]byte, error) {
	r, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("backup: 打开导出条目失败: %w", err)
	}
	defer func() { _ = r.Close() }()
	body, err := io.ReadAll(io.LimitReader(r, maxRestoreFile+1))
	if err != nil {
		return nil, fmt.Errorf("backup: 读取导出条目失败: %w", err)
	}
	if int64(len(body)) > maxRestoreFile {
		return nil, fmt.Errorf("backup: 导出条目过大")
	}
	return body, nil
}

func writeRestoreEntry(file *zip.File, target string) error {
	r, err := file.Open()
	if err != nil {
		return fmt.Errorf("backup: 打开导出条目失败: %w", err)
	}
	defer func() { _ = r.Close() }()
	w, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // target 由已校验的 ZIP 相对路径派生。
	if err != nil {
		return fmt.Errorf("backup: 创建恢复文件失败: %w", err)
	}
	n, copyErr := io.Copy(w, io.LimitReader(r, maxRestoreFile+1))
	closeErr := w.Close()
	if copyErr != nil {
		return fmt.Errorf("backup: 写入恢复文件失败: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("backup: 关闭恢复文件失败: %w", closeErr)
	}
	if n > maxRestoreFile {
		return fmt.Errorf("backup: 导出条目过大: %s", file.Name)
	}
	return nil
}

func verifyRestoredDatabase(dbPath string) error {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return fmt.Errorf("backup: 打开恢复数据库失败: %w", err)
	}
	defer func() { _ = db.Close() }()
	var result string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("backup: 校验恢复数据库失败: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("backup: 恢复数据库完整性校验失败: %s", result)
	}
	return nil
}
