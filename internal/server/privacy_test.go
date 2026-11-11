// 服务响应隐私测试 / Server Response Privacy Tests
// 功能：验证路由前的隐私保护涵盖未知路径与错误，公开商品页保持可索引
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 17:07:13
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSensitiveRoutesProtectErrorsAndMethodMismatch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /order/{id}", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	handler := securityHeaders(true)(mux)
	for _, path := range []string{"/order/nonexistent", "/order/nonexistent/extra", "/paypal/unknown", "/checkout", "/cart", "/cart/data", "/admin/api/unknown"} {
		for _, method := range []string{"GET", "POST", "HEAD"} {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(method, path, nil))
			if w.Header().Get("Cache-Control") != "private, no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" || w.Header().Get("X-Robots-Tag") != "noindex, nofollow, noarchive" {
				t.Errorf("private headers missing: %s %s status=%d", method, path, w.Code)
			}
			if w.Header().Get("Strict-Transport-Security") == "" || w.Header().Get("Content-Security-Policy") == "" {
				t.Error("existing security headers lost")
			}
		}
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/p/shirt", nil))
	if w.Header().Get("X-Robots-Tag") != "" || w.Header().Get("Cache-Control") != "" {
		t.Fatal("public product route accidentally private")
	}
}
