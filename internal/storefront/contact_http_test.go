// 店面客服表单测试 / Storefront Contact Form Tests
// 功能：验证开关、提交、防刷、同源与代理可信边界
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-28 15:20:00
package storefront

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kartwo/kartwo/internal/contact"
	"github.com/kartwo/kartwo/internal/settings"
)

func TestContactFormSubmissionAndSafety(t *testing.T) {
	sf, _, db := setup(t)
	h := NewHTTP(sf, nil, nil, settings.New(db), nil, nil, "Shop", "https://shop.example", nil)
	h.ConfigureContact(contact.New(db, settings.New(db)))
	mux := http.NewServeMux()
	h.Register(mux)
	post := func(extra func(*http.Request)) *httptest.ResponseRecorder {
		form := url.Values{"name": {"Alice"}, "email": {"alice@example.com"}, "topic": {"order"}, "body": {"Where is my order?"}}
		r := httptest.NewRequest("POST", "http://shop.example/contact", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", "http://shop.example")
		r.RemoteAddr = "203.0.113.44:1234"
		if extra != nil {
			extra(r)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	if got := post(nil).Code; got != http.StatusForbidden {
		t.Fatalf("disabled=%d", got)
	}
	cfg := contact.DefaultConfig()
	cfg.Enabled = true
	if err := h.contact.SaveConfig(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if got := post(func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }).Code; got != http.StatusForbidden {
		t.Fatalf("cross-origin=%d", got)
	}
	if got := post(func(r *http.Request) { r.Header.Del("Origin") }).Code; got != http.StatusForbidden {
		t.Fatalf("missing origin and fetch-site=%d", got)
	}
	if got := post(func(r *http.Request) { r.Header.Set("X-Forwarded-For", "8.8.8.8"); r.Header.Set("CF-IPCountry", "US") }).Code; got != http.StatusSeeOther {
		t.Fatalf("valid=%d", got)
	}
	rows, err := h.contact.List(context.Background(), "", 10)
	if err != nil || len(rows) != 1 || rows[0].IP != "203.0.113.44" || rows[0].Country != "" {
		t.Fatalf("untrusted headers: %+v %v", rows, err)
	}
	for i := 0; i < 4; i++ {
		if got := post(nil).Code; got != http.StatusSeeOther {
			t.Fatalf("allowed %d=%d", i, got)
		}
	}
	if got := post(nil).Code; got != http.StatusTooManyRequests {
		t.Fatalf("rate limit=%d", got)
	}
	if got := post(func(r *http.Request) {
		r.RemoteAddr = "203.0.113.45:1234"
		r.Header.Set("Sec-Fetch-Site", "cross-site")
	}).Code; got != http.StatusForbidden {
		t.Fatalf("fetch-site=%d", got)
	}
}

func TestContactTrustedProxyCountry(t *testing.T) {
	_, network, _ := net.ParseCIDR("127.0.0.1/32")
	h := &HTTP{trusted: []*net.IPNet{network}}
	r := httptest.NewRequest("POST", "http://example.com/contact", nil)
	r.RemoteAddr = "127.0.0.1:3000"
	r.Header.Set("X-Forwarded-For", "203.0.113.5, 127.0.0.1")
	r.Header.Set("CF-IPCountry", "CN")
	ip, country := h.contactIPCountry(r)
	if ip != "203.0.113.5" || country != "CN" {
		t.Fatalf("trusted proxy: %s %s", ip, country)
	}
}
