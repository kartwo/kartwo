// 商家结算贯通测试 / Merchant Checkout Integration Flow
// 功能：以正式后台与店面HTTP路径验证发布、配送、报价确认、订单工作台、真实邀请角色和游客隐私
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 20:09:25
package admin

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kartwo/kartwo/internal/cart"
	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/order"
	"github.com/kartwo/kartwo/internal/redirect"
	"github.com/kartwo/kartwo/internal/settings"
	"github.com/kartwo/kartwo/internal/storefront"
	"github.com/kartwo/kartwo/migrations"
	"golang.org/x/net/html"
)

// These helpers use real handlers and response bodies. No order, role, session,
// stock or payment state is inserted directly into SQL to make the flow pass.
func flowJSON(t *testing.T, mux http.Handler, method, path, body string, cookies []*http.Cookie, csrf string, status int) apiResp {
	t.Helper()
	response := doJSON(t, mux, method, path, body, cookies, csrf)
	if response.StatusCode != status {
		t.Fatalf("%s %s: status=%d want=%d", method, path, response.StatusCode, status)
	}
	return response
}
func flowDecode(t *testing.T, body []byte, value any) {
	t.Helper()
	if err := json.Unmarshal(body, value); err != nil {
		t.Fatalf("response contract: %v", err)
	}
}
func flowRequest(t *testing.T, mux http.Handler, method, path, body, contentType string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "https://store.example"+path, strings.NewReader(body))
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	if method == http.MethodPost {
		if contentType == "application/x-www-form-urlencoded" {
			request.Header.Set("Origin", "null")
		} else {
			request.Header.Set("Origin", "https://store.example")
		}
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if path == "/checkout" || strings.HasPrefix(path, "/order/") {
		if !strings.Contains(response.Header().Get("Cache-Control"), "no-store") || response.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatal("private commerce response lost privacy headers")
		}
	}
	return response
}
func flowInput(t *testing.T, response *httptest.ResponseRecorder, name string) string {
	t.Helper()
	document, err := html.Parse(strings.NewReader(response.Body.String()))
	if err != nil {
		t.Fatal(err)
	}
	var value string
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "input" {
			attributes := map[string]string{}
			for _, attribute := range node.Attr {
				attributes[attribute.Key] = attribute.Val
			}
			if attributes["name"] == name {
				value = attributes["value"]
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(document)
	return value
}
func flowCookie(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("response missing cookie %s", name)
	return nil
}
func flowPrivateAbsent(t *testing.T, body []byte) {
	t.Helper()
	for _, private := range []string{"buyer-flow@example.invalid", "12 Flow Street", "Flow Buyer", "Unit FLOW", `"postal_code":"02108"`, "+15555550123"} {
		if bytes.Contains(body, []byte(private)) {
			t.Fatal("unprivileged response exposed a private fixture field")
		}
	}
}

func TestMerchantCheckoutOrderPrivacyFlow(t *testing.T) {
	adminHTTP, _ := newHTTP(t)
	mux := http.NewServeMux()
	adminHTTP.Register(mux)
	db := adminHTTP.svc.db
	customerHTTP := storefront.NewHTTP(storefront.New(db), cart.New(db), adminHTTP.orders, adminHTTP.settings, nil, redirect.New(db), "Flow store", "https://store.example", nil)
	customerHTTP.Register(mux)
	owner, ownerCSRF := loginAndCookies(t, mux)
	ownerAuth := []*http.Cookie{owner}

	// Merchant path: create an actual draft through the same API as the UI, then publish.
	product := flowJSON(t, mux, "POST", "/admin/api/products", `{"title":"Running Tee","slug":"flow-running-tee","status":"draft","options":[{"name":"Size","values":["S"]}],"variants":[{"price_cents":1200,"quantity":4,"sku":"FLOW-S","selections":[{"option":"Size","value":"S"}]}]}`, ownerAuth, ownerCSRF, 201)
	var created struct {
		PublicID string `json:"public_id"`
	}
	flowDecode(t, product.Body, &created)
	if created.PublicID == "" {
		t.Fatal("product public ID missing")
	}
	if response := flowRequest(t, mux, "GET", "/p/flow-running-tee", "", ""); response.Code != 404 {
		t.Fatal("draft leaked into public storefront")
	}
	flowJSON(t, mux, "PATCH", "/admin/api/products/"+created.PublicID, `{"title":"Running Tee","description":"A fixture running shirt","status":"active"}`, ownerAuth, ownerCSRF, 200)
	if response := flowRequest(t, mux, "GET", "/p/flow-running-tee", "", ""); response.Code != 200 || !strings.Contains(response.Body.String(), "Running Tee") {
		t.Fatal("published product missing from storefront")
	}
	var detail struct {
		Variants []struct {
			PublicID string `json:"public_id"`
		} `json:"variants"`
	}
	flowDecode(t, flowJSON(t, mux, "GET", "/admin/api/products/"+created.PublicID, "", ownerAuth, "", 200).Body, &detail)
	if len(detail.Variants) != 1 {
		t.Fatal("product variant contract")
	}
	added := flowRequest(t, mux, "POST", "/cart/items", `{"variant":"`+detail.Variants[0].PublicID+`","quantity":1}`, "application/json")
	if added.Code != 200 {
		t.Fatal("public cart add rejected")
	}
	cartCookie := flowCookie(t, added.Result().Cookies(), "kartwo_cart")

	form := url.Values{"checkout_action": {"review"}, "country": {"US"}, "email": {"buyer-flow@example.invalid"}, "name": {"Flow Buyer"}, "phone": {"+15555550123"}, "line1": {"12 Flow Street"}, "line2": {"Unit FLOW"}, "city": {"Boston"}, "region": {"MA"}, "postal_code": {"02108"}}
	post := func(values url.Values) *httptest.ResponseRecorder {
		return flowRequest(t, mux, "POST", "/checkout", values.Encode(), "application/x-www-form-urlencoded", cartCookie)
	}
	assertPending := func(orders, reserved int) {
		t.Helper()
		var orderCount, stock, reservedStock int
		if err := db.QueryRow(`SELECT COUNT(*) FROM "order"`).Scan(&orderCount); err != nil || orderCount != orders {
			t.Fatalf("order count=%d want=%d err=%v", orderCount, orders, err)
		}
		if err := db.QueryRow(`SELECT SUM(quantity),SUM(reserved) FROM inventory`).Scan(&stock, &reservedStock); err != nil || stock != 4 || reservedStock != reserved {
			t.Fatalf("stock=%d reserved=%d want=%d err=%v", stock, reservedStock, reserved, err)
		}
		for _, table := range []string{"shipment", "shipment_notice", "email_outbox"} {
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
				t.Fatalf("%s unexpected side effect: count=%d err=%v", table, count, err)
			}
		}
	}
	unavailable := post(form)
	if unavailable.Code != 422 || flowInput(t, unavailable, "quote") != "" {
		t.Fatal("empty delivery configuration silently enabled a destination")
	}
	assertPending(0, 0)
	flowJSON(t, mux, "PUT", "/admin/api/settings/shipping/countries", `{"countries":["US","GB"]}`, ownerAuth, ownerCSRF, 200)
	flowJSON(t, mux, "PUT", "/admin/api/settings/shipping/default", `{"rate_cents":500,"free_over_cents":0}`, ownerAuth, ownerCSRF, 200)
	form.Set("country", "CA")
	if response := post(form); response.Code != 422 || flowInput(t, response, "quote") != "" {
		t.Fatal("global delivery rate expanded enabled countries")
	}
	form.Set("country", "US")
	reviewed := post(form)
	quote := flowInput(t, reviewed, "quote")
	if reviewed.Code != 200 || len(quote) != 64 || !strings.Contains(reviewed.Body.String(), "17.00") {
		t.Fatal("merchant shipping setting did not reach storefront quote")
	}
	assertPending(0, 0)

	// A normal merchant settings change must invalidate the buyer's earlier confirmation.
	flowJSON(t, mux, "PUT", "/admin/api/settings/shipping/default", `{"rate_cents":700,"free_over_cents":0}`, ownerAuth, ownerCSRF, 200)
	form.Set("checkout_action", "place")
	form.Set("quote", quote)
	stale := post(form)
	fresh := flowInput(t, stale, "quote")
	if stale.Code != 409 || fresh == quote || len(fresh) != 64 || !strings.Contains(stale.Body.String(), "19.00") || flowInput(t, stale, "line1") != form.Get("line1") || flowInput(t, stale, "postal_code") != "02108" {
		t.Fatal("stale delivery confirmation was not safely refreshed")
	}
	assertPending(0, 0)
	form.Set("quote", fresh)
	placed := post(form)
	if placed.Code != 303 || !strings.HasPrefix(placed.Header().Get("Location"), "/order/") {
		t.Fatal("confirmed checkout failed")
	}
	orderPath := placed.Header().Get("Location")
	id := strings.TrimPrefix(orderPath, "/order/")
	grant := flowCookie(t, placed.Result().Cookies(), "kartwo_order_access")
	if grant.Path != orderPath || !grant.HttpOnly || !grant.Secure || grant.SameSite != http.SameSiteLaxMode {
		t.Fatal("guest grant was not scoped to the TLS order path")
	}
	if cleared := flowCookie(t, placed.Result().Cookies(), "kartwo_cart"); cleared.MaxAge >= 0 {
		t.Fatal("converted cart cookie was not cleared")
	}
	assertPending(1, 1)
	// Public cookie lookup returns only active carts; a consumed cookie is an empty cart.
	if replay := post(form); replay.Code != 400 {
		t.Fatalf("converted cart replay status=%d want=400", replay.Code)
	}
	assertPending(1, 1)

	// Both sides must read the exact snapshot that was committed by public checkout.
	viewed := flowRequest(t, mux, "GET", orderPath, "", "", grant)
	if viewed.Code != 200 || !strings.Contains(viewed.Body.String(), "12 Flow Street") || !strings.Contains(viewed.Body.String(), "02108") || !strings.Contains(viewed.Body.String(), "19.00") {
		t.Fatal("guest order lost address or money snapshot")
	}
	hidden := flowRequest(t, mux, "GET", orderPath, "", "")
	flowPrivateAbsent(t, hidden.Body.Bytes())
	other := flowRequest(t, mux, "GET", "/order/nonexistent-flow-order", "", "", grant)
	flowPrivateAbsent(t, other.Body.Bytes())
	if strings.ReplaceAll(hidden.Body.String(), id, "redacted-order") != strings.ReplaceAll(flowRequest(t, mux, "GET", "/order/nonexistent-flow-order", "", "").Body.String(), "nonexistent-flow-order", "redacted-order") {
		t.Fatal("public order page distinguished known and unknown private orders")
	}
	searchBody := `{"term":"` + id + `","view":"awaiting_payment"}`
	search := flowJSON(t, mux, "POST", "/admin/api/orders/search", searchBody, ownerAuth, ownerCSRF, 200)
	var found struct {
		Total  int `json:"total"`
		Orders []struct {
			ID    string `json:"public_id"`
			Total int    `json:"total_cents"`
		} `json:"orders"`
	}
	flowDecode(t, search.Body, &found)
	if found.Total != 1 || len(found.Orders) != 1 || found.Orders[0].ID != id || found.Orders[0].Total != 1900 {
		t.Fatal("created order missing from merchant workbench")
	}
	orderAPI := "/admin/api/orders/" + id
	var snapshot struct {
		Total    int               `json:"total_cents"`
		Shipping int               `json:"shipping_cents"`
		Currency string            `json:"currency"`
		Country  string            `json:"ship_country"`
		Address  map[string]string `json:"ship_address_details"`
	}
	expectedAddress := map[string]string{}
	for _, field := range []string{"line1", "line2", "city", "district", "region", "postal_code"} {
		expectedAddress[field] = form.Get(field)
	}
	flowDecode(t, flowJSON(t, mux, "GET", orderAPI, "", ownerAuth, "", 200).Body, &snapshot)
	if snapshot.Total != 1900 || snapshot.Shipping != 700 || snapshot.Currency != "USD" || snapshot.Country != "US" || !maps.Equal(snapshot.Address, expectedAddress) {
		t.Fatal("admin order address/money differs from reviewed checkout")
	}
	flowJSON(t, mux, "POST", orderAPI+"/fulfill", `{"carrier":"Fixture carrier","tracking_number":"FLOW-TRACK","tracking_url":""}`, ownerAuth, ownerCSRF, 409)
	assertPending(1, 1)

	// Invite and authenticate a real isolated employee; do not forge an authorization row.
	role := flowJSON(t, mux, "POST", "/admin/api/roles", `{"name":"Flow reader","permissions":["orders.view","orders.customer_data"]}`, ownerAuth, ownerCSRF, 201)
	var roleID struct {
		ID string `json:"public_id"`
	}
	flowDecode(t, role.Body, &roleID)
	invitation := flowJSON(t, mux, "POST", "/admin/api/users", `{"username":"flow-reader","display_name":"Flow reader","role_id":"`+roleID.ID+`"}`, ownerAuth, ownerCSRF, 201)
	var invite struct {
		Path string `json:"invite_path"`
	}
	flowDecode(t, invitation.Body, &invite)
	link, err := url.Parse(invite.Path)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := url.Parse(link.Fragment)
	if err != nil {
		t.Fatal(err)
	}
	token := fragment.Query().Get("token")
	if token == "" {
		t.Fatal("invitation missing one-time token")
	}
	acceptance, err := json.Marshal(map[string]string{"token": token, "password": "fixture-flow-password"})
	if err != nil {
		t.Fatal(err)
	}
	flowJSON(t, mux, "POST", "/admin/api/invites/accept", string(acceptance), nil, "", 200)
	login := func() ([]*http.Cookie, string) {
		t.Helper()
		response := flowJSON(t, mux, "POST", "/admin/api/login", `{"username":"flow-reader","password":"fixture-flow-password"}`, nil, "", 200)
		return []*http.Cookie{flowCookie(t, response.Cookies, sessionCookie)}, flowCookie(t, response.Cookies, csrfCookie).Value
	}
	staff, staffCSRF := login()
	if _, ok := adminHTTP.svc.Key(staff[0].Value); ok {
		t.Fatal("employee obtained merchant encryption key")
	}
	authorized := flowJSON(t, mux, "GET", orderAPI, "", staff, "", 200)
	if !bytes.Contains(authorized.Body, []byte(`"postal_code":"02108"`)) {
		t.Fatal("authorized employee lost structured address")
	}
	flowJSON(t, mux, "POST", orderAPI+"/revoke-access", `{}`, staff, staffCSRF, 403)
	flowJSON(t, mux, "PATCH", "/admin/api/roles/"+roleID.ID, `{"name":"Flow reader","permissions":["orders.view"]}`, ownerAuth, ownerCSRF, 200)
	flowJSON(t, mux, "GET", orderAPI, "", staff, "", 401)
	staff, staffCSRF = login()
	masked := flowJSON(t, mux, "GET", orderAPI, "", staff, "", 200)
	flowPrivateAbsent(t, masked.Body)
	if !bytes.Contains(masked.Body, []byte(`"ship_address_details":null`)) {
		t.Fatal("structured address object not redacted")
	}
	emailSearch := flowJSON(t, mux, "POST", "/admin/api/orders/search", `{"term":"buyer-flow@example.invalid"}`, staff, staffCSRF, 200)
	flowDecode(t, emailSearch.Body, &found)
	if found.Total != 0 {
		t.Fatal("employee inferred private email through search")
	}
	flowPrivateAbsent(t, emailSearch.Body)
	flowJSON(t, mux, "POST", orderAPI+"/fulfill", `{"carrier":"Fixture carrier","tracking_number":"FLOW-TRACK"}`, staff, staffCSRF, 403)
	flowJSON(t, mux, "POST", orderAPI+"/revoke-access", `{}`, ownerAuth, "", 403)
	stillAllowed := flowRequest(t, mux, "GET", orderPath, "", "", grant)
	if !strings.Contains(stillAllowed.Body.String(), "12 Flow Street") {
		t.Fatal("rejected revocation changed customer access")
	}
	flowJSON(t, mux, "POST", orderAPI+"/revoke-access", `{}`, ownerAuth, ownerCSRF, 200)
	revoked := flowRequest(t, mux, "GET", orderPath, "", "", grant)
	flowPrivateAbsent(t, revoked.Body.Bytes())
	assertPending(1, 1)
	audit := flowJSON(t, mux, "GET", "/admin/api/audit-events", "", ownerAuth, "", 200)
	for _, action := range []string{"product.create", "product.update", "shipping.default_update", "order.customer_data_viewed", "role.update", "order.guest_access_revoked"} {
		if !bytes.Contains(audit.Body, []byte(action)) {
			t.Fatalf("missing business audit %s", action)
		}
	}
	flowPrivateAbsent(t, audit.Body)

	// Close the actual SQLite file and rebuild the order service. Persistence
	// must not depend on an HTTP instance or restore a revoked guest grant.
	var filename string
	if err := db.QueryRow("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&filename); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sql.Open("sqlite", "file:"+filename+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	reopened.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = reopened.Close() })
	if applied, err := migrate.Run(context.Background(), reopened, migrations.FS); err != nil || applied != 0 {
		t.Fatalf("restart migrations=%d err=%v", applied, err)
	}
	db = reopened
	restarted := order.New(reopened, settings.New(reopened))
	persisted, err := restarted.Get(context.Background(), id)
	if err != nil || persisted.ShipAddressDetails == nil || persisted.ShipAddressDetails.PostalCode != "02108" || persisted.ShipAddressDetails.Text() != "12 Flow Street\nUnit FLOW\nBoston\nMA\n02108" || persisted.ShipCountry != "US" || persisted.TotalCents != 1900 || persisted.Status != "pending" {
		t.Fatalf("restart lost committed order/address snapshot: %v", err)
	}
	if authorized, err := restarted.GuestAuthorized(context.Background(), id, grant.Value); err != nil || authorized {
		t.Fatalf("restart restored revoked grant: authorized=%v err=%v", authorized, err)
	}
	assertPending(1, 1)
}
