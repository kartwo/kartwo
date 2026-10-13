// 博客图片测试 / Blog Image Tests
// 功能：验证博客图片沿用类型校验、WebP 处理与独立相对路径
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-13 10:00:00
package media

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreBlogImageUsesProcessedLocalMedia(t *testing.T) {
	root := t.TempDir()
	svc := New(nil, NewLocalBackend(root), NewDefaultPolicy(root, 10<<20, 0), 20)
	if _, err := svc.StoreBlogImage([]byte("not an image")); err != ErrUnsupportedType {
		t.Fatalf("伪造图片应拒绝: %v", err)
	}
	var input bytes.Buffer
	if err := png.Encode(&input, image.NewRGBA(image.Rect(0, 0, 300, 200))); err != nil {
		t.Fatal(err)
	}
	img, err := svc.StoreBlogImage(input.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(img.Path, "blog/") || !strings.HasSuffix(img.Path, ".webp") || img.URL != "/media/"+img.Path {
		t.Fatalf("路径不正确: %+v", img)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(img.Path))); err != nil {
		t.Fatalf("文件未落盘: %v", err)
	}
}
