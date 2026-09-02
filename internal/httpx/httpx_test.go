// HTTP 请求安全判定测试 / HTTP Request Security Decision Tests
// 功能：验证直连 TLS 和可信反向代理的 HTTPS 判定边界
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-26 18:21:07
package httpx

import (
	"crypto/tls"
	"net"
	"net/http/httptest"
	"testing"
)

func mustPrefix(t *testing.T, raw string) *net.IPNet {
	t.Helper()
	_, p, err := net.ParseCIDR(raw)
	if err != nil {
		t.Fatalf("parse cidr failed: %v", err)
	}
	return p
}

func TestIsSecureRequestTLSWins(t *testing.T) {
	req := httptest.NewRequest("GET", "https://example.com", nil)
	req.TLS = &tls.ConnectionState{}
	if !IsSecureRequest(req, nil) {
		t.Fatal("TLS 请求应判定为安全")
	}
}

func TestIsSecureRequestRejectsUntrustedProxy(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.RemoteAddr = "198.51.100.10:443"
	if IsSecureRequest(req, nil) {
		t.Fatal("未配置白名单时不应信任 X-Forwarded-Proto")
	}
}

func TestIsSecureRequestAcceptsTrustedProxyHttps(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.RemoteAddr = "198.51.100.10:8080"
	trusted := []*net.IPNet{mustPrefix(t, "198.51.100.0/24")}
	if !IsSecureRequest(req, trusted) {
		t.Fatal("白名单命中且 X-Forwarded-Proto=https 时应视为安全")
	}
}

func TestIsSecureRequestRejectsWrongProtoFromTrustedProxy(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com", nil)
	req.Header.Set("X-Forwarded-Proto", "http")
	req.RemoteAddr = "203.0.113.15:8080"
	trusted := []*net.IPNet{mustPrefix(t, "203.0.113.0/24")}
	if IsSecureRequest(req, trusted) {
		t.Fatal("trusted 代理但 X-Forwarded-Proto 非 https 应判定为非安全")
	}
}
