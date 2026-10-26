// 全量导出测试 / Full Export Tests
// 功能：验证 SQLite 一致性快照、媒体打包和证书缓存排除
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-22 13:10:00
package backup

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/migrations"

	_ "modernc.org/sqlite"
)

func TestCreateIncludesSnapshotAndMediaButNotCertificates(t *testing.T) {
	dataDir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "shop.db")+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := migrate.Run(context.Background(), db, migrations.FS); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO meta (key, value) VALUES ('export_test', 'snapshot-value')`); err != nil {
		t.Fatalf("写入验收数据失败: %v", err)
	}
	mediaPath := filepath.Join(dataDir, "media", "original", "cover.jpg")
	if err := os.MkdirAll(filepath.Dir(mediaPath), 0o750); err != nil {
		t.Fatalf("创建媒体目录失败: %v", err)
	}
	if err := os.WriteFile(mediaPath, []byte("media-content"), 0o600); err != nil {
		t.Fatalf("写入媒体文件失败: %v", err)
	}
	certPath := filepath.Join(dataDir, "certs", "private-key.pem")
	if err := os.MkdirAll(filepath.Dir(certPath), 0o750); err != nil {
		t.Fatalf("创建证书目录失败: %v", err)
	}
	if err := os.WriteFile(certPath, []byte("must-not-export"), 0o600); err != nil {
		t.Fatalf("写入证书文件失败: %v", err)
	}

	path, manifest, err := New(db, dataDir, "test-version").Create(context.Background())
	if err != nil {
		t.Fatalf("创建导出包失败: %v", err)
	}
	defer func() { _ = os.Remove(path) }()
	if manifest.FormatVersion != exportFormatVersion || manifest.AppVersion != "test-version" {
		t.Fatalf("manifest = %+v", manifest)
	}

	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("打开 ZIP 失败: %v", err)
	}
	defer func() { _ = zr.Close() }()
	entries := map[string]*zip.File{}
	for _, file := range zr.File {
		entries[file.Name] = file
	}
	for _, name := range []string{"manifest.json", "shop.db", "media/original/cover.jpg"} {
		if entries[name] == nil {
			t.Fatalf("导出包缺少 %s", name)
		}
	}
	if entries["certs/private-key.pem"] != nil {
		t.Fatal("导出包不应包含证书缓存")
	}

	manifestBody := readZipFile(t, entries["manifest.json"])
	var exportedManifest Manifest
	if err := json.Unmarshal(manifestBody, &exportedManifest); err != nil || exportedManifest.AppVersion != "test-version" {
		t.Fatalf("manifest 内容错误: err=%v manifest=%+v", err, exportedManifest)
	}
	if got := string(readZipFile(t, entries["media/original/cover.jpg"])); got != "media-content" {
		t.Fatalf("媒体内容 = %q", got)
	}

	snapshotPath := filepath.Join(t.TempDir(), "restored-shop.db")
	if err := os.WriteFile(snapshotPath, readZipFile(t, entries["shop.db"]), 0o600); err != nil {
		t.Fatalf("写出数据库快照失败: %v", err)
	}
	snapshotDB, err := sql.Open("sqlite", "file:"+snapshotPath)
	if err != nil {
		t.Fatalf("打开数据库快照失败: %v", err)
	}
	defer func() { _ = snapshotDB.Close() }()
	var value string
	if err := snapshotDB.QueryRow(`SELECT value FROM meta WHERE key = 'export_test'`).Scan(&value); err != nil || value != "snapshot-value" {
		t.Fatalf("快照数据错误: value=%q err=%v", value, err)
	}
}

func readZipFile(t *testing.T, file *zip.File) []byte {
	t.Helper()
	r, err := file.Open()
	if err != nil {
		t.Fatalf("打开 ZIP 条目失败: %v", err)
	}
	defer func() { _ = r.Close() }()
	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("读取 ZIP 条目失败: %v", err)
	}
	return body
}

func TestRestoreRestoresExportIntoNewDataDir(t *testing.T) {
	sourceDir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(sourceDir, "shop.db")+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("打开源数据库失败: %v", err)
	}
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()
	if _, err := migrate.Run(context.Background(), db, migrations.FS); err != nil {
		t.Fatalf("迁移源数据库失败: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO meta (key, value) VALUES ('restore_test', 'restored-value')`); err != nil {
		t.Fatalf("写入源数据失败: %v", err)
	}
	mediaPath := filepath.Join(sourceDir, "media", "originals", "restored.jpg")
	if err := os.MkdirAll(filepath.Dir(mediaPath), 0o750); err != nil {
		t.Fatalf("创建源媒体目录失败: %v", err)
	}
	if err := os.WriteFile(mediaPath, []byte("restored-media"), 0o600); err != nil {
		t.Fatalf("写入源媒体失败: %v", err)
	}
	zipPath, _, err := New(db, sourceDir, "test-version").Create(context.Background())
	if err != nil {
		t.Fatalf("创建源导出包失败: %v", err)
	}
	defer func() { _ = os.Remove(zipPath) }()

	targetDir := filepath.Join(t.TempDir(), "restored-data")
	result, err := Restore(zipPath, targetDir)
	if err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if result.Manifest.AppVersion != "test-version" || result.MediaFiles != 1 {
		t.Fatalf("恢复结果错误: %+v", result)
	}
	if got, err := os.ReadFile(filepath.Join(targetDir, "media", "originals", "restored.jpg")); err != nil || string(got) != "restored-media" {
		t.Fatalf("恢复媒体错误: got=%q err=%v", got, err)
	}
	restoredDB, err := sql.Open("sqlite", "file:"+filepath.Join(targetDir, "shop.db")+"?mode=ro")
	if err != nil {
		t.Fatalf("打开恢复数据库失败: %v", err)
	}
	defer func() { _ = restoredDB.Close() }()
	var value string
	if err := restoredDB.QueryRow(`SELECT value FROM meta WHERE key = 'restore_test'`).Scan(&value); err != nil || value != "restored-value" {
		t.Fatalf("恢复数据库内容错误: value=%q err=%v", value, err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "certs")); !os.IsNotExist(err) {
		t.Fatalf("恢复目录不应包含证书缓存: %v", err)
	}
}

func TestRestoreRejectsExistingTargetAndUnsafeArchive(t *testing.T) {
	existing := t.TempDir()
	if _, err := Restore(filepath.Join(t.TempDir(), "missing.zip"), existing); err == nil {
		t.Fatal("已存在的恢复目标应被拒绝")
	}
	zipPath := filepath.Join(t.TempDir(), "unsafe.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("创建测试 ZIP 失败: %v", err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("../escape")
	if err != nil {
		t.Fatalf("创建不安全条目失败: %v", err)
	}
	if _, err := w.Write([]byte("no")); err != nil {
		t.Fatalf("写入不安全条目失败: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("关闭测试 ZIP 失败: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("关闭测试文件失败: %v", err)
	}
	if _, err := Restore(zipPath, filepath.Join(t.TempDir(), "target")); err == nil {
		t.Fatal("路径穿越 ZIP 应被拒绝")
	}
}

func TestCreateRejectsNonDirectoryMediaRoot(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "shop.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()
	if _, err = db.Exec(`CREATE TABLE fixture (value TEXT); INSERT INTO fixture VALUES ('source-preserved')`); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "media")
	marker := []byte("virtual-file-mistaken-for-media-directory")
	if err = os.WriteFile(root, marker, 0600); err != nil {
		t.Fatal(err)
	}
	exporter := New(db, dir, "media-root-test")
	for _, tc := range []struct {
		name   string
		create func(context.Context) (string, Manifest, error)
	}{
		{"download", exporter.Create}, {"automatic", exporter.CreatePersistent}, {"upgrade", exporter.CreateUpgradeSnapshot},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, _, e := tc.create(context.Background())
			if e == nil {
				defer func() { _ = os.Remove(path) }()
				zr, openErr := zip.OpenReader(path)
				var names []string
				if openErr == nil {
					for _, entry := range zr.File {
						names = append(names, entry.Name)
					}
					_ = zr.Close()
				}
				_, restoreErr := Restore(path, filepath.Join(t.TempDir(), "restored"))
				t.Errorf("non-directory media unexpectedly exported; entries=%v restore_error=%v", names, restoreErr)
			} else if path != "" {
				t.Fatal("failed export returned a usable archive path")
			}
		})
	}
	if leftovers, err := filepath.Glob(filepath.Join(dir, "backups", "*")); err != nil || len(leftovers) != 0 {
		t.Fatal("invalid export left files", err)
	}
	if got, err := os.ReadFile(root); err != nil || string(got) != string(marker) {
		t.Fatal("failure changed original media path", err)
	}
	var value string
	if err = db.QueryRow(`SELECT value FROM fixture`).Scan(&value); err != nil || value != "source-preserved" {
		t.Fatal("failure changed source database", err)
	}
	if err = os.Remove(root); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "repaired.txt"), []byte("repaired-media"), 0600); err != nil {
		t.Fatal(err)
	}
	path, _, err := exporter.Create(context.Background())
	if err != nil {
		t.Fatal("repair did not recover export", err)
	}
	defer func() { _ = os.Remove(path) }()
	target := filepath.Join(t.TempDir(), "restored-repaired")
	if _, err = Restore(path, target); err != nil {
		t.Fatal("repaired export not restorable", err)
	}
	if data, err := os.ReadFile(filepath.Join(target, "media", "repaired.txt")); err != nil || string(data) != "repaired-media" {
		t.Fatal("repaired media lost", err)
	}
}

func TestCreateRejectsDanglingMediaRoot(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "shop.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err = db.Exec(`CREATE TABLE fixture (value TEXT)`); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "media")
	target := filepath.Join(dir, "missing-media-target")
	if err = os.Symlink(target, root); err != nil {
		t.Skipf("symbolic links unavailable on this host: %v", err)
	}
	path, _, err := New(db, dir, "dangling-media-test").Create(context.Background())
	if err == nil {
		_ = os.Remove(path)
		t.Fatal("dangling media root silently treated as an absent media directory")
	}
	info, err := os.Lstat(root)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("failure modified media root", err)
	}
	if _, err = os.Lstat(target); !os.IsNotExist(err) {
		t.Fatal("failure created the missing link target")
	}
	if paths, err := filepath.Glob(filepath.Join(dir, "backups", ".export-*")); err != nil || len(paths) != 0 {
		t.Fatal("failed export left temporary files", err)
	}
}
