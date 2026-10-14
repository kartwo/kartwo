// 博客后台接口测试 / Blog Admin API Tests
// 功能：验证鉴权、CSRF、严格字段、发布和开关
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-13 10:00:00
package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestBlogAdminAPI(t *testing.T) {
	_, mux := newHTTPEnvDomain(t, "")
	if got := doJSON(t, mux, "GET", "/admin/api/blog/posts", "", nil, ""); got.StatusCode != 401 {
		t.Fatalf("未登录应拒绝: %d", got.StatusCode)
	}
	sess, csrf := loginAndCookies(t, mux)
	cookies := []*http.Cookie{sess}
	if got := doJSON(t, mux, "GET", "/admin/api/settings/blog", "", cookies, ""); got.StatusCode != 200 || !strings.Contains(string(got.Body), `"enabled":true`) {
		t.Fatalf("博客应默认开启: %d %s", got.StatusCode, got.Body)
	}
	body := `{"title":"News","slug":"news","excerpt":"Summary","body_markdown":"## Hello","cover_path":"","seo_description":"","status":"active"}`
	if got := doJSON(t, mux, "POST", "/admin/api/blog/posts", body, cookies, ""); got.StatusCode != 403 {
		t.Fatalf("缺 CSRF 应拒绝: %d", got.StatusCode)
	}
	if got := doJSON(t, mux, "POST", "/admin/api/blog/posts", strings.TrimSuffix(body, "}")+`,"public_id":"forged"}`, cookies, csrf); got.StatusCode != 400 {
		t.Fatalf("只读字段应拒绝: %d %s", got.StatusCode, got.Body)
	}
	if got := doJSON(t, mux, "POST", "/admin/api/blog/posts", body, cookies, csrf); got.StatusCode != 201 {
		t.Fatalf("发布失败: %d %s", got.StatusCode, got.Body)
	}
	rich := doJSON(t, mux, "POST", "/admin/api/blog/posts", `{"title":"Rich","slug":"rich","body_html":"<p onclick=\"bad()\">Hello <strong>world</strong></p>","status":"active"}`, cookies, csrf)
	if rich.StatusCode != 201 {
		t.Fatalf("富文本发布失败: %d %s", rich.StatusCode, rich.Body)
	}
	var created struct {
		PublicID string `json:"public_id"`
	}
	if err := json.Unmarshal(rich.Body, &created); err != nil {
		t.Fatal(err)
	}
	gotRich := doJSON(t, mux, "GET", "/admin/api/blog/posts/"+created.PublicID, "", cookies, "")
	var saved struct {
		BodyHTML string `json:"body_html"`
	}
	if err := json.Unmarshal(gotRich.Body, &saved); err != nil {
		t.Fatal(err)
	}
	if gotRich.StatusCode != 200 || saved.BodyHTML != "<p>Hello <strong>world</strong></p>" {
		t.Fatalf("富文本未安全保存: %d %s", gotRich.StatusCode, gotRich.Body)
	}
	if got := doJSON(t, mux, "PUT", "/admin/api/settings/blog", `{"enabled":false}`, cookies, csrf); got.StatusCode != 200 || !strings.Contains(string(got.Body), `"enabled":false`) {
		t.Fatalf("关闭失败: %d %s", got.StatusCode, got.Body)
	}
	if got := doJSON(t, mux, "GET", "/admin/api/blog/posts", "", cookies, ""); got.StatusCode != 200 || !strings.Contains(string(got.Body), `"title":"News"`) {
		t.Fatalf("关闭后后台文章应保留: %d %s", got.StatusCode, got.Body)
	}
}

func TestBlogRoleRequiresViewForManage(t *testing.T) {
	if _, _, err := cleanRoleInput("Editor", []string{"blog.manage"}); err == nil {
		t.Fatal("管理权限应要求查看权限")
	}
	if _, _, err := cleanRoleInput("Editor", []string{"blog.view", "blog.manage"}); err != nil {
		t.Fatal(err)
	}
}
