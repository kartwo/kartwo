// Stripe连接接口回归 / Stripe Connection HTTP Tests
// 功能：验证连接测试鉴权/CSRF、固定错误、成功审计及不改收款配置
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-26 20:57:23
package admin

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kartwo/kartwo/internal/payment"
)

type stripeConnectionReply struct {
	err   error
	calls int
}

func (s *stripeConnectionReply) TestStripeConnection(context.Context) error {
	s.calls++
	return s.err
}

func TestStripeConnectionHTTPPrivacyAndAudit(t *testing.T) {
	h, mux := newHTTP(t)
	h.ConfigureDemo(DemoConfig{Enabled: true, SessionTTL: 45 * time.Minute, MaxProducts: 3, MaxImageBytes: 2 << 20, CleanupInterval: 5 * time.Minute})
	session, csrf := loginAndCookies(t, mux)
	owner := []*http.Cookie{session}
	const path = "/admin/api/settings/payment/stripe/test"
	marker := rand.Text()
	request := func(cookies []*http.Cookie, token string, want int) {
		t.Helper()
		r := doJSON(t, mux, http.MethodPost, path, "", cookies, token)
		if r.StatusCode != want {
			t.Fatalf("connection HTTP status %d, want %d", r.StatusCode, want)
		}
		if strings.Contains(string(r.Body), marker) {
			t.Fatal("public connection error exposed provider details")
		}
	}
	settingCount := func() int {
		t.Helper()
		var n int
		if err := h.svc.db.QueryRow(`SELECT count(*) FROM setting`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	auditCount := func() int {
		t.Helper()
		var n int
		if err := h.svc.db.QueryRow(`SELECT count(*) FROM audit_event WHERE action='payment.stripe_connection_test'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := settingCount()
	request(owner, csrf, http.StatusNotImplemented)
	stub := &stripeConnectionReply{err: errors.New(marker)}
	h.stripeTest = stub
	request(owner, csrf, http.StatusBadGateway)
	stub.err = payment.ErrLocked
	request(owner, csrf, http.StatusConflict)
	if auditCount() != 0 {
		t.Fatal("failed connection was audited as successful")
	}
	stub.err = nil
	request(owner, csrf, http.StatusOK)
	if stub.calls != 3 || auditCount() != 1 {
		t.Fatal("successful connection must invoke the provider and create one audit")
	}
	request(owner, "", http.StatusForbidden)
	request(nil, "", http.StatusUnauthorized)
	demo := doJSON(t, mux, http.MethodPost, "/admin/api/demo-session", "", nil, "")
	if demo.StatusCode != http.StatusCreated {
		t.Fatal("demo setup failed")
	}
	var demoCSRF string
	for _, cookie := range demo.Cookies {
		if cookie.Name == csrfCookie {
			demoCSRF = cookie.Value
		}
	}
	if demoCSRF == "" {
		t.Fatal("demo CSRF missing")
	}
	request(demo.Cookies, demoCSRF, http.StatusForbidden)
	if stub.calls != 3 || auditCount() != 1 || settingCount() != before {
		t.Fatal("rejected or read-only test changed payment configuration, audit or provider calls")
	}
}
