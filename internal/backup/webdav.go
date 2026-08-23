// WebDAV 异地备份 / WebDAV Remote Backup
// 功能：以 HTTPS WebDAV PUT 与 MOVE 上传本地 ZIP，避免部分文件作为正式备份暴露
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-23 12:00:00
package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

// Uploader 是异地备份传输接口；默认 nil，即只保留本地备份。
type Uploader interface {
	Upload(context.Context, string) error
}

// WebDAVUploader 将 ZIP 上传到商家配置的 WebDAV 目录。
type WebDAVUploader struct {
	base     *url.URL
	username string
	password string
	client   *http.Client
}

// NewWebDAVUploader 构造只允许 HTTPS 且不含 URL 内嵌凭证的上传器。
func NewWebDAVUploader(rawURL, username, password string) (*WebDAVUploader, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return nil, errors.New("backup: WebDAV 地址必须是不含凭证的 HTTPS URL")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/"
	return &WebDAVUploader{
		base: u, username: username, password: password,
		client: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

// Upload 先 PUT 到临时名称，成功后以 WebDAV MOVE 原子发布最终名称。
func (u *WebDAVUploader) Upload(ctx context.Context, localPath string) error {
	name := path.Base(localPath)
	if !strings.HasPrefix(name, "kartwo-backup-") || !strings.HasSuffix(name, ".zip") {
		return fmt.Errorf("backup: 非法自动备份文件名")
	}
	temporary := u.objectURL(name + ".uploading")
	final := u.objectURL(name)
	if err := u.put(ctx, temporary, localPath); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, "MOVE", temporary.String(), nil)
	if err != nil {
		return fmt.Errorf("backup: 创建 WebDAV 发布请求失败: %w", err)
	}
	request.SetBasicAuth(u.username, u.password)
	request.Header.Set("Destination", final.String())
	request.Header.Set("Overwrite", "T")
	response, err := u.client.Do(request)
	if err != nil {
		return fmt.Errorf("backup: 发布异地备份失败: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("backup: WebDAV 发布失败: HTTP %d", response.StatusCode)
	}
	return nil
}

func (u *WebDAVUploader) objectURL(name string) *url.URL {
	v := *u.base
	v.Path = path.Join(u.base.Path, name)
	return &v
}

func (u *WebDAVUploader) put(ctx context.Context, target *url.URL, localPath string) error {
	file, err := os.Open(localPath) //nolint:gosec // localPath 仅由受控自动备份目录提供。
	if err != nil {
		return fmt.Errorf("backup: 打开本地自动备份失败: %w", err)
	}
	defer func() { _ = file.Close() }()
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, target.String(), file)
	if err != nil {
		return fmt.Errorf("backup: 创建 WebDAV 上传请求失败: %w", err)
	}
	request.Header.Set("Content-Type", "application/zip")
	request.SetBasicAuth(u.username, u.password)
	response, err := u.client.Do(request)
	if err != nil {
		return fmt.Errorf("backup: 上传异地备份失败: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, response.Body)
		return fmt.Errorf("backup: WebDAV 上传失败: HTTP %d", response.StatusCode)
	}
	return nil
}
