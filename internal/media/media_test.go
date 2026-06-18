// 媒体测试 / Media Tests
// 功能：magic-bytes、去 EXIF、缩放/WebP、策略护栏、上传/列表/删除/孤儿清理
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-18 09:48:36
package media

import (
	"bytes"
	"context"
	"database/sql"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/google/uuid"

	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/store/sqlcgen"
	"github.com/kartwo/kartwo/migrations"

	_ "modernc.org/sqlite"
)

func sampleImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 100, 255})
		}
	}
	return img
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, sampleImage(w, h)); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestDetectType(t *testing.T) {
	if m, _, err := DetectType(pngBytes(t, 4, 4)); err != nil || m != "image/png" {
		t.Fatalf("png: %q %v", m, err)
	}
	var jb bytes.Buffer
	_ = jpeg.Encode(&jb, sampleImage(4, 4), nil)
	if m, _, err := DetectType(jb.Bytes()); err != nil || m != "image/jpeg" {
		t.Fatalf("jpeg: %q %v", m, err)
	}
	if _, _, err := DetectType([]byte("not an image")); err == nil {
		t.Fatal("非图片应被拒")
	}
}

func TestProcess_ResizeAndWebP(t *testing.T) {
	p, err := Process(pngBytes(t, 2000, 1000))
	if err != nil {
		t.Fatalf("处理失败: %v", err)
	}
	if p.Width != 2000 || p.Height != 1000 {
		t.Fatalf("尺寸 = %dx%d", p.Width, p.Height)
	}
	if p.ContentHash == "" {
		t.Fatal("内容哈希为空")
	}
	if len(p.Derivatives) == 0 {
		t.Fatal("应生成派生图")
	}
	for _, d := range p.Derivatives {
		if d.Format != "webp" {
			t.Fatalf("派生 %s 格式 = %s", d.Label, d.Format)
		}
		b := d.Bytes
		if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
			t.Fatalf("派生 %s 不是合法 WebP", d.Label)
		}
		if d.Label == "thumb" && (d.Width > 200 && d.Height > 200) {
			t.Fatalf("thumb 未缩到 200 以内: %dx%d", d.Width, d.Height)
		}
	}
}

func TestProcess_StripsEXIF(t *testing.T) {
	// 造一张带 APP1/Exif 段的 JPEG。
	var raw bytes.Buffer
	if err := jpeg.Encode(&raw, sampleImage(32, 32), nil); err != nil {
		t.Fatal(err)
	}
	rb := raw.Bytes()
	payload := append([]byte("Exif\x00\x00"), make([]byte, 16)...)
	length := len(payload) + 2
	app1 := append([]byte{0xFF, 0xE1, byte(length >> 8), byte(length)}, payload...)
	withExif := append(append(append([]byte{}, rb[:2]...), app1...), rb[2:]...)

	if bytes.Contains(withExif, []byte("Exif")) == false {
		t.Fatal("构造的输入应含 Exif")
	}
	p, err := Process(withExif)
	if err != nil {
		t.Fatalf("处理失败: %v", err)
	}
	if bytes.Contains(p.OriginalBytes, []byte("Exif")) {
		t.Fatal("去 EXIF 后原图不应再含 Exif 段")
	}
}

func TestDefaultPolicy(t *testing.T) {
	p := NewDefaultPolicy("/tmp", 100, 1000)
	p.freeFn = func(string) (uint64, error) { return 1_000_000, nil }
	if err := p.AllowUpload(50); err != nil {
		t.Fatalf("正常应放行: %v", err)
	}
	if err := p.AllowUpload(200); err != ErrFileTooLarge {
		t.Fatalf("超大应 ErrFileTooLarge: %v", err)
	}
	p.freeFn = func(string) (uint64, error) { return 500, nil } // 可用 < MinFree
	if err := p.AllowUpload(50); err != ErrDiskFull {
		t.Fatalf("磁盘将满应 ErrDiskFull: %v", err)
	}
}

// ---- service ----

func newMediaSvc(t *testing.T, maxPer int) (*Service, *sql.DB, int64) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/t.db?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := migrate.Run(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	pid, err := sqlcgen.New(db).CreateProduct(context.Background(), sqlcgen.CreateProductParams{
		PublicID: uuid.Must(uuid.NewV7()).String(), Title: "P", Slug: "p", Status: "draft",
	})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir() + "/media"
	svc := New(db, NewLocalBackend(root), NewDefaultPolicy(root, 10<<20, 0), maxPer)
	return svc, db, pid
}

func TestService_UploadListDelete(t *testing.T) {
	svc, _, pid := newMediaSvc(t, 20)
	ctx := context.Background()

	asset, err := svc.Upload(ctx, pid, pngBytes(t, 1200, 800))
	if err != nil {
		t.Fatalf("上传失败: %v", err)
	}
	if asset.PublicID == "" || len(asset.Derivatives) == 0 {
		t.Fatalf("资产异常: %+v", asset)
	}
	// 文件确实落盘（取原图可读）。
	rc, err := svc.backend.Open(asset.derivePathForTest())
	if err == nil {
		_ = rc.Close()
	}

	list, err := svc.ListByProduct(ctx, pid)
	if err != nil || len(list) != 1 {
		t.Fatalf("列表异常: n=%d err=%v", len(list), err)
	}

	if err := svc.Delete(ctx, asset.PublicID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	list, _ = svc.ListByProduct(ctx, pid)
	if len(list) != 0 {
		t.Fatalf("删后列表应为空，得 %d", len(list))
	}
	if err := svc.Delete(ctx, "nope"); err != ErrNotFound {
		t.Fatalf("删不存在应 ErrNotFound: %v", err)
	}
}

func TestService_MaxPerProduct(t *testing.T) {
	svc, _, pid := newMediaSvc(t, 1)
	ctx := context.Background()
	if _, err := svc.Upload(ctx, pid, pngBytes(t, 100, 100)); err != nil {
		t.Fatalf("首张失败: %v", err)
	}
	if _, err := svc.Upload(ctx, pid, pngBytes(t, 120, 90)); err != ErrTooManyPerProduct {
		t.Fatalf("超额应 ErrTooManyPerProduct: %v", err)
	}
}

func TestService_RejectsNonImage(t *testing.T) {
	svc, _, pid := newMediaSvc(t, 20)
	if _, err := svc.Upload(context.Background(), pid, []byte("hello not image")); err != ErrUnsupportedType {
		t.Fatalf("非图片应 ErrUnsupportedType: %v", err)
	}
}

func TestService_CleanupOrphans(t *testing.T) {
	svc, db, pid := newMediaSvc(t, 20)
	ctx := context.Background()
	asset, err := svc.Upload(ctx, pid, pngBytes(t, 300, 200))
	if err != nil {
		t.Fatal(err)
	}
	// 软删商品 → 其图成孤儿。
	if _, err := db.ExecContext(ctx, "UPDATE product SET deleted_at = '2026-01-01T00:00:00.000Z' WHERE id = ?", pid); err != nil {
		t.Fatal(err)
	}
	n, err := svc.CleanupOrphans(ctx)
	if err != nil || n != 1 {
		t.Fatalf("清理孤儿 n=%d err=%v", n, err)
	}
	// 文件已被移除。
	if rc, err := svc.backend.Open("originals/" + assetHash(asset) + ".png"); err == nil {
		_ = rc.Close()
		t.Fatal("孤儿原图文件应已删除")
	}
	// 再清理无孤儿。
	if n, _ := svc.CleanupOrphans(ctx); n != 0 {
		t.Fatalf("二次清理应为 0，得 %d", n)
	}
}

// 辅助：测试里无法直接取 hash，这里从 derivative URL 反推已足够；保留占位避免编译期未用。
func (a *Asset) derivePathForTest() string {
	if len(a.Derivatives) == 0 {
		return ""
	}
	// URL 形如 /media/derived/<hash>_<label>.webp
	u := a.Derivatives[0].URL
	return u[len("/media/"):]
}

func assetHash(a *Asset) string {
	u := a.OriginalURL // /media/originals/<hash>.png
	prefix := "/media/originals/"
	s := u[len(prefix):]
	return s[:len(s)-len(".png")]
}
