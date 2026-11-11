// 订单隐私权限测试 / Order Privacy Permission Tests
// 功能：验证客户资料权限与最小审计、店主撤销入口的 CSRF/员工/演示边界和对象隔离
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 17:18:40
package admin

import (
	"bytes"
	"net/http"
	"testing"
	"time"
)

func TestOrderCustomerPrivacyAndOwnerAccessRevocation(t *testing.T) {
	h, mux := newHTTP(t)
	owner, csrf := loginAndCookies(t, mux)
	ownerAuth := []*http.Cookie{owner}
	db := h.svc.db
	for _, q := range []string{
		`INSERT INTO customer(id,public_id,email,name) VALUES(901,'privacy-customer','private@example.com','Private Name')`,
		`INSERT INTO "order"(id,public_id,customer_id,email,ship_name,ship_phone,ship_address,ship_country,subtotal_cents,total_cents) VALUES(901,'privacy-order',901,'private@example.com','Private Name','private-phone','private-address','US',100,100)`,
		`INSERT INTO "order"(id,public_id,customer_id,email,ship_name,ship_address,subtotal_cents,total_cents) VALUES(902,'another-order',901,'private@example.com','Private Name','private-address',100,100)`,
		`INSERT INTO order_guest_access(token_hash,order_id,expires_at) VALUES('digest-one',901,9999999999),('digest-two',902,9999999999)`,
		`INSERT INTO email_outbox(id,order_id,kind,to_addr,subject,body) VALUES(901,901,'guest_access:private-code','private@example.com','Code','private-email-code')`,
		`INSERT INTO order_guest_challenge(public_id,order_id,browser_hash,code_hash,outbox_id,expires_at,requested_at) VALUES('private-code',901,'browser-hash','code-hash',901,9999999999,1)`,
		`INSERT INTO admin_role(id,public_id,name,permissions) VALUES(901,'privacy-role','Privacy role','["orders.view"]')`,
		`INSERT INTO admin_user(id,public_id,username,password_hash,account_type,role_id) VALUES(901,'privacy-staff','privacy-staff','unused-test-hash','staff',901)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO session(token,admin_id,csrf_token,expires_at) VALUES('privacy-staff-token',901,'privacy-staff-csrf',?)`, time.Now().Add(time.Hour).UTC().Format(timeLayout)); err != nil {
		t.Fatal(err)
	}
	staff := []*http.Cookie{{Name: sessionCookie, Value: "privacy-staff-token"}}
	for _, path := range []string{"/admin/api/orders", "/admin/api/orders/privacy-order"} {
		res := doJSON(t, mux, "GET", path, "", staff, "")
		if res.StatusCode != 200 {
			t.Fatalf("staff view=%d", res.StatusCode)
		}
		for _, secret := range []string{"private@example.com", "Private Name", "private-phone", "private-address", `"ship_country":"US"`} {
			if bytes.Contains(res.Body, []byte(secret)) {
				t.Fatalf("staff missing customer permission saw %s", secret)
			}
		}
	}
	if _, err := db.Exec(`UPDATE admin_role SET permissions='["orders.view","orders.customer_data"]' WHERE id=901`); err != nil {
		t.Fatal(err)
	}
	res := doJSON(t, mux, "GET", "/admin/api/orders/privacy-order", "", staff, "")
	if res.StatusCode != 200 || !bytes.Contains(res.Body, []byte("private-address")) {
		t.Fatal("customer-data permission not applied")
	}
	revoke := "/admin/api/orders/privacy-order/revoke-access"
	for _, tc := range []struct {
		auth []*http.Cookie
		csrf string
		want int
	}{{nil, "", 401}, {ownerAuth, "", 403}, {staff, "privacy-staff-csrf", 403}} {
		if res := doJSON(t, mux, "POST", revoke, `{}`, tc.auth, tc.csrf); res.StatusCode != tc.want {
			t.Fatalf("revocation guard=%d want=%d", res.StatusCode, tc.want)
		}
	}
	h.ConfigureDemo(DemoConfig{Enabled: true, SessionTTL: time.Hour, MaxProducts: 3})
	demo := doJSON(t, mux, "POST", "/admin/api/demo-session", `{}`, nil, "")
	var demoCSRF string
	for _, c := range demo.Cookies {
		if c.Name == csrfCookie {
			demoCSRF = c.Value
		}
	}
	if demo.StatusCode != 201 || demoCSRF == "" {
		t.Fatal("demo setup failed")
	}
	if res := doJSON(t, mux, "POST", revoke, `{}`, demo.Cookies, demoCSRF); res.StatusCode != 403 {
		t.Fatal("demo revoked access")
	}
	if res := doJSON(t, mux, "POST", "/admin/api/orders/missing/revoke-access", `{}`, ownerAuth, csrf); res.StatusCode != 404 {
		t.Fatal("missing revocation must be 404")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM order_guest_access`).Scan(&count); err != nil || count != 2 {
		t.Fatal("denied request revoked grants")
	}
	if res := doJSON(t, mux, "POST", revoke, `{}`, ownerAuth, csrf); res.StatusCode != 200 {
		t.Fatalf("owner revoke: %d %s", res.StatusCode, res.Body)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM order_guest_access WHERE order_id=902`).Scan(&count); err != nil || count != 1 {
		t.Fatal("other order grant removed")
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM order_guest_access WHERE order_id=901`).Scan(&count); err != nil || count != 0 {
		t.Fatal("target grants retained")
	}
	var body string
	var consumed int64
	if err := db.QueryRow(`SELECT e.body,c.consumed_at FROM email_outbox e JOIN order_guest_challenge c ON c.outbox_id=e.id WHERE e.id=901`).Scan(&body, &consumed); err != nil || body != "" || consumed == 0 {
		t.Fatal("outstanding code not revoked")
	}
	events := doJSON(t, mux, "GET", "/admin/api/audit-events", "", ownerAuth, "")
	if bytes.Count(events.Body, []byte(`"action":"order.guest_access_revoked"`)) != 1 || !bytes.Contains(events.Body, []byte(`"action":"order.customer_data_viewed"`)) {
		t.Fatalf("missing audit events: %s", events.Body)
	}
	for _, secret := range []string{"private@example.com", "private-address", "private-email-code", "privacy-staff-token"} {
		if bytes.Contains(events.Body, []byte(secret)) {
			t.Fatal("audit leaked private data")
		}
	}
}
