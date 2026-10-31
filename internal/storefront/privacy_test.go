// 店面隐私与同源测试 / Storefront Privacy and Origin Tests
// 功能：验证私密响应错误/跳转不缓存、拒绝跨站写入、支付返回兼容及日志脱敏
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 17:07:13
package storefront

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func requirePrivate(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	for key, want := range map[string]string{"Cache-Control": "private, no-store", "Pragma": "no-cache", "Expires": "0", "X-Robots-Tag": "noindex, nofollow, noarchive", "Referrer-Policy": "no-referrer"} {
		if got := w.Header().Get(key); got != want {
			t.Errorf("%s=%q want=%q", key, got, want)
		}
	}
}

func TestCommercePrivateResponsesAndPublicCatalog(t *testing.T) {
	_, mux := newHTTP(t)
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/cart", 200}, {"GET", "/cart/data", 200}, {"GET", "/checkout", 200},
		{"GET", "/order/nonexistent", 200}, {"POST", "/order/nonexistent/pay", 303},
		{"GET", "/paypal/return", 303}, {"POST", "/checkout", 200},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Header.Set("Sec-Fetch-Site", "same-origin")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			requirePrivate(t, w)
		})
	}
	for _, path := range []string{"/", "/collections/all", "/blog"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Header().Get("X-Robots-Tag") != "" || w.Header().Get("Cache-Control") == "private, no-store" {
			t.Fatalf("public page became private: %s", path)
		}
	}
}

func TestCommerceRejectsCrossOriginBeforeMutation(t *testing.T) {
	h, mux := newHTTP(t)
	for _, path := range []string{"/cart/items", "/cart/items/variant", "/checkout", "/order/private-id/pay"} {
		for _, method := range []string{"POST", "PATCH", "DELETE"} {
			if path == "/cart/items" && method != "POST" || strings.HasSuffix(path, "variant") && method == "POST" || !strings.HasPrefix(path, "/cart/") && method != "POST" {
				continue
			}
			for _, headers := range []map[string]string{
				{}, {"Origin": "https://evil.example"}, {"Origin": "null"},
				{"Sec-Fetch-Site": "cross-site", "Origin": "http://example.com"},
				{"Sec-Fetch-Site": "same-site", "Origin": "http://example.com"},
				{"Sec-Fetch-Site": "same-origin", "Origin": "https://evil.example"},
			} {
				r := httptest.NewRequest(method, path, strings.NewReader(`{}`))
				for k, v := range headers {
					r.Header.Set(k, v)
				}
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, r)
				if w.Code != 403 {
					t.Fatalf("%s %s headers=%v code=%d", method, path, headers, w.Code)
				}
				requirePrivate(t, w)
			}
		}
	}
	for _, table := range []string{"cart", "\"order\""} {
		var count int
		// Both table names are fixed test literals, not request input.
		if err := h.svc.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("unexpected mutation %s count=%d err=%v", table, count, err)
		}
	}
}

func TestCommerceOriginUsesTrustedTLSAndExactOrigin(t *testing.T) {
	_, trusted, _ := net.ParseCIDR("127.0.0.0/8")
	for _, tc := range []struct {
		origin, site, proxy string
		trust, allowed      bool
	}{
		{"http://shop.example", "", "", false, true},
		{"", "same-origin", "", false, true},
		{"http://shop.example:81", "", "", false, false},
		{"https://shop.example", "", "https", false, false},
		{"https://shop.example", "", "https", true, true},
		{"http://shop.example", "", "https", true, false},
		{"http://user@shop.example", "", "", false, false},
		{"http://shop.example/path", "", "", false, false},
		{"http://shop.example?x=1", "", "", false, false},
		{"http://shop.example#fragment", "", "", false, false},
		{"http://shop.example.evil.test", "", "", false, false},
		{"http://shop.example", "none", "", false, false},
	} {
		h := &HTTP{}
		if tc.trust {
			h.trusted = []*net.IPNet{trusted}
		}
		r := httptest.NewRequest("POST", "http://shop.example/checkout", nil)
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Sec-Fetch-Site", tc.site)
		r.Header.Set("X-Forwarded-Proto", tc.proxy)
		if got := h.sameOrigin(r); got != tc.allowed {
			t.Errorf("%+v allowed=%v", tc, got)
		}
	}
}

type privateReturnGateway struct {
	fakeGateway
	calls int
	fail  bool
}

func (f *privateReturnGateway) CapturePayPal(context.Context, string) (string, error) {
	f.calls++
	if f.fail {
		return "private-order-reference", errors.New("raw-provider-error customer-private@example.com")
	}
	return "private-order-reference", nil
}

func TestPayPalCrossSiteReturnAndPrivateLogging(t *testing.T) {
	h, _ := newHTTP(t)
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	gateway := &privateReturnGateway{}
	h.pay = gateway
	mux := http.NewServeMux()
	h.Register(mux)
	probe := httptest.NewRecorder()
	mux.ServeHTTP(probe, httptest.NewRequest("HEAD", "/paypal/return?token=private-gateway-token", nil))
	if probe.Code != 405 || gateway.calls != 0 {
		t.Fatal("HEAD probe must not capture")
	}
	requirePrivate(t, probe)
	for _, fail := range []bool{false, true} {
		gateway.fail = fail
		r := httptest.NewRequest("GET", "/paypal/return?token=private-gateway-token&email=customer-private@example.com", nil)
		r.Header.Set("Sec-Fetch-Site", "cross-site")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != 303 {
			t.Fatalf("approved provider return blocked: %d", w.Code)
		}
		requirePrivate(t, w)
	}
	h.pay = nil
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/paypal/return?token=private-gateway-token&email=customer-private@example.com", nil))
	if gateway.calls != 2 {
		t.Fatalf("capture calls=%d", gateway.calls)
	}
	for _, secret := range []string{"private-gateway-token", "private-order-reference", "customer-private@example.com", "raw-provider-error"} {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("log leaked %s", secret)
		}
	}
	if !strings.Contains(logs.String(), "reason=1") {
		t.Fatal("missing safe failure category")
	}
}
