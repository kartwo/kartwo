// 游客订单页面测试 / Guest Order Page Tests
// 功能：验证匿名/跨订单隔离、下单 Cookie、跨设备邮箱恢复、退出和无 SMTP 降级
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 17:18:40
package storefront

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/kartwo/kartwo/internal/order"
)

func responseCookie(t *testing.T, w *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	res := w.Result()
	defer func() { _ = res.Body.Close() }()
	for _, c := range res.Cookies() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("missing cookie %s", name)
	return nil
}
func guestCodeFromDB(t *testing.T, h *HTTP) string {
	t.Helper()
	var body string
	if err := h.svc.db.QueryRow(`SELECT body FROM email_outbox WHERE kind GLOB 'guest_access:*' ORDER BY id DESC LIMIT 1`).Scan(&body); err != nil {
		t.Fatal(err)
	}
	found := regexp.MustCompile(`code is: ([0-9]{8})`).FindStringSubmatch(body)
	if len(found) != 2 {
		t.Fatal("missing email code")
	}
	return found[1]
}
func authorizeGuestTest(t *testing.T, h *HTTP, id string) *http.Cookie {
	t.Helper()
	ctx := context.Background()
	var email string
	if err := h.svc.db.QueryRow(`SELECT email FROM "order" WHERE public_id=?`, id).Scan(&email); err != nil {
		t.Fatal(err)
	}
	browser, err := order.NewGuestToken()
	if err != nil {
		t.Fatal(err)
	}
	if err = h.order.RequestGuestAccess(ctx, id, email, browser); err != nil {
		t.Fatal(err)
	}
	token, err := h.order.VerifyGuestAccess(ctx, id, browser, guestCodeFromDB(t, h))
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: guestAccessCookie, Value: token, Path: guestOrderPath(id)}
}
func guestRequest(t *testing.T, mux http.Handler, method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	requirePrivate(t, w)
	return w
}
func seedPrivateOrder(t *testing.T, h *HTTP) {
	t.Helper()
	for _, query := range []string{
		`INSERT INTO customer(id,public_id,email,name) VALUES(901,'private-customer','private-buyer@example.com','Private Customer')`,
		`INSERT INTO "order"(public_id,customer_id,status,email,ship_name,ship_phone,ship_address,currency,subtotal_cents,total_cents) VALUES('private-order',901,'pending','private-buyer@example.com','Private Customer','+15555550190','Secret street 92','USD',12345,12345)`,
		`INSERT INTO "order"(public_id,customer_id,status,email,ship_name,ship_address,currency,subtotal_cents,total_cents) VALUES('second-private-order',901,'paid','private-buyer@example.com','Private Customer','Other street 8','USD',12345,12345)`,
	} {
		if _, err := h.svc.db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
}
func assertNoPrivateOrder(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	for _, secret := range []string{"private-buyer@example.com", "Private Customer", "Secret street 92", "+15555550190", "123.45", "Other street 8", "Pay now"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatalf("private data leaked: %s", secret)
		}
	}
	if !strings.Contains(w.Body.String(), "View your order") {
		t.Fatal("missing verification screen")
	}
}
func TestGuestOrderRecoveryAndObjectIsolation(t *testing.T) {
	h, mux := newHTTP(t)
	seedPrivateOrder(t, h)
	h.guestMailReady = func() bool { return true }
	for _, id := range []string{"private-order", "missing-order"} {
		w := guestRequest(t, mux, "GET", guestOrderPath(id), "")
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		assertNoPrivateOrder(t, w)
	}
	denied := guestRequest(t, mux, "POST", "/order/private-order/pay", "payment_method=stripe")
	if denied.Code != 303 || denied.Header().Get("Location") != "/order/private-order" {
		t.Fatal("anonymous payment must return to verification")
	}
	// Lookup replies have no address/existence signal and create no mail for a mismatch.
	wrong := guestRequest(t, mux, "POST", "/order/private-order/access/request", "email=wrong%40example.com")
	var count int
	if err := h.svc.db.QueryRow(`SELECT COUNT(*) FROM email_outbox`).Scan(&count); err != nil || count != 0 {
		t.Fatal("wrong email queued mail")
	}
	sent := guestRequest(t, mux, "POST", "/order/private-order/access/request", "email=private-buyer%40example.com")
	if sent.Body.String() != wrong.Body.String() {
		t.Fatal("lookup reply reveals whether address matches")
	}
	challenge := responseCookie(t, sent, guestChallengeCookie)
	if !challenge.HttpOnly || challenge.Path != "/order/private-order" || challenge.SameSite != http.SameSiteLaxMode {
		t.Fatal("unsafe challenge cookie")
	}
	code := guestCodeFromDB(t, h)
	failed := guestRequest(t, mux, "POST", "/order/private-order/access/verify", "code="+code)
	assertNoPrivateOrder(t, failed) // email code alone, in a different browser, is insufficient.
	verified := guestRequest(t, mux, "POST", "/order/private-order/access/verify", "code="+code, challenge)
	if verified.Code != 303 {
		t.Fatalf("verify status=%d", verified.Code)
	}
	access := responseCookie(t, verified, guestAccessCookie)
	if access.Secure || !access.HttpOnly || access.Path != "/order/private-order" || access.MaxAge != int(order.GuestAccessTTL.Seconds()) {
		t.Fatal("unsafe access cookie")
	}
	page := guestRequest(t, mux, "GET", "/order/private-order", "", access)
	if !strings.Contains(page.Body.String(), "Secret street 92") {
		t.Fatal("verified customer cannot view order")
	}
	other := guestRequest(t, mux, "GET", "/order/second-private-order", "", access)
	assertNoPrivateOrder(t, other)
	queryToken := guestRequest(t, mux, "GET", "/order/private-order?token="+access.Value, "")
	assertNoPrivateOrder(t, queryToken)
	closed := guestRequest(t, mux, "POST", "/order/private-order/access/logout", "", access)
	if closed.Code != 303 || responseCookie(t, closed, guestAccessCookie).MaxAge >= 0 {
		t.Fatal("logout did not clear cookie")
	}
	assertNoPrivateOrder(t, guestRequest(t, mux, "GET", "/order/private-order", "", access))
}
func TestGuestCheckoutSetsScopedGrantBeforeGatewayRedirect(t *testing.T) {
	h, _ := newHTTP(t)
	h.pay = fakeGateway{methods: []string{"stripe"}, url: "https://gateway.example/checkout"}
	if _, err := h.svc.db.Exec(`INSERT INTO shipping_zone(public_id,name,countries,rate_cents,free_over_cents) VALUES('guest-zone','Guest zone','US',0,0)`); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.Register(mux)
	var variant string
	if err := h.svc.db.QueryRow(`SELECT public_id FROM variant ORDER BY id LIMIT 1`).Scan(&variant); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "https://shop.example/cart/items", strings.NewReader(`{"variant":"`+variant+`","quantity":1}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://shop.example")
	add := httptest.NewRecorder()
	mux.ServeHTTP(add, r)
	if add.Code != 200 {
		t.Fatalf("add %d %s", add.Code, add.Body.String())
	}
	cart := responseCookie(t, add, cartCookie)
	form := url.Values{"email": {"checkout@example.com"}, "name": {"New customer"}, "address": {"Checkout address"}, "country": {"US"}}
	checkout := guestRequest(t, mux, "POST", "https://shop.example/checkout", form.Encode(), cart)
	if checkout.Code != 303 || checkout.Header().Get("Location") != "https://gateway.example/checkout" {
		t.Fatalf("checkout=%d %s", checkout.Code, checkout.Body.String())
	}
	grant := responseCookie(t, checkout, guestAccessCookie)
	if !grant.Secure || !grant.HttpOnly || !strings.HasPrefix(grant.Path, "/order/") {
		t.Fatal("TLS checkout grant flags")
	}
	returned := guestRequest(t, mux, "GET", "https://shop.example"+grant.Path, "", grant)
	if returned.Code != 200 || !strings.Contains(returned.Body.String(), "Checkout address") {
		t.Fatal("provider return lost browser order access")
	}
	assertNoPrivateOrderForCheckout := guestRequest(t, mux, "GET", "https://shop.example"+grant.Path, "")
	if strings.Contains(assertNoPrivateOrderForCheckout.Body.String(), "Checkout address") {
		t.Fatal("checkout data leaked without cookie")
	}
}
func TestGuestRecoveryCSRFUnavailableMailAndLimits(t *testing.T) {
	h, _ := newHTTP(t)
	seedPrivateOrder(t, h)
	h.accessLimit = newCommerceLimiter(2)
	mux := http.NewServeMux()
	h.Register(mux)
	page := guestRequest(t, mux, "GET", "/order/private-order", "")
	if !strings.Contains(page.Body.String(), "temporarily unavailable") {
		t.Fatal("missing SMTP fallback")
	}
	r := httptest.NewRequest("POST", "/order/private-order/access/request", strings.NewReader("email=private-buyer%40example.com"))
	r.Header.Set("Origin", "https://evil.example")
	denied := httptest.NewRecorder()
	mux.ServeHTTP(denied, r)
	if denied.Code != 403 {
		t.Fatal("cross-origin code request accepted")
	}
	for i := 0; i < 3; i++ {
		w := guestRequest(t, mux, "POST", "/order/private-order/access/request", "email=private-buyer%40example.com")
		if i == 2 && w.Code != 429 {
			t.Fatal("access IP limiter missing")
		}
	}
	var count int
	if err := h.svc.db.QueryRow(`SELECT COUNT(*) FROM email_outbox`).Scan(&count); err != nil || count != 0 {
		t.Fatal("unavailable mail should not enqueue codes")
	}
}
