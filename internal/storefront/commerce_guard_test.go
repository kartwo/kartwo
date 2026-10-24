// 交易入口保护测试 / Commerce Guard Tests
// 功能：验证空购物车无写入、限流内存上界、代理信任及超大请求提前拒绝
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 13:30:20
package storefront

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEmptyCartReadsDoNotCreateRows(t *testing.T) {
	h, mux := newHTTP(t)
	for _, path := range []string{"/cart", "/cart/data", "/checkout"} {
		for _, token := range []string{"", "invalid-token"} {
			r := httptest.NewRequest(http.MethodGet, path, nil)
			if token != "" {
				r.AddCookie(&http.Cookie{Name: cartCookie, Value: token})
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != 200 || w.Header().Get("Set-Cookie") != "" {
				t.Fatalf("%s code=%d cookie=%s", path, w.Code, w.Header().Get("Set-Cookie"))
			}
		}
	}
	var count int
	if err := h.svc.db.QueryRow(`SELECT COUNT(*) FROM cart`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("empty reads created %d carts: %v", count, err)
	}
	var variant string
	if err := h.svc.db.QueryRow(`SELECT public_id FROM variant LIMIT 1`).Scan(&variant); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/cart/items", strings.NewReader(`{"variant":"`+variant+`","quantity":1}`))
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("add: %d %s", w.Code, w.Body.String())
	}
	response := w.Result()
	defer func() { _ = response.Body.Close() }()
	var cookie *http.Cookie
	for _, c := range response.Cookies() {
		if c.Name == cartCookie {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("add must set cart cookie")
	}
	view := httptest.NewRequest(http.MethodGet, "/cart/data", nil)
	view.AddCookie(cookie)
	shown := httptest.NewRecorder()
	mux.ServeHTTP(shown, view)
	if shown.Code != 200 || !strings.Contains(shown.Body.String(), `"count":1`) {
		t.Fatalf("existing cart not shown: %s", shown.Body.String())
	}
	if err := h.svc.db.QueryRow(`SELECT COUNT(*) FROM cart`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("carts=%d err=%v", count, err)
	}
}

func TestCommerceLimiterBoundsAndExpires(t *testing.T) {
	l := newCommerceLimiter(2)
	now := time.Unix(10000, 0)
	for i := 0; i < 2; i++ {
		if ok, _ := l.allowAt("same", now); !ok {
			t.Fatal("normal request rejected")
		}
	}
	if ok, retry := l.allowAt("same", now); ok || retry < 1 {
		t.Fatalf("over quota ok=%v retry=%d", ok, retry)
	}
	for i := 1; i < commerceMaxKeys; i++ {
		if ok, _ := l.allowAt(fmt.Sprint(i), now); !ok {
			t.Fatal("unexpected capacity rejection")
		}
	}
	if ok, _ := l.allowAt("overflow", now); ok {
		t.Fatal("capacity must be bounded")
	}
	if ok, _ := l.allowAt("same", now); ok {
		t.Fatal("new keys must not evict limited key")
	}
	if len(l.entries) != commerceMaxKeys {
		t.Fatalf("keys=%d", len(l.entries))
	}
	if ok, _ := l.allowAt("same", now.Add(time.Minute)); !ok {
		t.Fatal("expired window must reopen")
	}
	if len(l.entries) != 1 {
		t.Fatalf("expired entries retained: %d", len(l.entries))
	}
}

func TestCommerceGuardTrustAndBodyLimit(t *testing.T) {
	h, _ := newHTTP(t)
	for _, trusted := range []bool{false, true} {
		t.Run(fmt.Sprint(trusted), func(t *testing.T) {
			var networks []*net.IPNet
			if trusted {
				_, network, _ := net.ParseCIDR("127.0.0.0/8")
				networks = append(networks, network)
			}
			h.trusted = networks
			called := 0
			guard := h.protectCommerce(newCommerceLimiter(1), func(w http.ResponseWriter, r *http.Request) { called++; w.WriteHeader(204) })
			for i := 1; i <= 2; i++ {
				r := httptest.NewRequest(http.MethodPost, "/checkout", strings.NewReader("email=a@example.com"))
				r.Header.Set("Sec-Fetch-Site", "same-origin")
				r.RemoteAddr = "127.0.0.1:1234"
				r.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", i))
				w := httptest.NewRecorder()
				guard(w, r)
				want := 204
				if i == 2 && !trusted {
					want = 429
				}
				if w.Code != want {
					t.Fatalf("request %d code=%d want=%d", i, w.Code, want)
				}
				if want == 429 && w.Header().Get("Retry-After") == "" {
					t.Fatal("missing Retry-After")
				}
			}
			want := 1
			if trusted {
				want = 2
			}
			if called != want {
				t.Fatalf("handler calls=%d", called)
			}
		})
	}
	called := false
	guard := h.protectCommerce(newCommerceLimiter(1), func(http.ResponseWriter, *http.Request) { called = true })
	large := httptest.NewRequest(http.MethodPost, "/checkout", strings.NewReader(strings.Repeat("x", commerceMaxBody+1)))
	large.Header.Set("Sec-Fetch-Site", "same-origin")
	w := httptest.NewRecorder()
	guard(w, large)
	if w.Code != 413 || called {
		t.Fatalf("large body code=%d reached handler=%v", w.Code, called)
	}
}

func TestCheckoutRateLimitPreventsExtraWork(t *testing.T) {
	h, _ := newHTTP(t)
	h.checkoutLimit = newCommerceLimiter(1)
	// Register 已捕获原 limiter，因此重新注册以使用测试限额。
	mux := http.NewServeMux()
	h.Register(mux)
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest(http.MethodPost, "/checkout", strings.NewReader("email=a@example.com"))
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if i == 1 && w.Code != 429 {
			t.Fatalf("second submit code=%d", w.Code)
		}
	}
	var carts, orders int
	if err := h.svc.db.QueryRow(`SELECT COUNT(*) FROM cart`).Scan(&carts); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.db.QueryRow(`SELECT COUNT(*) FROM "order"`).Scan(&orders); err != nil {
		t.Fatal(err)
	}
	if carts != 0 || orders != 0 {
		t.Fatalf("empty spam created carts=%d orders=%d", carts, orders)
	}
}

func TestCommerceGuardRejectsOversizedChunkedForm(t *testing.T) {
	h, mux := newHTTP(t)
	r := httptest.NewRequest(http.MethodPost, "/checkout", strings.NewReader("email="+strings.Repeat("a", commerceMaxBody)))
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.ContentLength = -1
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("unknown length code=%d", w.Code)
	}
	var count int
	if err := h.svc.db.QueryRow(`SELECT COUNT(*) FROM cart`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("carts=%d err=%v", count, err)
	}
}
