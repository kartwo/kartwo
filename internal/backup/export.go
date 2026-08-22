// 全量导出 / Full Export
// 功能：生成含 SQLite 一致性快照与媒体文件的临时 ZIP，不包含证书缓存
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-22 13:00:00
package backup

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const exportFormatVersion = 1

// Manifest 写入导出包，供后续恢复流程识别格式和创建时间。
type Manifest struct {
	FormatVersion int       `json:"format_version"`
	CreatedAt     time.Time `json:"created_at"`
	AppVersion    string    `json:"app_version"`
}

// Exporter 生成自部署数据目录的可移植导出包。
type Exporter struct {
	db      *sql.DB
	dataDir string
	version string
}

// New 构造导出服务。dataDir 是当前实例的持久数据目录。
func New(db *sql.DB, dataDir, version string) *Exporter {
	return &Exporter{db: db, dataDir: dataDir, version: version}
}

// Create 生成 ZIP 到 dataDir/backups 下的临时文件。调用方完成传输后必须 Remove 返回路径。
func (e *Exporter) Create(ctx context.Context) (string, Manifest, error) {
	workDir := filepath.Join(e.dataDir, "backups")
	if err := os.MkdirAll(workDir, 0o750); err != nil {
		return "", Manifest{}, fmt.Errorf("backup: 创建工作目录失败: %w", err)
	}
	dbSnapshot, err := temporaryPath(workDir, ".export-db-*.sqlite")
	if err != nil {
		return "", Manifest{}, err
	}
	defer func() { _ = os.Remove(dbSnapshot) }()
	if _, err := e.db.ExecContext(ctx, "VACUUM INTO ?", dbSnapshot); err != nil {
		return "", Manifest{}, fmt.Errorf("backup: 创建数据库快照失败: %w", err)
	}

	zipPath, err := temporaryPath(workDir, ".export-*.zip")
	if err != nil {
		return "", Manifest{}, err
	}
	manifest := Manifest{FormatVersion: exportFormatVersion, CreatedAt: time.Now().UTC(), AppVersion: e.version}
	if err := e.writeZIP(zipPath, dbSnapshot, manifest); err != nil {
		_ = os.Remove(zipPath)
		return "", Manifest{}, err
	}
	return zipPath, manifest, nil
}

func temporaryPath(dir, pattern string) (string, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("backup: 创建临时文件失败: %w", err)
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("backup: 关闭临时文件失败: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return "", fmt.Errorf("backup: 准备临时文件失败: %w", err)
	}
	return path, nil
}

func (e *Exporter) writeZIP(zipPath, dbSnapshot string, manifest Manifest) error {
	f, err := os.OpenFile(zipPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // zipPath 仅由 temporaryPath 在受控数据目录生成。
	if err != nil {
		return fmt.Errorf("backup: 创建导出包失败: %w", err)
	}
	zw := zip.NewWriter(f)
	if err := writeJSON(zw, "manifest.json", manifest); err != nil {
		_ = zw.Close()
		_ = f.Close()
		return err
	}
	if err := writeFile(zw, "shop.db", dbSnapshot); err != nil {
		_ = zw.Close()
		_ = f.Close()
		return err
	}
	if err := e.writeMedia(zw); err != nil {
		_ = zw.Close()
		_ = f.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		_ = f.Close()
		return fmt.Errorf("backup: 写入导出包失败: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("backup: 关闭导出包失败: %w", err)
	}
	return nil
}

func writeJSON(zw *zip.Writer, name string, value any) error {
	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("backup: 创建 %s 失败: %w", name, err)
	}
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return fmt.Errorf("backup: 写入 %s 失败: %w", name, err)
	}
	return nil
}

func writeFile(zw *zip.Writer, name, source string) error {
	in, err := os.Open(source) //nolint:gosec // source 仅来自受控数据目录或导出快照。
	if err != nil {
		return fmt.Errorf("backup: 打开 %s 失败: %w", name, err)
	}
	defer func() { _ = in.Close() }()
	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("backup: 创建 %s 失败: %w", name, err)
	}
	if _, err := io.Copy(w, in); err != nil {
		return fmt.Errorf("backup: 写入 %s 失败: %w", name, err)
	}
	return nil
}

func (e *Exporter) writeMedia(zw *zip.Writer) error {
	root := filepath.Join(e.dataDir, "media")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("backup: 读取媒体目录失败: %w", err)
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("backup: 媒体目录不允许符号链接: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("backup: 媒体目录包含非普通文件: %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("backup: 计算媒体相对路径失败: %w", err)
		}
		return writeFile(zw, "media/"+strings.ReplaceAll(rel, string(filepath.Separator), "/"), path)
	})
}
