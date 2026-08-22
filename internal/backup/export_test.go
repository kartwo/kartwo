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
