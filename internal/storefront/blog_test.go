// 博客店面测试 / Blog Storefront Tests
// 功能：验证开关、草稿隔离、导航与站点地图
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-13 10:00:00
package storefront

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kartwo/kartwo/internal/blog"
	"github.com/kartwo/kartwo/internal/cart"
	"github.com/kartwo/kartwo/internal/order"
	"github.com/kartwo/kartwo/internal/redirect"
	"github.com/kartwo/kartwo/internal/settings"
)

func TestBlogRoutesSwitchAndSitemap(t *testing.T) {
	sf, _, db := setup(t)
	settingSvc := settings.New(db)
	b := blog.New(db, settingSvc)
	ctx := context.Background()
	draft := blog.Post{Title: "Draft", Slug: "draft", BodyMarkdown: "Hidden", Status: "draft"}
	if _, err := b.Create(ctx, draft); err != nil {
		t.Fatal(err)
	}
	pub := blog.Post{Title: "Published", Slug: "published", Excerpt: "Read me", BodyMarkdown: "## Hello", Status: "active"}
	if _, err := b.Create(ctx, pub); err != nil {
		t.Fatal(err)
	}
	richImage := "/media/blog/" + strings.Repeat("a", 64) + ".webp"
	rich := blog.Post{Title: "Rich", Slug: "rich", BodyHTML: `<h2>Visual title</h2><p><strong>Formatted</strong></p><p><img src="` + richImage + `" alt="photo"></p><p><iframe src="https://www.youtube-nocookie.com/embed/M7lc1UVf-VE"></iframe></p>`, Status: "active"}
	if _, err := b.Create(ctx, rich); err != nil {
		t.Fatal(err)
	}
	h := NewHTTP(sf, cart.New(db), order.New(db, settingSvc), settingSvc, nil, redirect.New(db), "Shop", "https://shop.test", nil)
	mux := http.NewServeMux()
	h.Register(mux)
	get := func(path string) (int, string) {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w.Code, w.Body.String()
	}
	if code, body := get("/blog"); code != 200 || !strings.Contains(body, "Published") || !strings.Contains(body, "<p>Visual title Formatted</p>") || !strings.Contains(body, `src="`+richImage+`"`) || strings.Contains(body, "Hidden") {
		t.Fatalf("列表不正确: %d %s", code, body)
	}
	if code, _ := get("/blog/draft"); code != 404 {
		t.Fatalf("草稿应 404: %d", code)
	}
	if code, body := get("/blog/published"); code != 200 || !strings.Contains(body, "Hello") {
		t.Fatalf("详情不正确: %d %s", code, body)
	}
	if code, body := get("/blog/rich"); code != 200 || !strings.Contains(body, "<h2>Visual title</h2>") || !strings.Contains(body, "<strong>Formatted</strong>") || !strings.Contains(body, "youtube-nocookie.com/embed/M7lc1UVf-VE") || !strings.Contains(body, `referrerpolicy="strict-origin"`) {
		t.Fatalf("富文本详情不正确: %d %s", code, body)
	}
	if _, body := get("/sitemap.xml"); !strings.Contains(body, "/blog/published") || strings.Contains(body, "/blog/draft") {
		t.Fatalf("站点地图不正确: %s", body)
	}
	if _, body := get("/"); !strings.Contains(body, `href="/blog"`) {
		t.Fatalf("导航未显示博客: %s", body)
	}
	if err := b.SetEnabled(ctx, false); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/blog", "/blog/published"} {
		if code, _ := get(path); code != 404 {
			t.Fatalf("关闭后 %s 应 404: %d", path, code)
		}
	}
	if _, body := get("/sitemap.xml"); strings.Contains(body, "/blog") {
		t.Fatalf("关闭后仍在 sitemap: %s", body)
	}
	if _, body := get("/"); strings.Contains(body, `href="/blog"`) {
		t.Fatalf("关闭后仍在导航: %s", body)
	}
}
