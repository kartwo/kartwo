// 售后后台权限测试 / After Sale Admin Boundary Tests
// 功能：真实登录/邀请/改权、资料和CSRF保护、版本重放审计及与实际退款的权限隔离
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 21:35:39
package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kartwo/kartwo/internal/order"
)

func afterSaleAPIOrder(t *testing.T, h *HTTP) (string, string, order.AfterSaleCase) {
	t.Helper()
	id := uuid.Must(uuid.NewV7()).String()
	db := h.svc.db
	if _, err := db.Exec(`INSERT INTO customer(id,public_id,email) VALUES(901,'after-sale-customer','after-sale-buyer@example.invalid')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO "order"(public_id,customer_id,status,email,ship_name,ship_address,currency,subtotal_cents,total_cents) VALUES(?,901,'paid','after-sale-buyer@example.invalid','Private sale name','Private sale street','USD',5000,5000)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO payment_receipt(order_id,provider,payment_ref,amount_cents,currency) SELECT id,'stripe','after-sale-payment-private',5000,'USD' FROM "order" WHERE public_id=?`, id); err != nil {
		t.Fatal(err)
	}
	browser, err := order.NewGuestToken()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = h.orders.RequestGuestAccess(ctx, id, "after-sale-buyer@example.invalid", browser); err != nil {
		t.Fatal(err)
	}
	var body string
	if err = db.QueryRow(`SELECT body FROM email_outbox WHERE kind GLOB 'guest_access:*' ORDER BY id DESC LIMIT 1`).Scan(&body); err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile(`code is: ([0-9]{8})`).FindStringSubmatch(body)
	if len(code) != 2 {
		t.Fatal("guest code missing")
	}
	token, err := h.orders.VerifyGuestAccess(ctx, id, browser, code[1])
	if err != nil {
		t.Fatal(err)
	}
	c, err := h.orders.CreateAfterSale(ctx, id, token, order.AfterSaleInput{Key: uuid.Must(uuid.NewV7()).String(), Kind: "refund", Reason: "PRIVATE-AFTERSALE-REASON", AmountCents: 1200})
	if err != nil {
		t.Fatal(err)
	}
	return id, token, c
}

func afterSaleAPIStaff(t *testing.T, mux http.Handler, owner []*http.Cookie, csrf string) (string, func() ([]*http.Cookie, string)) {
	t.Helper()
	var role struct {
		ID string `json:"public_id"`
	}
	flowDecode(t, flowJSON(t, mux, "POST", "/admin/api/roles", `{"name":"After sale staff","permissions":["orders.view"]}`, owner, csrf, 201).Body, &role)
	var invitation struct {
		Path string `json:"invite_path"`
	}
	flowDecode(t, flowJSON(t, mux, "POST", "/admin/api/users", `{"username":"aftersale-staff","display_name":"After sale staff","role_id":"`+role.ID+`"}`, owner, csrf, 201).Body, &invitation)
	link, err := url.Parse(invitation.Path)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := url.Parse(link.Fragment)
	if err != nil {
		t.Fatal(err)
	}
	acceptance, err := json.Marshal(map[string]string{"token": fragment.Query().Get("token"), "password": "fixture-after-sale-password"})
	if err != nil {
		t.Fatal(err)
	}
	flowJSON(t, mux, "POST", "/admin/api/invites/accept", string(acceptance), nil, "", 200)
	return role.ID, func() ([]*http.Cookie, string) {
		response := flowJSON(t, mux, "POST", "/admin/api/login", `{"username":"aftersale-staff","password":"fixture-after-sale-password"}`, nil, "", 200)
		return []*http.Cookie{flowCookie(t, response.Cookies, sessionCookie)}, flowCookie(t, response.Cookies, csrfCookie).Value
	}
}

func TestAfterSaleAPIStaffPrivacyReviewReplayAndNoRefundAuthority(t *testing.T) {
	h, mux := newHTTP(t)
	owner, csrf := loginAndCookies(t, mux)
	auth := []*http.Cookie{owner}
	id, token, c := afterSaleAPIOrder(t, h)
	fake := &fakeRefundRequests{}
	h.pay = fake
	path := "/admin/api/orders/" + id + "/after-sales"
	actionPath := path + "/" + c.PublicID + "/actions"
	role, login := afterSaleAPIStaff(t, mux, auth, csrf)
	staff, _ := login()
	flowJSON(t, mux, "GET", path, "", nil, "", 401)
	for _, account := range [][]*http.Cookie{staff, nil} {
		want := 403
		if account == nil {
			want = 401
		}
		response := flowJSON(t, mux, "GET", path, "", account, "", want)
		for _, secret := range []string{c.PublicID, "PRIVATE-AFTERSALE-REASON", "after-sale-buyer", token} {
			if bytes.Contains(response.Body, []byte(secret)) {
				t.Fatal("unauthorized sale data")
			}
		}
	}
	for _, permissions := range []string{`["orders.aftersales"]`, `["orders.view","orders.aftersales"]`, `["orders.customer_data","orders.aftersales"]`} {
		flowJSON(t, mux, "PATCH", "/admin/api/roles/"+role, `{"name":"After sale staff","permissions":`+permissions+`}`, auth, csrf, 400)
	}
	flowJSON(t, mux, "PATCH", "/admin/api/roles/"+role, `{"name":"After sale staff","permissions":["orders.view","orders.customer_data"]}`, auth, csrf, 200)
	flowJSON(t, mux, "GET", path, "", staff, "", 401)
	staff, staffCSRF := login()
	var view struct {
		OrderID  string                  `json:"order_id"`
		Snapshot order.AfterSaleSnapshot `json:"snapshot"`
		Key      string                  `json:"next_action_key"`
	}
	flowDecode(t, flowJSON(t, mux, "GET", path, "", staff, "", 200).Body, &view)
	key, err := uuid.Parse(view.Key)
	if err != nil || key.Version() != 7 || view.OrderID != id || len(view.Snapshot.Cases) != 1 || view.Snapshot.Cases[0].Reason != "PRIVATE-AFTERSALE-REASON" {
		t.Fatal("private history or server nonce missing", err)
	}
	payload := `{"request_key":"` + view.Key + `","expected_version":1,"action":"approve","message":"Approved; awaiting the actual refund."}`
	flowJSON(t, mux, "POST", actionPath, payload, staff, staffCSRF, 403)
	flowJSON(t, mux, "PATCH", "/admin/api/roles/"+role, `{"name":"After sale staff","permissions":["orders.view","orders.customer_data","orders.aftersales"]}`, auth, csrf, 200)
	flowJSON(t, mux, "GET", path, "", staff, "", 401)
	staff, staffCSRF = login()
	if _, ok := h.svc.Key(staff[0].Value); ok {
		t.Fatal("review permission granted merchant encryption key")
	}
	flowJSON(t, mux, "POST", actionPath, payload, staff, "", 403)
	for range 2 {
		flowJSON(t, mux, "POST", actionPath, payload, staff, staffCSRF, 200)
	}
	var result struct {
		Case order.AfterSaleCase `json:"case"`
	}
	flowDecode(t, flowJSON(t, mux, "POST", actionPath, payload, staff, staffCSRF, 200).Body, &result)
	if result.Case.State != "approved" || result.Case.Version != 2 || len(result.Case.Events) != 2 {
		t.Fatal("review/replay duplicated state")
	}
	flowJSON(t, mux, "POST", actionPath, `{"request_key":"`+uuid.Must(uuid.NewV7()).String()+`","expected_version":1,"action":"reject","message":"Stale browser"}`, auth, csrf, 409)
	flowJSON(t, mux, "POST", "/admin/api/orders/"+id+"/refunds", `{"request_key":"browser-refund-unit-key","provider":"stripe","payment_ref":"after-sale-payment-private","amount_cents":1200,"expected_refunded_cents":0}`, staff, staffCSRF, 403)
	if fake.calls != 0 || fake.refreshes != 0 || fake.checks != 0 {
		t.Fatal("sale approval/refund denial invoked gateway")
	}
	var receipts, refunds, shipments, outbox int
	if err = h.svc.db.QueryRow(`SELECT (SELECT COUNT(*) FROM payment_receipt),(SELECT COUNT(*) FROM refund_request),(SELECT COUNT(*) FROM shipment),(SELECT COUNT(*) FROM email_outbox)`).Scan(&receipts, &refunds, &shipments, &outbox); err != nil || receipts != 1 || refunds != 0 || shipments != 0 || outbox != 1 {
		t.Fatal("review changed money, fulfillment or notifications", err)
	}
	audits := flowJSON(t, mux, "GET", "/admin/api/audit-events", "", auth, "", 200).Body
	if bytes.Count(audits, []byte(`"action":"order.after_sale.approve"`)) != 1 {
		t.Fatal("replay duplicated operational audit")
	}
	for _, secret := range []string{"PRIVATE-AFTERSALE-REASON", "after-sale-buyer", token, view.Key, "Private sale street", "after-sale-payment-private"} {
		if bytes.Contains(audits, []byte(secret)) {
			t.Fatal("sale audit leaked private data")
		}
	}
	// Enforce both dependencies even if a role row was incorrectly written.
	if _, err = h.svc.db.Exec(`UPDATE admin_role SET permissions='["orders.aftersales","orders.customer_data"]' WHERE public_id=?`, role); err != nil {
		t.Fatal(err)
	}
	flowJSON(t, mux, "POST", actionPath, payload, staff, staffCSRF, 403)
}

func TestAfterSaleAPIRejectsDemoMalformedAndCrossOrder(t *testing.T) {
	h, mux := newHTTP(t)
	owner, csrf := loginAndCookies(t, mux)
	auth := []*http.Cookie{owner}
	id, _, c := afterSaleAPIOrder(t, h)
	path := "/admin/api/orders/" + id + "/after-sales/" + c.PublicID + "/actions"
	for _, body := range []string{
		`{}`, `{"request_key":"` + uuid.Must(uuid.NewV7()).String() + `","action":"approve","message":"Missing version"}`,
		`{"request_key":"` + uuid.Must(uuid.NewV7()).String() + `","expected_version":1,"action":"approve","message":"Approved","unknown_field":true}`,
		`{"request_key":"` + uuid.Must(uuid.NewV7()).String() + `","expected_version":1,"action":"approve","message":""}`,
		`{"request_key":"` + uuid.Must(uuid.NewV7()).String() + `","expected_version":1,"action":"complete","refund_request_id":"not-a-refund"}`,
	} {
		flowJSON(t, mux, "POST", path, body, auth, csrf, 400)
	}
	other := uuid.Must(uuid.NewV7()).String()
	if _, err := h.svc.db.Exec(`INSERT INTO "order"(public_id,customer_id,status,email,ship_name,ship_address,currency,subtotal_cents,total_cents) SELECT ?,customer_id,status,email,ship_name,ship_address,currency,subtotal_cents,total_cents FROM "order" WHERE public_id=?`, other, id); err != nil {
		t.Fatal(err)
	}
	payload := `{"request_key":"` + uuid.Must(uuid.NewV7()).String() + `","expected_version":1,"action":"reject","message":"Cannot act on another order"}`
	flowJSON(t, mux, "POST", "/admin/api/orders/"+other+"/after-sales/"+c.PublicID+"/actions", payload, auth, csrf, 404)
	h.ConfigureDemo(DemoConfig{Enabled: true, SessionTTL: time.Hour, MaxProducts: 3})
	demo := flowJSON(t, mux, "POST", "/admin/api/demo-session", `{}`, nil, "", 201)
	demoCSRF := flowCookie(t, demo.Cookies, csrfCookie).Value
	flowJSON(t, mux, "GET", "/admin/api/orders/"+id+"/after-sales", "", demo.Cookies, "", 403)
	flowJSON(t, mux, "POST", path, payload, demo.Cookies, demoCSRF, 403)
	history, err := h.orders.AdminAfterSales(context.Background(), id)
	if err != nil || history.Cases[0].State != "requested" || history.Cases[0].Version != 1 {
		t.Fatal("denied operation changed sale", err)
	}
}
