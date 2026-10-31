// 多国结算入口测试 / International Checkout HTTP Tests
// 功能：验证只读报价、国家字段、错误保留与转义、价格变化确认、旧入口与访客隐私
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 18:47:20
package storefront

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func checkoutElement(t *testing.T, body, tag, attr, value string) map[string]string {
	t.Helper()
	root, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var found map[string]string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == tag {
			attrs := map[string]string{}
			for _, a := range n.Attr {
				attrs[a.Key] = a.Val
			}
			if attrs[attr] == value {
				found = attrs
				return
			}
		}
		for c := n.FirstChild; c != nil && found == nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return found
}
func reviewedQuote(t *testing.T, page *httptest.ResponseRecorder) string {
	t.Helper()
	if page.Code != 200 {
		t.Fatalf("review %d %s", page.Code, page.Body.String())
	}
	value := checkoutElement(t, page.Body.String(), "input", "name", "quote")["value"]
	if len(value) != 64 {
		t.Fatalf("missing reviewed quote: %s", page.Body.String())
	}
	return value
}
func checkoutCart(t *testing.T, h *HTTP, mux http.Handler) *http.Cookie {
	t.Helper()
	var variant string
	if err := h.svc.db.QueryRow(`SELECT v.public_id FROM variant v JOIN inventory i ON i.variant_id=v.id WHERE i.quantity>0 LIMIT 1`).Scan(&variant); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "https://shop.example/cart/items", strings.NewReader(`{"variant":"`+variant+`","quantity":1}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://shop.example")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("add: %d %s", w.Code, w.Body.String())
	}
	return responseCookie(t, w, cartCookie)
}
func deliveryFixture(t *testing.T, countries string) (*HTTP, http.Handler, *http.Cookie) {
	t.Helper()
	h, mux := newHTTP(t)
	if _, err := h.svc.db.Exec(`INSERT INTO shipping_zone(public_id,name,countries,rate_cents,free_over_cents) VALUES('checkout-zone','Standard delivery',?,500,0)`, countries); err != nil {
		t.Fatal(err)
	}
	return h, mux, checkoutCart(t, h, mux)
}
func assertCheckoutUnchanged(t *testing.T, h *HTTP) {
	t.Helper()
	for _, table := range []string{`"order"`, "customer", "order_guest_access", "email_outbox"} {
		var count int
		if err := h.svc.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s rows=%d err=%v", table, count, err)
		}
	}
	var quantity int
	var state string
	if err := h.svc.db.QueryRow(`SELECT SUM(quantity-reserved) FROM inventory`).Scan(&quantity); err != nil || quantity != 5 {
		t.Fatalf("stock=%d err=%v", quantity, err)
	}
	if err := h.svc.db.QueryRow(`SELECT status FROM cart LIMIT 1`).Scan(&state); err != nil || state != "active" {
		t.Fatalf("cart=%s err=%v", state, err)
	}
}
func validCheckoutForm() url.Values {
	return url.Values{"checkout_action": {"place"}, "country": {"US"}, "email": {"buyer@example.invalid"}, "name": {"Sample Customer"}, "phone": {"+15555550123"}, "line1": {"12 Sample Street"}, "line2": {"Unit 2"}, "city": {"Boston"}, "region": {"MA"}, "postal_code": {"02108"}}
}
func checkoutPost(t *testing.T, mux http.Handler, cookie *http.Cookie, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	return guestRequest(t, mux, "POST", "https://shop.example/checkout", form.Encode(), cookie)
}
func TestCheckoutReviewIsReadOnlyAndShowsFullTotal(t *testing.T) {
	h, mux, cookie := deliveryFixture(t, "US,GB,HK")
	script := httptest.NewRecorder()
	mux.ServeHTTP(script, httptest.NewRequest("GET", "/static/checkout.js", nil))
	if script.Code != 200 || !strings.HasPrefix(script.Header().Get("Content-Type"), "text/javascript") || !strings.Contains(script.Body.String(), "checkout-quote-stale") {
		t.Fatal("embedded checkout enhancement is unavailable")
	}

	initial := guestRequest(t, mux, "GET", "https://shop.example/checkout?email=URL-PRIVATE&country=US", "", cookie)
	if initial.Code != 200 || strings.Contains(initial.Body.String(), "URL-PRIVATE") || checkoutElement(t, initial.Body.String(), "input", "name", "quote")["value"] != "" {
		t.Fatal("GET trusted private query or selected multiple destinations")
	}
	disabled := checkoutElement(t, initial.Body.String(), "button", "id", "checkout-place")
	if _, ok := disabled["disabled"]; !ok {
		t.Fatal("unreviewed checkout enabled")
	}
	for _, action := range []string{"review", ""} {
		form := url.Values{"checkout_action": {action}, "country": {"US"}, "email": {"unfinished"}, "address": {"Old freeform"}}
		page := checkoutPost(t, mux, cookie, form)
		reviewedQuote(t, page)
		if !strings.Contains(page.Body.String(), "104.00") || !strings.Contains(page.Body.String(), "Standard delivery") || !strings.Contains(page.Body.String(), "Subtotal") || !strings.Contains(page.Body.String(), "Shipping") {
			t.Fatal("review omitted full total")
		}
		assertCheckoutUnchanged(t, h)
	}
}
func TestCheckoutCountryFieldsAndPreservedOptionalValues(t *testing.T) {
	h, mux, cookie := deliveryFixture(t, "US,GB,HK")
	for _, tc := range []struct{ country, required, absent string }{{"US", "region", "district"}, {"GB", "postal_code", "region"}, {"HK", "line1", "postal_code"}} {
		page := checkoutPost(t, mux, cookie, url.Values{"checkout_action": {"review"}, "country": {tc.country}})
		reviewedQuote(t, page)
		attrs := checkoutElement(t, page.Body.String(), "input", "name", tc.required)
		if _, ok := attrs["required"]; !ok {
			t.Fatalf("%s missing required %s", tc.country, tc.required)
		}
		if checkoutElement(t, page.Body.String(), "input", "name", tc.absent) != nil {
			t.Fatalf("%s unexpectedly shows %s", tc.country, tc.absent)
		}
	}
	kept := checkoutPost(t, mux, cookie, url.Values{"checkout_action": {"review"}, "country": {"HK"}, "postal_code": {"02108"}, "region": {"MA"}})
	if checkoutElement(t, kept.Body.String(), "input", "name", "postal_code")["value"] != "02108" || !strings.Contains(kept.Body.String(), "Retained from your previous input") {
		t.Fatal("country change silently lost fields")
	}
	assertCheckoutUnchanged(t, h)
}
func TestCheckoutAddressErrorsRetainEscapedBodyAndPaymentChoice(t *testing.T) {
	h, mux, cookie := deliveryFixture(t, "US,GB")
	h.pay = fakeGateway{methods: []string{"stripe", "paypal"}}
	form := validCheckoutForm()
	form.Set("quote", reviewedQuote(t, checkoutPost(t, mux, cookie, url.Values{"country": {"US"}})))
	form.Set("line1", `<img src=x onerror="alert(1)">`)
	form.Set("payment_method", "paypal")
	form.Set("postal_code", "invalid")
	page := checkoutPost(t, mux, cookie, form)
	if page.Code != 422 || !strings.Contains(page.Body.String(), "postal code format") || strings.Contains(page.Body.String(), `<img src=x`) {
		t.Fatalf("invalid address %d %s", page.Code, page.Body.String())
	}
	if checkoutElement(t, page.Body.String(), "input", "name", "line1")["value"] != form.Get("line1") || checkoutElement(t, page.Body.String(), "input", "name", "postal_code")["aria-invalid"] != "true" {
		t.Fatal("input lost or field unmarked")
	}
	if _, ok := checkoutElement(t, page.Body.String(), "input", "value", "paypal")["checked"]; !ok {
		t.Fatal("payment choice lost")
	}
	assertCheckoutUnchanged(t, h)
}
func TestCheckoutPlaceRequiresReviewedStructuredAddress(t *testing.T) {
	h, mux, cookie := deliveryFixture(t, "US")
	initial := guestRequest(t, mux, "GET", "https://shop.example/checkout", "", cookie)
	quote := reviewedQuote(t, initial)
	for _, tc := range []string{"missing-quote", "invalid-quote", "legacy-address", "missing-city"} {
		form := validCheckoutForm()
		form.Set("quote", quote)
		switch tc {
		case "missing-quote":
			form.Del("quote")
		case "invalid-quote":
			form.Set("quote", strings.Repeat("0", 64))
		case "legacy-address":
			form.Set("address", "Legacy body must not bypass structured checks")
			form.Del("line1")
		case "missing-city":
			form.Del("city")
		}
		page := checkoutPost(t, mux, cookie, form)
		if page.Code != 409 && page.Code != 422 {
			t.Fatalf("%s status=%d", tc, page.Code)
		}
		assertCheckoutUnchanged(t, h)
	}
	form := validCheckoutForm()
	form.Set("quote", quote)
	form.Set("total_cents", "1")
	form.Set("shipping_cents", "0")
	form.Set("currency", "EUR")
	placed := checkoutPost(t, mux, cookie, form)
	if placed.Code != 303 {
		t.Fatalf("no-JS place %d %s", placed.Code, placed.Body.String())
	}
	var total, shipping, version int
	var postal, currency string
	if err := h.svc.db.QueryRow(`SELECT total_cents,shipping_cents,ship_address_version,ship_postal_code,currency FROM "order"`).Scan(&total, &shipping, &version, &postal, &currency); err != nil || total != 10400 || shipping != 500 || version != 1 || postal != "02108" || currency != "USD" {
		t.Fatalf("wrong snapshot total=%d shipping=%d version=%d postal=%s currency=%s err=%v", total, shipping, version, postal, currency, err)
	}
	grant := responseCookie(t, placed, guestAccessCookie)
	shown := guestRequest(t, mux, "GET", "https://shop.example"+grant.Path, "", grant)
	if shown.Code != 200 || !strings.Contains(shown.Body.String(), "02108") || !strings.Contains(shown.Body.String(), "12 Sample Street") {
		t.Fatal("private order lost address snapshot")
	}
}
func TestCheckoutStaleQuoteMustBeExplicitlyConfirmedAgain(t *testing.T) {
	for _, tc := range []string{"shipping", "price", "country", "disabled-country"} {
		t.Run(tc, func(t *testing.T) {
			h, mux, cookie := deliveryFixture(t, "US,GB")
			form := validCheckoutForm()
			form.Set("quote", reviewedQuote(t, checkoutPost(t, mux, cookie, url.Values{"country": {"US"}})))
			switch tc {
			case "shipping":
				if _, err := h.svc.db.Exec(`UPDATE shipping_zone SET rate_cents=700`); err != nil {
					t.Fatal(err)
				}
			case "price":
				if _, err := h.svc.db.Exec(`UPDATE variant SET price_cents=10000 WHERE id IN (SELECT variant_id FROM inventory WHERE quantity>0)`); err != nil {
					t.Fatal(err)
				}
			case "country":
				form.Set("country", "GB")
				form.Set("postal_code", "SW1A 1AA")
			case "disabled-country":
				if _, err := h.svc.db.Exec(`UPDATE shipping_zone SET countries='GB'`); err != nil {
					t.Fatal(err)
				}
			}
			page := checkoutPost(t, mux, cookie, form)
			want := 409
			if tc == "disabled-country" {
				want = 422
			}
			if page.Code != want {
				t.Fatalf("status=%d want=%d %s", page.Code, want, page.Body.String())
			}
			assertCheckoutUnchanged(t, h)
			next := checkoutElement(t, page.Body.String(), "input", "name", "quote")["value"]
			if tc == "disabled-country" {
				if next != "" || !strings.Contains(page.Body.String(), "Delivery is not available") {
					t.Fatal("disabled destination kept quote")
				}
				return
			}
			if len(next) != 64 || next == form.Get("quote") || checkoutElement(t, page.Body.String(), "input", "name", "line1")["value"] != form.Get("line1") {
				t.Fatal("new quote/input missing")
			}
			form.Set("quote", next)
			if retry := checkoutPost(t, mux, cookie, form); retry.Code != 303 {
				t.Fatalf("explicit reconfirm %d %s", retry.Code, retry.Body.String())
			}
		})
	}
}
func TestCheckoutUnavailableDestinationsAndProductsFailClosed(t *testing.T) {
	for _, tc := range []string{"no-destinations", "unsupported", "removed-product", "bad-rate"} {
		t.Run(tc, func(t *testing.T) {
			h, mux, cookie := deliveryFixture(t, "US")
			country := "US"
			want := 422
			switch tc {
			case "no-destinations":
				if _, err := h.svc.db.Exec(`UPDATE shipping_zone SET active=0`); err != nil {
					t.Fatal(err)
				}
			case "unsupported":
				country = "CA"
			case "removed-product":
				want = 409
				if _, err := h.svc.db.Exec(`UPDATE product SET status='draft'`); err != nil {
					t.Fatal(err)
				}
			case "bad-rate":
				want = 500
				if _, err := h.svc.db.Exec(`PRAGMA ignore_check_constraints=ON; UPDATE shipping_zone SET rate_cents=-1; PRAGMA ignore_check_constraints=OFF`); err != nil {
					t.Fatal(err)
				}
			}
			page := checkoutPost(t, mux, cookie, url.Values{"country": {country}, "line1": {"Preserved Street"}})
			if page.Code != want || checkoutElement(t, page.Body.String(), "input", "name", "quote")["value"] != "" || checkoutElement(t, page.Body.String(), "input", "name", "line1")["value"] != "Preserved Street" {
				t.Fatalf("unsafe failed quote %d %s", page.Code, page.Body.String())
			}
			if _, ok := checkoutElement(t, page.Body.String(), "button", "id", "checkout-place")["disabled"]; !ok {
				t.Fatal("failed quote permits submit")
			}
			assertCheckoutUnchanged(t, h)
		})
	}
}
