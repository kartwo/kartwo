// WebDAV 上传 / WebDAV Uploader
// 功能：将本地备份文件上传到 HTTPS WebDAV 目录，支持基于文件名的 PUT
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-25 10:20:00
package backup

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

const maxUploadResponsePreview = 512

// WebDAVUploader 负责把本地文件上传到远端目录。
// 说明：密码读取只在启动时传入；若密码为空即不带认证。
type WebDAVUploader struct {
	httpClient *http.Client
	remoteURL  *url.URL
	username   string
	password   string
}

// Uploader 是备份任务可选的远端推送能力。
type Uploader interface {
	Upload(ctx context.Context, sourcePath string) error
	Test(ctx context.Context, sourcePath string) error
}

// NewWebDAVUploader 仅支持 https URL。
func NewWebDAVUploader(rawURL, username, password, remotePath string) (*WebDAVUploader, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("解析 WebDAV URL 失败: %w", err)
	}
	if parsed.Scheme != "https" {
		return nil, fmt.Errorf("WebDAV URL 必须是 https")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("WebDAV URL 必须包含主机")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("WebDAV URL 不能包含用户名或密码")
	}
	if remotePath == "" {
		remotePath = "/"
	}
	if !strings.HasPrefix(remotePath, "/") {
		return nil, fmt.Errorf("WebDAV 目录必须以 / 开头")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if remotePath != "/" {
		parsed.Path = path.Join(parsed.Path, remotePath)
	}
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return &WebDAVUploader{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		remoteURL:  parsed,
		username:   strings.TrimSpace(username),
		password:   password,
	}, nil
}

// Upload 把 sourcePath 按原文件名 PUT 到 remotePath 下。
func (u *WebDAVUploader) Upload(ctx context.Context, sourcePath string) error {
	r, err := os.Open(sourcePath) // #nosec G304 -- 路径仅来自 Exporter.CreatePersistent 或测试创建的临时文件。
	if err != nil {
		return fmt.Errorf("打开备份文件失败: %w", err)
	}
	defer func() { _ = r.Close() }()

	target, err := u.buildURL(sourcePath)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, r)
	if err != nil {
		return fmt.Errorf("创建 WebDAV 请求失败: %w", err)
	}
	if u.username != "" {
		req.SetBasicAuth(u.username, u.password)
	}
	resp, err := u.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("上传 WebDAV 失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxUploadResponsePreview))
		return fmt.Errorf("上传 WebDAV 失败（HTTP %d）%s", resp.StatusCode, string(body))
	}
	return nil
}

// Test 走一次 1KB 空文件的 PUT 上传，使用临时文件验证连通与凭据。
func (u *WebDAVUploader) Test(ctx context.Context, sourcePath string) error {
	if sourcePath == "" {
		f, err := os.CreateTemp("", "kartwo-webdav-test-*.txt")
		if err != nil {
			return fmt.Errorf("创建测试临时文件失败: %w", err)
		}
		sourcePath = f.Name()
		_ = f.Close()
		if err := os.WriteFile(sourcePath, []byte("kartwo-webdav-test"), 0o600); err != nil {
			return fmt.Errorf("测试文件写入失败: %w", err)
		}
		defer func() { _ = os.Remove(sourcePath) }()
	}
	return u.Upload(ctx, sourcePath)
}

func (u *WebDAVUploader) buildURL(sourcePath string) (string, error) {
	name := path.Base(strings.TrimSpace(sourcePath))
	if name == "." || name == "/" || strings.Contains(name, "\n") {
		return "", fmt.Errorf("非法源文件名")
	}
	remote := *u.remoteURL
	remote.Path = path.Join(remote.Path, name)
	return remote.String(), nil
}
