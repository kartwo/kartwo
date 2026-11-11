// 后台响应隐私测试 / Admin Response Privacy Tests
// 功能：验证登录、鉴权拒绝、订单查询与导出均不可缓存，保留 CSRF 边界
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 17:07:13
package admin

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminPrivateResponsesIncludeAuthErrorsAndExports(t *testing.T) {
	_, mux := newHTTP(t)
	sess, _ := loginAndCookies(t, mux)
	for _, tc := range []struct {
		method, path, body string
		auth               bool
		status             int
	}{
		{"GET", "/admin/api/orders", "", false, 401},
		{"GET", "/admin/api/orders", "", true, 200},
		{"GET", "/admin/api/orders/nonexistent", "", true, 404},
		{"GET", "/admin/api/orders/export", "", true, 200},
		{"POST", "/admin/api/orders/nonexistent/refund", "{}", true, 403},
		{"GET", "/admin/api/status", "", false, 200},
		{"POST", "/admin/api/login", "{}", false, 401},
	} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		r.Header.Set("Content-Type", "application/json")
		if tc.auth {
			r.AddCookie(sess)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s %s code=%d body=%s", tc.method, tc.path, w.Code, w.Body.String())
		}
		for k, v := range map[string]string{"Cache-Control": "private, no-store", "Referrer-Policy": "no-referrer", "X-Robots-Tag": "noindex, nofollow, noarchive"} {
			if w.Header().Get(k) != v {
				t.Errorf("%s %s header %s=%q", tc.method, tc.path, k, w.Header().Get(k))
			}
		}
	}
}
