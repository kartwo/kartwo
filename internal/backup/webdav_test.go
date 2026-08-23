// WebDAV 异地备份测试 / WebDAV Remote Backup Tests
// 功能：验证 HTTPS WebDAV 的临时上传、原子发布、认证及地址安全边界
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-23 12:10:00
package backup

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestWebDAVUploaderUploadsThenMoves(t *testing.T) {
	var methods, paths []string
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "merchant" || password != "secret" {
			t.Fatal("WebDAV 请求未携带正确认证")
		}
		methods = append(methods, r.Method)
		paths = append(paths, r.URL.Path)
		switch r.Method {
		case http.MethodPut:
			if got, _ := io.ReadAll(r.Body); string(got) != "zip-fixture" {
				t.Fatalf("上传内容错误: %q", got)
			}
			w.WriteHeader(http.StatusCreated)
		case "MOVE":
			if got := r.Header.Get("Destination"); got != server.URL+"/remote/kartwo-backup-20260823T120000Z.zip" {
				t.Fatalf("最终目标错误: %s", got)
			}
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("意外方法: %s", r.Method)
		}
	}))
	defer server.Close()

	uploader, err := NewWebDAVUploader(server.URL+"/remote", "merchant", "secret")
	if err != nil {
		t.Fatalf("构造上传器失败: %v", err)
	}
	uploader.client = server.Client()
	local := filepath.Join(t.TempDir(), "kartwo-backup-20260823T120000Z.zip")
	if err := os.WriteFile(local, []byte("zip-fixture"), 0o600); err != nil {
		t.Fatalf("写入 ZIP 夹具失败: %v", err)
	}
	if err := uploader.Upload(context.Background(), local); err != nil {
		t.Fatalf("上传失败: %v", err)
	}
	if len(methods) != 2 || methods[0] != http.MethodPut || methods[1] != "MOVE" {
		t.Fatalf("请求序列错误: %v", methods)
	}
	if paths[0] != "/remote/kartwo-backup-20260823T120000Z.zip.uploading" || paths[1] != "/remote/kartwo-backup-20260823T120000Z.zip.uploading" {
		t.Fatalf("临时对象路径错误: %v", paths)
	}
}

func TestWebDAVUploaderRejectsUnsafeURLAndRedirect(t *testing.T) {
	for _, raw := range []string{"http://example.com/backup", "https://user:pass@example.com/backup", "not-a-url"} {
		if _, err := NewWebDAVUploader(raw, "", ""); err == nil {
			t.Fatalf("不安全地址应被拒绝: %q", raw)
		}
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://other.example/", http.StatusFound)
	}))
	defer server.Close()
	uploader, err := NewWebDAVUploader(server.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}
	uploader.client = server.Client()
	uploader.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	local := filepath.Join(t.TempDir(), "kartwo-backup-20260823T120000Z.zip")
	if err := os.WriteFile(local, []byte("zip-fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := uploader.Upload(context.Background(), local); err == nil {
		t.Fatal("重定向响应应被拒绝")
	}
}
