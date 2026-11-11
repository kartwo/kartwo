// 运单通知接口测试 / Shipment Notice API Tests
// 功能：验证鉴权、CSRF、客户资料隔离、版本冲突、未知投递确认和最小审计
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 15:50:12
package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestShipmentAPIAuthPrivacyAndRecovery(t *testing.T) {
	h, mux := newHTTP(t)
	owner, csrf := loginAndCookies(t, mux)
	auth := []*http.Cookie{owner}
	db := h.svc.db
	for _, query := range []string{
		`INSERT INTO customer(id,public_id,email) VALUES(901,'shipment-customer','buyer-private@example.com')`,
		`INSERT INTO "order"(id,public_id,customer_id,status,email,ship_name,ship_address,subtotal_cents,total_cents) VALUES(901,'shipment-order',901,'paid','buyer-private@example.com','Private name','Private address',100,100)`,
		`INSERT INTO "order"(id,public_id,customer_id,status,email,ship_name,ship_address,subtotal_cents,total_cents) VALUES(902,'shipment-other',901,'paid','buyer-private@example.com','Private name','Private address',100,100)`,
		`INSERT INTO admin_role(id,public_id,name,permissions) VALUES(901,'shipment-role','Shipment role','["orders.view"]')`,
		`INSERT INTO admin_user(id,public_id,username,password_hash,account_type,role_id) VALUES(901,'shipment-staff','shipment-staff','unused-test-hash','staff',901)`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO session(token,admin_id,csrf_token,expires_at) VALUES('shipment-session',901,'shipment-csrf',?)`, time.Now().Add(time.Hour).UTC().Format(timeLayout)); err != nil {
		t.Fatal(err)
	}
	staff := []*http.Cookie{{Name: sessionCookie, Value: "shipment-session"}}
	const path = "/admin/api/orders/shipment-order/shipment"
	const payload = `{"carrier":"Carrier","tracking_number":"PRIVATE-TRACK-1","tracking_url":"https://carrier.example/private-tracking"}`
	if res := doJSON(t, mux, "GET", path, "", nil, ""); res.StatusCode != 401 {
		t.Fatal("unauthenticated history", res.StatusCode)
	}
	for _, tc := range []struct {
		auth []*http.Cookie
		csrf string
		want int
	}{{auth, "", 403}, {staff, "shipment-csrf", 403}} {
		if res := doJSON(t, mux, "POST", "/admin/api/orders/shipment-order/fulfill", payload, tc.auth, tc.csrf); res.StatusCode != tc.want {
			t.Fatal("fulfillment guard", res.StatusCode)
		}
	}
	res := doJSON(t, mux, "POST", "/admin/api/orders/shipment-order/fulfill", payload, auth, csrf)
	if res.StatusCode != 200 {
		t.Fatal("fulfill", res.StatusCode, string(res.Body))
	}
	res = doJSON(t, mux, "POST", "/admin/api/orders/shipment-order/fulfill", payload, auth, csrf)
	if res.StatusCode != 200 {
		t.Fatal("idempotent fulfillment", res.StatusCode)
	}
	res = doJSON(t, mux, "GET", path, "", staff, "")
	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	for _, secret := range []string{"PRIVATE-TRACK-1", "private-tracking", "buyer-private", "Private name", "Private address", `"body"`, `"outbox_id"`} {
		if bytes.Contains(res.Body, []byte(secret)) {
			t.Fatal("history leaked", secret)
		}
	}
	var history struct {
		CurrentRevision string `json:"current_revision"`
		Notices         []struct {
			PublicID string `json:"public_id"`
		} `json:"notices"`
	}
	if err := json.Unmarshal(res.Body, &history); err != nil || history.CurrentRevision == "" || len(history.Notices) != 1 {
		t.Fatal("history contract", err)
	}
	update := `{"expected_revision":"` + history.CurrentRevision + `","carrier":"Carrier","tracking_number":"TRACK-2","tracking_url":""}`
	if res = doJSON(t, mux, "PATCH", path, update, staff, "shipment-csrf"); res.StatusCode != 403 {
		t.Fatal("view-only staff changed tracking")
	}
	if _, err := db.Exec(`UPDATE admin_role SET permissions='["orders.view","orders.fulfill"]' WHERE id=901`); err != nil {
		t.Fatal(err)
	}
	if res = doJSON(t, mux, "PATCH", path, update, staff, "shipment-csrf"); res.StatusCode != 403 {
		t.Fatal("staff without customer permission changed tracking")
	}
	if _, err := db.Exec(`UPDATE admin_role SET permissions='["orders.view","orders.fulfill","orders.customer_data"]' WHERE id=901`); err != nil {
		t.Fatal(err)
	}
	if res = doJSON(t, mux, "PATCH", path, update, staff, ""); res.StatusCode != 403 {
		t.Fatal("missing csrf allowed tracking write")
	}
	if res = doJSON(t, mux, "PATCH", path, `{"carrier":"C","tracking_number":"N"}`, staff, "shipment-csrf"); res.StatusCode != 400 {
		t.Fatal("missing version accepted")
	}
	if res = doJSON(t, mux, "PATCH", path, update, staff, "shipment-csrf"); res.StatusCode != 200 {
		t.Fatal("authorized update", res.StatusCode, string(res.Body))
	}
	stale := `{"expected_revision":"` + history.CurrentRevision + `","carrier":"Stale","tracking_number":"TRACK-3"}`
	if res = doJSON(t, mux, "PATCH", path, stale, auth, csrf); res.StatusCode != 409 {
		t.Fatal("stale write accepted")
	}
	res = doJSON(t, mux, "GET", path, "", auth, "")
	if err := json.Unmarshal(res.Body, &history); err != nil || len(history.Notices) != 2 {
		t.Fatal("updated history", err)
	}
	current := history.Notices[0].PublicID
	if _, err := db.Exec(`UPDATE email_outbox SET status='delivery_unknown',last_error='private SMTP echo' WHERE id=(SELECT outbox_id FROM shipment_notice WHERE public_id=?)`, current); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE shipment_notice SET created_at='2000-01-01T00:00:00.000Z' WHERE public_id=?`, current); err != nil {
		t.Fatal(err)
	}
	actionPath := "/admin/api/orders/shipment-order/shipment-notices/" + current + "/actions"
	action := `{"request_key":"notice-request-key-0001","action":"resend_after_check"}`
	if res = doJSON(t, mux, "POST", actionPath, action, auth, csrf); res.StatusCode != 400 {
		t.Fatal("unknown resend lacked acknowledgement")
	}
	if res = doJSON(t, mux, "POST", actionPath, `{"request_key":"notice-request-key-0001","action":"retry"}`, auth, csrf); res.StatusCode != 409 {
		t.Fatal("unknown result blindly retried")
	}
	action = `{"request_key":"notice-request-key-0001","action":"resend_after_check","confirm_unknown":true}`
	if res = doJSON(t, mux, "POST", actionPath, action, staff, "shipment-csrf"); res.StatusCode != 200 {
		t.Fatal("explicit acknowledged retry", res.StatusCode, string(res.Body))
	}
	if res = doJSON(t, mux, "POST", "/admin/api/orders/shipment-other/shipment-notices/"+current+"/actions", action, auth, csrf); res.StatusCode != 404 {
		t.Fatal("cross-order notice mutated")
	}
	res = doJSON(t, mux, "GET", path, "", auth, "")
	if bytes.Contains(res.Body, []byte("private SMTP echo")) {
		t.Fatal("raw SMTP error escaped")
	}
	h.ConfigureDemo(DemoConfig{Enabled: true, SessionTTL: time.Hour, MaxProducts: 3})
	demo := doJSON(t, mux, "POST", "/admin/api/demo-session", `{}`, nil, "")
	var demoCSRF string
	for _, cookie := range demo.Cookies {
		if cookie.Name == csrfCookie {
			demoCSRF = cookie.Value
		}
	}
	if demo.StatusCode != 201 || demoCSRF == "" {
		t.Fatal("demo setup")
	}
	for _, tc := range []struct{ method, path, body string }{{"GET", path, ""}, {"PATCH", path, update}, {"POST", actionPath, action}} {
		if res = doJSON(t, mux, tc.method, tc.path, tc.body, demo.Cookies, demoCSRF); res.StatusCode != 403 {
			t.Fatal("demo accessed shipment", tc.method, res.StatusCode)
		}
	}
	events := doJSON(t, mux, "GET", "/admin/api/audit-events", "", auth, "")
	if !bytes.Contains(events.Body, []byte(`"action":"order.shipment_updated"`)) || !bytes.Contains(events.Body, []byte(`"action":"order.shipment_notice.resend_after_check"`)) {
		t.Fatal("missing shipment audit")
	}
	for _, secret := range []string{"buyer-private", "PRIVATE-TRACK", "private-tracking", "notice-request-key", "private SMTP echo"} {
		if bytes.Contains(events.Body, []byte(secret)) {
			t.Fatal("audit leaked", secret)
		}
	}
}

func TestShipmentAPIDatabaseFailureDoesNotReportInvalidInput(t *testing.T) {
	h, mux := newHTTP(t)
	owner, csrf := loginAndCookies(t, mux)
	db := h.svc.db
	for _, query := range []string{
		`INSERT INTO customer(id,public_id,email) VALUES(901,'ship-failure-customer','buyer@example.com')`,
		`INSERT INTO "order"(id,public_id,customer_id,status,email,ship_name,ship_address,subtotal_cents,total_cents) VALUES(901,'ship-failure-order',901,'paid','buyer@example.com','Name','Address',100,100)`,
		`CREATE TRIGGER reject_shipment_notice BEFORE INSERT ON shipment_notice BEGIN SELECT RAISE(ABORT,'private DB failure'); END`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	res := doJSON(t, mux, "POST", "/admin/api/orders/ship-failure-order/fulfill", `{"carrier":"Carrier","tracking_number":"TRACK"}`, []*http.Cookie{owner}, csrf)
	if res.StatusCode != 500 || bytes.Contains(res.Body, []byte("private DB failure")) || bytes.Contains(res.Body, []byte("信息无效")) {
		t.Fatal("failed enqueue misreported", res.StatusCode, string(res.Body))
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM "order" WHERE id=901`).Scan(&status); err != nil || status != "paid" {
		t.Fatal("partial fulfillment committed", status, err)
	}
}
