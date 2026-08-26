// WebDAV 上传测试 / WebDAV Uploader Tests
// 功能：验证接入点路径与配置目录会共同构成远端上传位置
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-25 17:10:00
package backup

import (
	"path"
	"strings"
	"testing"
)

func TestWebDAVUploaderPreservesEndpointPath(t *testing.T) {
	uploader, err := NewWebDAVUploader(
		"https://dav.example.com/remote.php/dav/files/kartwo",
		"merchant",
		"secret",
		"/backups/2026",
	)
	if err != nil {
		t.Fatalf("构造上传器失败: %v", err)
	}
	target, err := uploader.buildURL("/tmp/kartwo-backup.zip")
	if err != nil {
		t.Fatalf("构造上传地址失败: %v", err)
	}
	const want = "https://dav.example.com/remote.php/dav/files/kartwo/backups/2026/kartwo-backup.zip"
	if target != want {
		t.Fatalf("上传地址 = %q，期望 %q", target, want)
	}
}

func TestWebDAVUploaderRejectsCredentialsInURL(t *testing.T) {
	if _, err := NewWebDAVUploader("https://merchant:secret@dav.example.com", "", "", "/"); err == nil {
		t.Fatal("URL 内嵌凭证应被拒绝")
	}
}

func TestWebDAVUploaderRejectsHTTPOrMissingHost(t *testing.T) {
	if _, err := NewWebDAVUploader("http://dav.example.com", "", "", "/"); err == nil {
		t.Fatal("非 HTTPS URL 应被拒绝")
	}
	if _, err := NewWebDAVUploader("https://", "", "", "/"); err == nil {
		t.Fatal("缺主机的 URL 应被拒绝")
	}
}

func TestWebDAVUploaderBuildURLRejectsIllegalSourcePath(t *testing.T) {
	uploader, err := NewWebDAVUploader(
		"https://dav.example.com/backups",
		"merchant",
		"secret",
		"/",
	)
	if err != nil {
		t.Fatalf("构造上传器失败: %v", err)
	}
	if _, err := uploader.buildURL(""); err == nil {
		t.Fatal("空源路径应被拒绝")
	}
	if _, err := uploader.buildURL("  "); err == nil {
		t.Fatal("空白源路径应被拒绝")
	}
	if _, err := uploader.buildURL("."); err == nil {
		t.Fatal("非法源文件名应被拒绝")
	}
}

func TestWebDAVUploaderBuildURLRejectsNewlineInFileName(t *testing.T) {
	uploader, err := NewWebDAVUploader("https://dav.example.com/backups", "", "", "/")
	if err != nil {
		t.Fatalf("构造上传器失败: %v", err)
	}
	if _, err := uploader.buildURL("kartwo\nbackup.zip"); err == nil {
		t.Fatal("包含换行符的源文件名应被拒绝")
	}
}

func TestWebDAVUploaderTrimsAndNormalizesPaths(t *testing.T) {
	uploader, err := NewWebDAVUploader(
		"https://dav.example.com/dav/endpoint/",
		"",
		"",
		"/backups/2026/",
	)
	if err != nil {
		t.Fatalf("构造上传器失败: %v", err)
	}

	u := path.Base("/tmp/kartwo-backup.zip")
	target, err := uploader.buildURL("/tmp/kartwo-backup.zip")
	if err != nil {
		t.Fatalf("构造上传地址失败: %v", err)
	}
	expect := strings.TrimRight("/dav/endpoint", "/") + "/backups/2026/" + u
	if target != "https://dav.example.com"+path.Join("/", expect) {
		t.Fatalf("上传地址 = %q，期望包含归一化路径", target)
	}
}
