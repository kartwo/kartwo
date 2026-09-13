// 品牌 Logo 存储测试 / Brand Logo Storage Tests
// 功能：验证 Logo 处理、WebP 落盘及品牌目录删除边界
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-12 23:00:00
package media

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreBrandLogoAndDeleteBoundary(t *testing.T) {
	root := t.TempDir()
	svc := New(nil, NewLocalBackend(root), NewDefaultPolicy(root, 10<<20, 0), 20)
	var input bytes.Buffer
	if err := png.Encode(&input, image.NewRGBA(image.Rect(0, 0, 1200, 400))); err != nil {
		t.Fatal(err)
	}
	logo, err := svc.StoreBrandLogo(input.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if logo.Path == "" || logo.URL != "/media/"+logo.Path || logo.Width != 800 || logo.Height != 266 {
		t.Fatalf("Logo 结果异常: %+v", logo)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(logo.Path))); err != nil {
		t.Fatalf("Logo 未落盘: %v", err)
	}
	if err := svc.DeleteBrandLogo("originals/do-not-delete.png"); err == nil {
		t.Fatal("不得删除 brand 目录外的文件")
	}
	if err := svc.DeleteBrandLogo(logo.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(logo.Path))); !os.IsNotExist(err) {
		t.Fatalf("Logo 应已删除，得到: %v", err)
	}
}
