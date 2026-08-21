// 远程图片抓取 / Remote Image Fetcher
// 功能：Shopify 导入图片的 URL 校验、SSRF 防护、限时限量下载与媒体预处理
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-21 15:20:00
package importer

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/kartwo/kartwo/internal/media"
)

const maxRemoteImageBytes = 10 << 20

type imageReference struct {
	Slug string
	Row  int
	URL  string
}

type preparedImage struct {
	Slug      string
	Row       int
	Processed *media.Processed
}

func validateRemoteImageURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return fmt.Errorf("图片地址必须是公开的 HTTPS URL")
	}
	if port := u.Port(); port != "" && port != "443" {
		return fmt.Errorf("图片地址只允许 HTTPS 默认端口")
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !isPublicIP(ip) {
		return fmt.Errorf("图片地址不能指向内网或本机")
	}
	return nil
}

func prepareRemoteImages(ctx context.Context, refs []imageReference) ([]preparedImage, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	out := make([]preparedImage, 0, len(refs))
	for _, ref := range refs {
		b, err := fetchRemoteImage(ctx, ref.URL)
		if err != nil {
			return nil, &ValidationError{Message: fmt.Sprintf("第 %d 行图片下载失败：%v", ref.Row, err)}
		}
		p, err := media.Process(b)
		if err != nil {
			return nil, &ValidationError{Message: fmt.Sprintf("第 %d 行图片无效：%v", ref.Row, err)}
		}
		out = append(out, preparedImage{Slug: ref.Slug, Row: ref.Row, Processed: p})
	}
	return out, nil
}

func fetchRemoteImage(ctx context.Context, raw string) ([]byte, error) {
	if err := validateRemoteImageURL(raw); err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("解析图片域名: %w", err)
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("图片域名无可用地址")
			}
			for _, ip := range ips {
				if !isPublicIP(ip) {
					return nil, fmt.Errorf("图片地址不能指向内网或本机")
				}
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return fmt.Errorf("图片地址不允许重定向")
		},
	}
	req, err := newRemoteImageRequest(ctx, raw)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req) //nolint:gosec // URL、DNS 与连接地址均在上方受限。
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("图片服务返回 HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxRemoteImageBytes {
		return nil, fmt.Errorf("图片超过 10MB")
	}
	return b, nil
}

func newRemoteImageRequest(ctx context.Context, raw string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	// 部分公开图片 CDN 会拒绝 Go 默认的空白请求标识；明确表明导入用途，不伪装浏览器。
	req.Header.Set("User-Agent", "Kartwo/0.5 (+https://kartwo.com; self-hosted image importer)")
	req.Header.Set("Accept", "image/avif,image/webp,image/png,image/jpeg,image/*;q=0.8")
	return req, nil
}

func isPublicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsValid() && !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsMulticast() && !ip.IsUnspecified() && !strings.HasPrefix(ip.String(), "100.64.")
}
