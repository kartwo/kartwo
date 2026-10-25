// 订单查询权限测试 / Order Search Permission Tests
// 功能：验证请求体搜索的鉴权/CSRF/角色/客户搜索隔离及审计最小化
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 13:42:16
package admin

import (
	"bytes"
	"net/http"
	"testing"
	"time"
)

func TestOrderSearchAuthCSRFPrivacyAndValidation(t *testing.T) {
	h, mux := newHTTP(t)
	owner, csrf := loginAndCookies(t, mux)
	db := h.svc.db
	for _, statement := range []string{
		`INSERT INTO customer(id,public_id,email) VALUES(901,'search-customer','search-private@example.invalid')`,
		`INSERT INTO "order"(id,public_id,customer_id,status,email,ship_name,ship_address,subtotal_cents,total_cents) VALUES(901,'search-order-901',901,'paid','search-private@example.invalid','Fixture','Fixture',100,100)`,
		`INSERT INTO admin_role(id,public_id,name,permissions) VALUES(901,'search-role','Search role','["orders.view"]')`,
		`INSERT INTO admin_user(id,public_id,username,password_hash,account_type,role_id) VALUES(901,'search-staff','search-staff','unused-test-hash','staff',901)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO session(token,admin_id,csrf_token,expires_at) VALUES('search-staff-session',901,'search-staff-csrf',?)`, time.Now().Add(time.Hour).UTC().Format(timeLayout)); err != nil {
		t.Fatal(err)
	}
	staff := []*http.Cookie{{Name: sessionCookie, Value: "search-staff-session"}}
	const path = "/admin/api/orders/search"
	ownerResult := doJSON(t, mux, "POST", path, `{"term":"search-private@example.invalid"}`, []*http.Cookie{owner}, csrf)
	if ownerResult.StatusCode != 200 || !bytes.Contains(ownerResult.Body, []byte(`"total":1`)) {
		t.Fatalf("owner search=%d %s", ownerResult.StatusCode, ownerResult.Body)
	}
	for _, test := range []struct {
		cookies []*http.Cookie
		csrf    string
		want    int
	}{{nil, "", 401}, {[]*http.Cookie{owner}, "", 403}, {staff, "", 403}} {
		res := doJSON(t, mux, "POST", path, `{}`, test.cookies, test.csrf)
		if res.StatusCode != test.want {
			t.Fatalf("search guard=%d want=%d", res.StatusCode, test.want)
		}
	}
	for _, body := range []string{`{"term":"search-private@example.invalid"}`, `{"term":"search-order"}`} {
		res := doJSON(t, mux, "POST", path, body, staff, "search-staff-csrf")
		if res.StatusCode != 200 || bytes.Contains(res.Body, []byte("search-private@example.invalid")) {
			t.Fatalf("staff query=%d %s", res.StatusCode, res.Body)
		}
		if bytes.Contains([]byte(body), []byte("@")) && !bytes.Contains(res.Body, []byte(`"total":0`)) {
			t.Fatal("email search leaked order existence")
		}
		if !bytes.Contains([]byte(body), []byte("@")) && !bytes.Contains(res.Body, []byte(`"total":1`)) {
			t.Fatal("public ID search blocked")
		}
	}
	for _, body := range []string{`{"customer_data":true}`, `{"view":"bogus"}`, `{"page_size":101}`, `{"page":1.5}`, `{"to":"2026-02-30"}`} {
		if res := doJSON(t, mux, "POST", path, body, staff, "search-staff-csrf"); res.StatusCode != 400 {
			t.Fatalf("invalid body accepted: %s = %d", body, res.StatusCode)
		}
	}
	if _, err := db.Exec(`UPDATE admin_role SET permissions='["orders.view","orders.customer_data"]' WHERE id=901`); err != nil {
		t.Fatal(err)
	}
	res := doJSON(t, mux, "POST", path, `{"term":"search-private@example.invalid"}`, staff, "search-staff-csrf")
	if res.StatusCode != 200 || !bytes.Contains(res.Body, []byte(`"total":1`)) || !bytes.Contains(res.Body, []byte("search-private@example.invalid")) {
		t.Fatalf("authorized search=%d %s", res.StatusCode, res.Body)
	}
	if _, err := db.Exec(`UPDATE admin_role SET permissions='[]' WHERE id=901`); err != nil {
		t.Fatal(err)
	}
	if res := doJSON(t, mux, "POST", path, `{}`, staff, "search-staff-csrf"); res.StatusCode != 403 {
		t.Fatal("staff without orders.view searched orders")
	}
	h.ConfigureDemo(DemoConfig{Enabled: true, SessionTTL: time.Hour, MaxProducts: 3})
	demo := doJSON(t, mux, "POST", "/admin/api/demo-session", `{}`, nil, "")
	var demoCSRF string
	for _, c := range demo.Cookies {
		if c.Name == csrfCookie {
			demoCSRF = c.Value
		}
	}
	res = doJSON(t, mux, "POST", path, `{}`, demo.Cookies, demoCSRF)
	if res.StatusCode != 200 || bytes.Contains(res.Body, []byte("search-private@example.invalid")) {
		t.Fatalf("demo query must remain readonly and private: %d %s", res.StatusCode, res.Body)
	}
	events := doJSON(t, mux, "GET", "/admin/api/audit-events", "", []*http.Cookie{owner}, "")
	if !bytes.Contains(events.Body, []byte(`"action":"order.customer_list_viewed"`)) || bytes.Contains(events.Body, []byte("search-private@example.invalid")) {
		t.Fatal("search audit missing or contains customer query")
	}
}
