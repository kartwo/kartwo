// 后台退款请求测试 / Admin Refund Request Tests
// 功能：验证新退款接口的登录、CSRF、演示隔离与成功审计边界
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 23:10:00
package admin

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/kartwo/kartwo/internal/payment"
)

type fakeRefundRequests struct{ calls, refreshes int }

func (*fakeRefundRequests) Refund(context.Context, string) error { return nil }
func (f *fakeRefundRequests) RequestRefund(_ context.Context, _ string, in payment.RefundInput) (payment.RefundResult, error) {
	f.calls++
	return payment.RefundResult{ID: "refund-request", State: "pending", AmountCents: in.AmountCents, Created: f.calls == 1}, nil
}
func (f *fakeRefundRequests) RetryRefund(context.Context, string, string, string) (payment.RefundResult, error) {
	f.refreshes++
	return payment.RefundResult{ID: "refund-request", State: "succeeded", Completed: f.refreshes == 1}, nil
}
func TestHTTPRefundRequestsCSRFAndAudit(t *testing.T) {
	h, mux := newHTTP(t)
	h.ConfigureDemo(DemoConfig{Enabled: true, SessionTTL: 45 * time.Minute, MaxProducts: 3, MaxImageBytes: 2 << 20, CleanupInterval: 5 * time.Minute})
	fake := &fakeRefundRequests{}
	h.pay = fake
	sess, csrf := loginAndCookies(t, mux)
	path := "/admin/api/orders/order-public/refunds"
	body := `{"request_key":"browser-refund-001","provider":"stripe","payment_ref":"pi-public","amount_cents":500,"expected_refunded_cents":0}`
	if r := doJSON(t, mux, "POST", path, body, nil, ""); r.StatusCode != 401 {
		t.Fatalf("anonymous refund: %d", r.StatusCode)
	}
	if r := doJSON(t, mux, "POST", path, body, []*http.Cookie{sess}, ""); r.StatusCode != 403 {
		t.Fatalf("missing CSRF: %d", r.StatusCode)
	}
	if fake.calls != 0 {
		t.Fatal("gateway reached without authorization")
	}
	for range 2 {
		if r := doJSON(t, mux, "POST", path, body, []*http.Cookie{sess}, csrf); r.StatusCode != 202 {
			t.Fatalf("pending: %d %s", r.StatusCode, r.Body)
		}
	}
	audits := doJSON(t, mux, "GET", "/admin/api/audit-events", "", []*http.Cookie{sess}, "")
	if bytes.Count(audits.Body, []byte(`"action":"order.refund_requested"`)) != 1 || bytes.Contains(audits.Body, []byte(`"action":"order.refund"`)) {
		t.Fatalf("pending refund audited as completed: %s", audits.Body)
	}
	for range 2 {
		if r := doJSON(t, mux, "POST", path+"/refund-request/refresh", `{}`, []*http.Cookie{sess}, csrf); r.StatusCode != 200 {
			t.Fatalf("refresh: %d", r.StatusCode)
		}
	}
	audits = doJSON(t, mux, "GET", "/admin/api/audit-events", "", []*http.Cookie{sess}, "")
	if bytes.Count(audits.Body, []byte(`"action":"order.refund"`)) != 1 {
		t.Fatalf("completion audit duplicated: %s", audits.Body)
	}
	demo := doJSON(t, mux, "POST", "/admin/api/demo-session", "", nil, "")
	var demoSession *http.Cookie
	var demoCSRF string
	for _, c := range demo.Cookies {
		if c.Name == sessionCookie {
			demoSession = c
		}
		if c.Name == csrfCookie {
			demoCSRF = c.Value
		}
	}
	if demoSession == nil {
		t.Fatal("no demo session")
	}
	if r := doJSON(t, mux, "POST", path, body, []*http.Cookie{demoSession}, demoCSRF); r.StatusCode != 403 {
		t.Fatalf("demo refund: %d", r.StatusCode)
	}
	if fake.calls != 2 {
		t.Fatal("demo reached refund service")
	}
}
