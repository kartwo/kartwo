// 远程图片抓取测试 / Remote Image Fetcher Tests
// 功能：验证 Shopify 图片 URL 的协议、端口与内网地址 SSRF 防护
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-21 15:20:00
package importer

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

func TestValidateRemoteImageURL(t *testing.T) {
	for _, raw := range []string{
		"https://cdn.example.com/product.png",
		"https://8.8.8.8/photo.jpg",
	} {
		if err := validateRemoteImageURL(raw); err != nil {
			t.Fatalf("公开 HTTPS 地址 %q 被拒绝: %v", raw, err)
		}
	}
	for _, raw := range []string{
		"http://cdn.example.com/product.png",
		"https://127.0.0.1/secret.png",
		"https://10.0.0.1/secret.png",
		"https://cdn.example.com:8443/product.png",
		"https://user:pass@cdn.example.com/product.png",
	} {
		if err := validateRemoteImageURL(raw); err == nil {
			t.Fatalf("不安全地址 %q 未被拒绝", raw)
		}
	}
}

func TestPrepareRemoteImagesReturnsValidationError(t *testing.T) {
	_, err := prepareRemoteImages(context.Background(), []imageReference{{Row: 2, URL: "https://127.0.0.1/secret.png"}})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Message == "" {
		t.Fatalf("下载失败应给出可展示校验错误: %v", err)
	}
}

func TestRemoteImageRequestHeaders(t *testing.T) {
	req, err := newRemoteImageRequest(context.Background(), "https://cdn.example.com/image.png")
	if err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("User-Agent") == "" {
		t.Fatal("远程图片请求必须带可识别的 User-Agent")
	}
}

func TestIsPublicIP(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "169.254.1.1", "::1", "fc00::1", "100.64.0.1"} {
		ip := netip.MustParseAddr(raw)
		if isPublicIP(ip) {
			t.Fatalf("内网地址 %s 被当作公网", raw)
		}
	}
	if !isPublicIP(netip.MustParseAddr("1.1.1.1")) {
		t.Fatal("公网地址被拒绝")
	}
}
