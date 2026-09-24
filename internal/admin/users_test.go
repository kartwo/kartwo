// 后台用户权限测试 / Admin User Permission Tests
// 功能：邀请、员工登录、角色服务端鉴权、会话失效与店主隔离
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-24 15:00:00
package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/kartwo/kartwo/internal/audit"
)

func TestAuditKeepsUsernameAtActionTime(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	if err := svc.Initialize(ctx, "original-owner", "supersecret"); err != nil {
		t.Fatal(err)
	}
	sess, err := svc.Login(ctx, "original-owner", "supersecret")
	if err != nil {
		t.Fatal(err)
	}
	log := audit.New(svc.db)
	if err := log.Record(ctx, sess.AdminID, "user.test", "admin", sess.AdminPublicID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateOwnerCredentials(ctx, sess.AdminID, "renamed-owner", "supersecret", ""); err != nil {
		t.Fatal(err)
	}
	events, err := log.List(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].AdminUsername != "original-owner" {
		t.Fatalf("审计操作人用户名应为操作时快照: %+v", events)
	}
}

func TestStaffInvitationPermissionsAndDisable(t *testing.T) {
	h, mux := newHTTP(t)
	owner, ownerCSRF := loginAndCookies(t, mux)
	ownerAuth := []*http.Cookie{owner}
	viewerRole := "00000000-0000-7000-8000-000000000004"
	created := doJSON(t, mux, "POST", "/admin/api/users", `{"username":"viewer1","display_name":"Viewer","role_id":"`+viewerRole+`"}`, ownerAuth, ownerCSRF)
	if created.StatusCode != 201 {
		t.Fatalf("创建员工: %d %s", created.StatusCode, created.Body)
	}
	var invitation struct {
		PublicID   string `json:"public_id"`
		InvitePath string `json:"invite_path"`
	}
	if err := json.Unmarshal(created.Body, &invitation); err != nil {
		t.Fatal(err)
	}
	if invitation.PublicID == "" || invitation.InvitePath == "" {
		t.Fatalf("缺少邀请信息: %s", created.Body)
	}
	if resp := doJSON(t, mux, "POST", "/admin/api/login", `{"username":"viewer1","password":"staffsecret"}`, nil, ""); resp.StatusCode != 401 {
		t.Fatalf("未接受邀请不能登录: %d", resp.StatusCode)
	}
	u, err := url.Parse(invitation.InvitePath)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := url.Parse(u.Fragment)
	if err != nil {
		t.Fatal(err)
	}
	token := fragment.Query().Get("token")
	if token == "" {
		t.Fatal("邀请链接缺少 token")
	}
	acceptBody := `{"token":"` + token + `","password":"staffsecret"}`
	if resp := doJSON(t, mux, "POST", "/admin/api/invites/accept", acceptBody, nil, ""); resp.StatusCode != 200 {
		t.Fatalf("接受邀请: %d %s", resp.StatusCode, resp.Body)
	}
	if resp := doJSON(t, mux, "POST", "/admin/api/invites/accept", acceptBody, nil, ""); resp.StatusCode != 400 {
		t.Fatalf("重复使用邀请应失败: %d", resp.StatusCode)
	}
	login := doJSON(t, mux, "POST", "/admin/api/login", `{"username":"viewer1","password":"staffsecret"}`, nil, "")
	if login.StatusCode != 200 {
		t.Fatalf("员工登录: %d %s", login.StatusCode, login.Body)
	}
	var staff *http.Cookie
	var staffCSRF string
	for _, cookie := range login.Cookies {
		if cookie.Name == sessionCookie {
			staff = cookie
		}
		if cookie.Name == csrfCookie {
			staffCSRF = cookie.Value
		}
	}
	if staff == nil || staffCSRF == "" {
		t.Fatal("员工缺少会话 cookie")
	}
	if _, ok := h.svc.Key(staff.Value); ok {
		t.Fatal("员工会话不得持有店铺 KEK")
	}
	staffAuth := []*http.Cookie{staff}
	for _, path := range []string{"/admin/api/dashboard", "/admin/api/products", "/admin/api/orders"} {
		if resp := doJSON(t, mux, "GET", path, "", staffAuth, ""); resp.StatusCode != 200 {
			t.Fatalf("只读权限 %s: %d %s", path, resp.StatusCode, resp.Body)
		}
	}
	for _, path := range []string{"/admin/api/users", "/admin/api/settings/payment", "/admin/api/export"} {
		if resp := doJSON(t, mux, "GET", path, "", staffAuth, ""); resp.StatusCode != 403 {
			t.Fatalf("敏感接口 %s 应拒绝员工: %d", path, resp.StatusCode)
		}
	}
	if resp := doJSON(t, mux, "POST", "/admin/api/products", `{}`, staffAuth, staffCSRF); resp.StatusCode != 403 {
		t.Fatalf("只读员工不应创建商品: %d", resp.StatusCode)
	}
	if resp := doJSON(t, mux, "PUT", "/admin/api/account/password", `{"current_password":"staffsecret","new_password":"newstaffsecret"}`, staffAuth, staffCSRF); resp.StatusCode != 200 {
		t.Fatalf("员工修改自己的密码失败: %d %s", resp.StatusCode, resp.Body)
	}
	if resp := doJSON(t, mux, "GET", "/admin/api/me", "", staffAuth, ""); resp.StatusCode != 401 {
		t.Fatalf("改密后旧会话应失效: %d", resp.StatusCode)
	}
	if resp := doJSON(t, mux, "POST", "/admin/api/login", `{"username":"viewer1","password":"staffsecret"}`, nil, ""); resp.StatusCode != 401 {
		t.Fatalf("旧密码应失效: %d", resp.StatusCode)
	}
	newLogin := doJSON(t, mux, "POST", "/admin/api/login", `{"username":"viewer1","password":"newstaffsecret"}`, nil, "")
	if newLogin.StatusCode != 200 {
		t.Fatalf("新密码登录失败: %d", newLogin.StatusCode)
	}
	for _, c := range newLogin.Cookies {
		if c.Name == sessionCookie {
			staff = c
		}
	}
	staffAuth = []*http.Cookie{staff}
	if resp := doJSON(t, mux, "PATCH", "/admin/api/users/"+invitation.PublicID, `{"display_name":"Viewer","role_id":"`+viewerRole+`","disabled":true}`, ownerAuth, ownerCSRF); resp.StatusCode != 200 {
		t.Fatalf("停用员工: %d %s", resp.StatusCode, resp.Body)
	}
	if resp := doJSON(t, mux, "GET", "/admin/api/me", "", staffAuth, ""); resp.StatusCode != 401 {
		t.Fatalf("停用后旧会话应失效: %d", resp.StatusCode)
	}
	if resp := doJSON(t, mux, "POST", "/admin/api/login", `{"username":"viewer1","password":"newstaffsecret"}`, nil, ""); resp.StatusCode != 401 {
		t.Fatalf("停用后不能登录: %d", resp.StatusCode)
	}
	if resp := doJSON(t, mux, "GET", "/admin/api/me", "", ownerAuth, ""); resp.StatusCode != 200 {
		t.Fatalf("店主会话不受影响: %d", resp.StatusCode)
	}
}

func TestCustomRoleUpdateRevokesStaffSessions(t *testing.T) {
	_, mux := newHTTP(t)
	owner, csrf := loginAndCookies(t, mux)
	auth := []*http.Cookie{owner}
	role := doJSON(t, mux, "POST", "/admin/api/roles", `{"name":"查看商品","permissions":["products.view","products.create"]}`, auth, csrf)
	if role.StatusCode != 201 {
		t.Fatalf("建角色: %d %s", role.StatusCode, role.Body)
	}
	var ids struct {
		PublicID string `json:"public_id"`
	}
	_ = json.Unmarshal(role.Body, &ids)
	user := doJSON(t, mux, "POST", "/admin/api/users", `{"username":"staff2","display_name":"Staff","role_id":"`+ids.PublicID+`"}`, auth, csrf)
	if user.StatusCode != 201 {
		t.Fatalf("建员工: %d %s", user.StatusCode, user.Body)
	}
	var invite struct {
		InvitePath string `json:"invite_path"`
	}
	_ = json.Unmarshal(user.Body, &invite)
	u, _ := url.Parse(invite.InvitePath)
	f, _ := url.Parse(u.Fragment)
	if resp := doJSON(t, mux, "POST", "/admin/api/invites/accept", `{"token":"`+f.Query().Get("token")+`","password":"staffsecret"}`, nil, ""); resp.StatusCode != 200 {
		t.Fatalf("接受邀请: %d", resp.StatusCode)
	}
	login := doJSON(t, mux, "POST", "/admin/api/login", `{"username":"staff2","password":"staffsecret"}`, nil, "")
	var staff *http.Cookie
	var staffCSRF string
	for _, c := range login.Cookies {
		if c.Name == sessionCookie {
			staff = c
		}
		if c.Name == csrfCookie {
			staffCSRF = c.Value
		}
	}
	if staff == nil {
		t.Fatalf("员工登录失败: %d", login.StatusCode)
	}
	if resp := doJSON(t, mux, "GET", "/admin/api/products", "", []*http.Cookie{staff}, ""); resp.StatusCode != 200 {
		t.Fatalf("角色授权失败: %d", resp.StatusCode)
	}
	if resp := doJSON(t, mux, "POST", "/admin/api/products", `{"status":"active"}`, []*http.Cookie{staff}, staffCSRF); resp.StatusCode != 403 {
		t.Fatalf("无上架权限时不得直接创建上架商品: %d", resp.StatusCode)
	}
	if resp := doJSON(t, mux, "PATCH", "/admin/api/roles/"+ids.PublicID, `{"name":"查看概览","permissions":["dashboard.view"]}`, auth, csrf); resp.StatusCode != 200 {
		t.Fatalf("改角色: %d %s", resp.StatusCode, resp.Body)
	}
	if resp := doJSON(t, mux, "GET", "/admin/api/me", "", []*http.Cookie{staff}, ""); resp.StatusCode != 401 {
		t.Fatalf("角色更新后应撤销会话: %d", resp.StatusCode)
	}
}
