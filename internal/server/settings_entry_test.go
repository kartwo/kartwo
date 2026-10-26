// 内容与设置贯通测试 / Content and Settings Entry Tests
// 功能：验证实际店面发布和开关、细分员工授权、CSRF及持久配置，不调用外部服务
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-26 17:24:52
package server

import (
	"strings"
	"testing"
)

func TestContentAndSettingsPermissionPersistenceFlow(t *testing.T) {
	handler, st, _ := newAdminEntryFixture(t)
	owner, _ := dataEntryOwner(t, handler)
	anon := dataEntryClient{t: t, handler: handler}
	noCSRF := owner
	noCSRF.csrf = ""
	content := map[string]string{"title": "Entry delivery guide", "slug": "entry-delivery-guide", "body_markdown": "## Entry guidance\n\nSafe **delivery** text.\n\n<script>alert(1)</script>", "seo_description": "Delivery guide", "status": "draft"}
	var created struct {
		ID string `json:"public_id"`
	}
	owner.decode(owner.json("POST", "/admin/api/content-pages", content, 201), &created)
	path := "/admin/api/content-pages/" + created.ID
	anon.json("GET", path, nil, 401)
	anon.send("GET", "/pages/entry-delivery-guide", nil, "", 404)
	editor := catalogEntryStaff(t, owner, "entry-content-editor", []string{"content.manage", "blog.view", "contact.view", "settings.domain"})
	viewer := catalogEntryStaff(t, owner, "entry-blog-reader", []string{"blog.view"})
	content["title"], content["status"], content["slug"] = "Updated delivery guide", "active", "must-not-replace-existing-slug"
	noCSRF.json("PATCH", path, content, 403)
	viewer.json("PATCH", path, content, 403)
	editor.json("PATCH", path, content, 200)
	var read struct{ Title, Slug, Status string }
	editor.decode(editor.json("GET", path, nil, 200), &read)
	if read.Title != "Updated delivery guide" || read.Slug != "entry-delivery-guide" || read.Status != "active" {
		t.Fatal("saved content or immutable link mismatch")
	}
	public := anon.send("GET", "/pages/entry-delivery-guide", nil, "", 200).Body.String()
	if !strings.Contains(public, "Updated delivery guide") || !strings.Contains(public, "<h2>Entry guidance</h2>") || strings.Contains(public, "<script>alert(1)</script>") {
		t.Fatal("public content render did not preserve safe Markdown boundary")
	}
	owner.json("POST", "/admin/api/blog/posts", map[string]string{"title": "Entry running tips", "slug": "entry-running-tips", "body_html": "<p>Fictional running tips.</p>", "status": "active"}, 201)
	anon.send("GET", "/blog/entry-running-tips", nil, "", 200)
	for _, value := range []bool{false, true} {
		body := map[string]bool{"enabled": value}
		anon.json("PUT", "/admin/api/settings/blog", body, 401)
		noCSRF.json("PUT", "/admin/api/settings/blog", body, 403)
		editor.json("PUT", "/admin/api/settings/blog", body, 403)
		owner.json("PUT", "/admin/api/settings/blog", body, 200)
		var blog struct{ Enabled bool }
		viewer.decode(viewer.json("GET", "/admin/api/settings/blog", nil, 200), &blog)
		if blog.Enabled != value {
			t.Fatal("blog setting did not persist")
		}
		want := 404
		if value {
			want = 200
		}
		anon.send("GET", "/blog/entry-running-tips", nil, "", want)
	}
	contact := map[string]any{"enabled": true, "retention_months": 6, "ip_retention_days": 14}
	editor.json("GET", "/admin/api/settings/contact", nil, 200)
	editor.json("PUT", "/admin/api/settings/contact", contact, 403)
	owner.json("PUT", "/admin/api/settings/contact", contact, 200)
	owner.json("PUT", "/admin/api/settings/contact", map[string]any{"enabled": false, "retention_months": 0, "ip_retention_days": 14}, 400)
	var savedContact struct {
		Enabled bool
		Months  int `json:"retention_months"`
		Days    int `json:"ip_retention_days"`
	}
	editor.decode(editor.json("GET", "/admin/api/settings/contact", nil, 200), &savedContact)
	if !savedContact.Enabled || savedContact.Months != 6 || savedContact.Days != 14 {
		t.Fatal("rejected contact configuration changed saved retention")
	}
	viewer.json("PUT", "/admin/api/settings/domain", map[string]string{"domain": "native.example.invalid"}, 403)
	editor.json("PUT", "/admin/api/settings/domain", map[string]string{"domain": "https://native.example.invalid/path"}, 400)
	editor.json("PUT", "/admin/api/settings/domain", map[string]string{"domain": "native.example.invalid"}, 200)
	var domain struct {
		Domain, Source string
		HTTPS          bool `json:"https_capable"`
	}
	editor.decode(editor.json("GET", "/admin/api/settings/domain", nil, 200), &domain)
	if domain.Domain != "native.example.invalid" || domain.Source != "db" || domain.HTTPS {
		t.Fatal("local domain persistence/source mismatch")
	}
	editor.json("GET", "/admin/api/settings/backup", nil, 403)
	owner.json("PUT", "/admin/api/settings/backup", map[string]string{"interval": "48h", "retention": "5"}, 200)
	var policy struct {
		Interval  string
		Retention int
	}
	owner.decode(owner.json("GET", "/admin/api/settings/backup", nil, 200), &policy)
	if policy.Interval != "48h" || policy.Retention != 5 {
		t.Fatal("backup policy did not persist")
	}
	demo := anon.signed(anon.json("POST", "/admin/api/demo-session", nil, 201))
	demo.json("PATCH", path, content, 403)
	demo.json("PUT", "/admin/api/settings/blog", map[string]bool{"enabled": false}, 403)
	demo.json("PUT", "/admin/api/settings/contact", contact, 403)
	demo.json("PUT", "/admin/api/settings/domain", map[string]string{"domain": "forbidden.example.invalid"}, 403)
	demo.json("PUT", "/admin/api/settings/backup", map[string]string{"interval": "90m"}, 403)
	dataEntryCounts(t, st, 0, 0, 0)
}
