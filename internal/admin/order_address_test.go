// 订单地址权限测试 / Order Address Permission Tests
// 功能：验证结构化收货地址复用客户资料权限，旧订单空值与最小审计不回归
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 17:31:26
package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestOrderAddressDetailsRespectCustomerPermission(t *testing.T) {
	h, mux := newHTTP(t)
	owner, _ := loginAndCookies(t, mux)
	db := h.svc.db
	for _, statement := range []string{
		`INSERT INTO customer(id,public_id,email) VALUES(901,'address-customer','address@example.invalid')`,
		`INSERT INTO "order"(id,public_id,customer_id,email,ship_name,ship_address,subtotal_cents,total_cents,ship_address_version,ship_address_line1,ship_address_line2,ship_city,ship_district,ship_region,ship_postal_code) VALUES(901,'address-order',901,'address@example.invalid','Fixture','PRIVATE-ADDRESS',100,100,1,'PRIVATE-LINE-1','PRIVATE-LINE-2','PRIVATE-CITY','PRIVATE-DISTRICT','PRIVATE-REGION','PRIVATE-POSTAL')`,
		`INSERT INTO "order"(id,public_id,customer_id,email,ship_name,ship_address,subtotal_cents,total_cents) VALUES(902,'legacy-address-order',901,'address@example.invalid','Fixture','Original address',100,100)`,
		`INSERT INTO admin_role(id,public_id,name,permissions) VALUES(901,'address-role','Address viewer','["orders.view"]')`,
		`INSERT INTO admin_user(id,public_id,username,password_hash,account_type,role_id) VALUES(901,'address-staff','address-staff','unused-fixture-hash','staff',901)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO session(token,admin_id,csrf_token,expires_at) VALUES('address-session',901,'address-csrf',?)`, time.Now().Add(time.Hour).UTC().Format(timeLayout)); err != nil {
		t.Fatal(err)
	}
	staff := []*http.Cookie{{Name: sessionCookie, Value: "address-session"}}
	const path = "/admin/api/orders/address-order"
	if res := doJSON(t, mux, "GET", path, "", nil, ""); res.StatusCode != 401 {
		t.Fatal("unauthenticated address allowed")
	}
	res := doJSON(t, mux, "GET", path, "", staff, "")
	if res.StatusCode != 200 || bytes.Contains(res.Body, []byte("PRIVATE-")) || !bytes.Contains(res.Body, []byte(`"ship_address_details":null`)) {
		t.Fatal("structured address leaked to unprivileged staff")
	}
	if _, err := db.Exec(`UPDATE admin_role SET permissions='["orders.view","orders.customer_data"]' WHERE id=901`); err != nil {
		t.Fatal(err)
	}
	for _, cookies := range [][]*http.Cookie{staff, {owner}} {
		res = doJSON(t, mux, "GET", path, "", cookies, "")
		var value struct {
			Address map[string]string `json:"ship_address_details"`
		}
		if res.StatusCode != 200 || json.Unmarshal(res.Body, &value) != nil || value.Address["city"] != "PRIVATE-CITY" || value.Address["postal_code"] != "PRIVATE-POSTAL" {
			t.Fatal("authorized structured snapshot missing")
		}
	}
	res = doJSON(t, mux, "GET", "/admin/api/orders/legacy-address-order", "", []*http.Cookie{owner}, "")
	if res.StatusCode != 200 || !bytes.Contains(res.Body, []byte(`"ship_address_details":null`)) || !bytes.Contains(res.Body, []byte("Original address")) {
		t.Fatal("legacy address rewritten or guessed")
	}
	var viewed int
	if err := db.QueryRow("SELECT count(*) FROM audit_event WHERE action='order.customer_data_viewed' AND target_public_id='address-order'").Scan(&viewed); err != nil || viewed != 2 {
		t.Fatalf("address audit count=%d err=%v", viewed, err)
	}
}
