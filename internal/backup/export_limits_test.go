// 备份容量边界测试 / Backup Capacity Boundary Tests
// 功能：验证导出不会将恢复容量限制拒绝的数据报告为成功备份
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-26 15:54:55
package backup

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateRejectsMediaAboveRestoreFileLimit(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "shop.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err = db.Exec(`CREATE TABLE fixture (value TEXT); INSERT INTO fixture VALUES ('source-preserved')`); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "media")
	if err = os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "oversized-fixture.bin")
	f, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(maxRestoreFile + 1); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	exporter := New(db, dir, "capacity-test")
	for _, create := range []struct {
		name string
		run  func(context.Context) (string, Manifest, error)
	}{
		{"download", exporter.Create}, {"automatic", exporter.CreatePersistent}, {"upgrade", exporter.CreateUpgradeSnapshot},
	} {
		t.Run(create.name, func(t *testing.T) {
			archive, _, err := create.run(context.Background())
			if err == nil {
				defer func() { _ = os.Remove(archive) }()
				_, restoreErr := Restore(archive, filepath.Join(t.TempDir(), "restored"))
				t.Fatalf("oversized media reported successful backup: restore_error=%v", restoreErr)
			}
			if archive != "" || !strings.Contains(err.Error(), "条目过大") {
				t.Fatal("export did not reject the file size at its existing boundary", err)
			}
		})
	}
	if entries, err := os.ReadDir(filepath.Join(dir, "backups")); err != nil || len(entries) != 0 {
		t.Fatal("failed export left backup artifacts", err)
	}
	if info, err := os.Stat(source); err != nil || info.Size() != maxRestoreFile+1 {
		t.Fatal("failed export changed source media", err)
	}
	var value string
	if err = db.QueryRow(`SELECT value FROM fixture`).Scan(&value); err != nil || value != "source-preserved" {
		t.Fatal("failed export changed source database", err)
	}
}

func TestCreateRejectsTotalIncludingDatabaseAndManifest(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "shop.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err = db.Exec(`CREATE TABLE fixture (value TEXT); INSERT INTO fixture VALUES ('source-preserved')`); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "media")
	if err = os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	// Sparse files exercise the actual format boundary without allocating 4 GiB.
	var sources []string
	for i := range 4 {
		source := filepath.Join(root, fmt.Sprintf("boundary-%d.bin", i))
		f, err := os.Create(source)
		if err != nil {
			t.Fatal(err)
		}
		if err = f.Truncate(maxRestoreFile); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
		sources = append(sources, source)
	}
	exporter := New(db, dir, "total-capacity-test")
	archive, _, err := exporter.Create(context.Background())
	if err == nil {
		defer func() { _ = os.Remove(archive) }()
		_, restoreErr := Restore(archive, filepath.Join(t.TempDir(), "restored"))
		t.Fatalf("4 GiB media plus database/manifest reported success: restore_error=%v", restoreErr)
	}
	if archive != "" || !strings.Contains(err.Error(), "解压后过大") {
		t.Fatal("total limit did not include database and manifest", err)
	}
	if entries, err := os.ReadDir(filepath.Join(dir, "backups")); err != nil || len(entries) != 0 {
		t.Fatal("rejected total left backup artifacts", err)
	}
	for _, source := range sources {
		if info, err := os.Stat(source); err != nil || info.Size() != maxRestoreFile {
			t.Fatal("failed export changed source media", err)
		}
		if err = os.WriteFile(source, []byte("repaired-media"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, create := range []struct {
		name string
		run  func(context.Context) (string, Manifest, error)
	}{
		{"download", exporter.Create}, {"automatic", exporter.CreatePersistent}, {"upgrade", exporter.CreateUpgradeSnapshot},
	} {
		t.Run(create.name, func(t *testing.T) {
			archive, _, err := create.run(context.Background())
			if err != nil {
				t.Fatal("repaired capacity did not recover backup", err)
			}
			defer func() { _ = os.Remove(archive) }()
			target := filepath.Join(t.TempDir(), "restored-repaired")
			result, err := Restore(archive, target)
			if err != nil || result.MediaFiles != 4 {
				t.Fatal("repaired export not restorable", result, err)
			}
			for _, source := range sources {
				if got, err := os.ReadFile(filepath.Join(target, "media", filepath.Base(source))); err != nil || string(got) != "repaired-media" {
					t.Fatal("repaired backup lost media", err)
				}
			}
			restored, err := sql.Open("sqlite", "file:"+filepath.Join(target, "shop.db")+"?mode=ro")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = restored.Close() }()
			var value string
			if err = restored.QueryRow(`SELECT value FROM fixture`).Scan(&value); err != nil || value != "source-preserved" {
				t.Fatal("repaired backup lost database fact", err)
			}
		})
	}
}

func TestCompletedExportCountIncludesManifestAndDatabase(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "too-many-entries.zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	zw := zip.NewWriter(f)
	if err = writeJSON(zw, "manifest.json", Manifest{FormatVersion: exportFormatVersion}); err != nil {
		t.Fatal(err)
	}
	if _, err = zw.CreateRaw(&zip.FileHeader{Name: "shop.db", Method: zip.Store}); err != nil {
		t.Fatal(err)
	}
	for i := range maxRestoreEntries - 1 {
		if _, err = zw.CreateRaw(&zip.FileHeader{Name: fmt.Sprintf("media/%06d", i), Method: zip.Store}); err != nil {
			t.Fatal(err)
		}
	}
	if err = zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if err = validateCompletedExport(archive); err == nil || !strings.Contains(err.Error(), "条目数量") {
		t.Fatal("completed export accepted too many entries", err)
	}
	target := filepath.Join(t.TempDir(), "restored")
	if _, err = Restore(archive, target); err == nil || !strings.Contains(err.Error(), "条目数量") {
		t.Fatal("restore capacity differs from completed export", err)
	}
	if _, err = os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("rejected count exposed a restore destination")
	}
	zr, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zr.Close() }()
	if len(zr.File) != maxRestoreEntries+1 {
		t.Fatal("count fixture did not reach the actual boundary")
	}
	// This is metadata validation only; it does not restore an empty shop.db.
	if _, err = validateRestoreArchive(zr.File[:maxRestoreEntries]); err != nil {
		t.Fatal("exact count boundary must remain inclusive", err)
	}
}

func TestExportPlanTotalBoundaryIncludesEncodedManifest(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shop.db")
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err = db.Exec(`CREATE TABLE fixture (value TEXT)`); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{FormatVersion: exportFormatVersion, AppVersion: "exact-capacity-test"}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	dbInfo, err := os.Stat(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	overhead := dbInfo.Size() + int64(len(body)) + 1
	root := filepath.Join(dir, "media")
	if err = os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	var last string
	for i := range 4 {
		size := int64(maxRestoreFile)
		if i == 3 {
			size -= overhead
		}
		last = filepath.Join(root, fmt.Sprintf("boundary-%d.bin", i))
		f, err := os.Create(last)
		if err != nil {
			t.Fatal(err)
		}
		if err = f.Truncate(size); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	// Check the real filesystem plan only, avoiding a 4 GiB copy for a positive boundary.
	exporter := New(db, dir, "exact-capacity-test")
	files, err := exporter.collectExportFiles(dbPath, manifest)
	if err != nil || len(files) != 5 {
		t.Fatal("exact total/file limits must remain inclusive", len(files), err)
	}
	if err = os.Truncate(last, maxRestoreFile-overhead+1); err != nil {
		t.Fatal(err)
	}
	if files, err = exporter.collectExportFiles(dbPath, manifest); err == nil || len(files) != 0 || !strings.Contains(err.Error(), "解压后过大") {
		t.Fatal("one extra byte beyond encoded total was accepted", err)
	}
}
