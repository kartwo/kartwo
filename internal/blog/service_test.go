// 博客服务测试 / Blog Service Tests
// 功能：覆盖默认开关、草稿发布、撤回、软删与文章链接约束
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-13 10:00:00
package blog

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/settings"
	"github.com/kartwo/kartwo/migrations"
	_ "modernc.org/sqlite"
)

func testService(t *testing.T) *Service {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/blog.db?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := migrate.Run(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	return New(db, settings.New(db))
}

func TestPostLifecycleAndDefaultOn(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	if on, err := s.Enabled(ctx); err != nil || !on {
		t.Fatalf("默认应开启: %v %v", on, err)
	}
	p := Post{Title: "First story", Slug: "first-story", Excerpt: "Intro", BodyMarkdown: "## Run", Status: "draft"}
	id, err := s.Create(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPublished(ctx, p.Slug); !errors.Is(err, ErrNotFound) {
		t.Fatalf("草稿公开: %v", err)
	}
	p.Status = "active"
	if err := s.Update(ctx, id, p); err != nil {
		t.Fatal(err)
	}
	pub, err := s.GetPublished(ctx, p.Slug)
	if err != nil || pub.PublishedAt == "" {
		t.Fatalf("发布失败: %+v %v", pub, err)
	}
	posts, total, err := s.ListPublished(ctx, 1)
	if err != nil || total != 1 || len(posts) != 1 {
		t.Fatalf("列表不正确: %d %d %v", total, len(posts), err)
	}
	if err := s.SetEnabled(ctx, false); err != nil {
		t.Fatal(err)
	}
	if on, err := s.Enabled(ctx); err != nil || on {
		t.Fatalf("关闭失败: %v %v", on, err)
	}
	if _, err := s.Get(ctx, id); err != nil {
		t.Fatalf("关闭不应删除文章: %v", err)
	}
	p.Status = "draft"
	if err := s.Update(ctx, id, p); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPublished(ctx, p.Slug); !errors.Is(err, ErrNotFound) {
		t.Fatalf("撤回后仍公开: %v", err)
	}
	if err := s.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("软删后仍存在: %v", err)
	}
}

func TestPostValidationAndUniqueSlug(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	p := Post{Title: "Title", Slug: "safe-link", BodyMarkdown: "body", Status: "active"}
	if _, err := s.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, p); !errors.Is(err, ErrConflict) {
		t.Fatalf("重复链接应拒绝: %v", err)
	}
	p.Slug = "../unsafe"
	if _, err := s.Create(ctx, p); !errors.Is(err, ErrInvalid) {
		t.Fatalf("非法链接应拒绝: %v", err)
	}
	p.Slug = "another"
	p.CoverPath = "../../private"
	if _, err := s.Create(ctx, p); !errors.Is(err, ErrInvalid) {
		t.Fatalf("封面路径应拒绝: %v", err)
	}
	p.CoverPath = ""
	p.BodyMarkdown = " "
	if _, err := s.Create(ctx, p); !errors.Is(err, ErrInvalid) {
		t.Fatalf("空正文不能发布: %v", err)
	}
}

func TestMarkdownEscapesScriptsAndRestrictsImages(t *testing.T) {
	html := string(RenderMarkdown("<script>alert(1)</script>\n[bad](javascript:alert) [safe](/p/shoe)\n![x](/media/blog/" + strings.Repeat("a", 64) + ".webp)\n![evil](https://evil.test/a.png)"))
	if strings.Contains(html, "<script>") || strings.Contains(html, "href=\"javascript:") || strings.Contains(html, "<img src=\"https://evil") {
		t.Fatalf("不安全内容被渲染: %s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;") || !strings.Contains(html, "href=\"/p/shoe\"") || !strings.Contains(html, "<img src=\"/media/blog/") {
		t.Fatalf("安全内容缺失: %s", html)
	}
}

func TestRichTextSanitizedBeforeSaveAndOnRender(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	image := "/media/blog/" + strings.Repeat("a", 64) + ".webp"
	p := Post{Title: "Rich", Slug: "rich", Status: "active", BodyHTML: `<h2 onclick="bad()">Title</h2><p><b>Bold</b> <a href="javascript:alert(1)">bad link</a> <a href="https://example.com/story" target="_blank">good link</a></p><script>alert(1)</script><p><img src="` + image + `" onerror="bad()"><img src="https://evil.example/x.png"></p>`}
	id, err := s.Create(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.GetPublished(ctx, "rich")
	if err != nil || saved.PublicID != id {
		t.Fatalf("富文本未发布: %+v %v", saved, err)
	}
	for _, unsafe := range []string{"onclick", "onerror", "javascript:", "<script", "alert(1)", "evil.example", "target="} {
		if strings.Contains(saved.BodyHTML, unsafe) {
			t.Fatalf("保存了不安全内容 %q: %s", unsafe, saved.BodyHTML)
		}
	}
	for _, safe := range []string{"<h2>Title</h2>", "<strong>Bold</strong>", `href="https://example.com/story"`, `src="` + image + `"`} {
		if !strings.Contains(saved.BodyHTML, safe) {
			t.Fatalf("安全排版丢失 %q: %s", safe, saved.BodyHTML)
		}
	}
	if rendered := string(RenderHTML(`<p>Hello</p><img src="https://evil.example/x.png"><svg onload="bad()"></svg>`)); strings.Contains(rendered, "evil.example") || strings.Contains(rendered, "svg") {
		t.Fatalf("读取时未二次过滤: %s", rendered)
	}
	p.Slug = "empty-rich"
	p.BodyHTML = `<script>alert(1)</script><p><br></p>`
	if _, err := s.Create(ctx, p); !errors.Is(err, ErrInvalid) {
		t.Fatalf("仅有危险代码或空段落不能发布: %v", err)
	}
}

func TestRichTextToolbarFormatsAndVideoAllowlist(t *testing.T) {
	source := `<blockquote>Quote</blockquote><p style="text-align:center;line-height:1.8;position:absolute"><font face="serif" size="5" color="rgb(12, 34, 56)">Styled</font><span style="background-color:#fff1b8; background-image:url(javascript:bad)">mark</span></p><table onclick="bad()"><tbody><tr><td>Cell</td></tr></tbody></table><p><iframe src="https://www.youtube-nocookie.com/embed/M7lc1UVf-VE" onload="bad()"></iframe><iframe src="https://evil.example/embed/M7lc1UVf-VE"></iframe></p>`
	safe, visible := SanitizeHTML(source)
	if !visible {
		t.Fatal("工具栏内容应可发布")
	}
	for _, want := range []string{`<blockquote>Quote</blockquote>`, `text-align:center`, `line-height:1.8`, `font-family:serif`, `font-size:24px`, `color:#0c2238`, `background-color:#fff1b8`, `<table>`, `youtube-nocookie.com/embed/M7lc1UVf-VE`} {
		if !strings.Contains(safe, want) {
			t.Fatalf("排版或视频丢失 %q: %s", want, safe)
		}
	}
	for _, unsafe := range []string{"position:", "background-image", "javascript:", "onclick", "onload", "evil.example"} {
		if strings.Contains(safe, unsafe) {
			t.Fatalf("危险属性未过滤 %q: %s", unsafe, safe)
		}
	}
	if safeVideoEmbed("https://www.youtube-nocookie.com/embed/M7lc1UVf-VE?autoplay=1") || safeVideoEmbed("https://player.vimeo.com/video/123#x") {
		t.Fatal("视频地址不应保留任意参数或片段")
	}
	if !safeVideoEmbed("https://player.vimeo.com/video/123456") {
		t.Fatal("Vimeo 播放器地址应可用")
	}
}

// Existing stored HTML is sanitized again on read, so old videos also gain the required policy.
func TestVideoReferrerPolicyIsProviderScoped(t *testing.T) {
	for _, tc := range []struct{ name, src, policy string }{
		{"youtube", "https://www.youtube-nocookie.com/embed/M7lc1UVf-VE", "strict-origin"},
		{"youtube uppercase scheme", "HTTPS://www.youtube-nocookie.com/embed/M7lc1UVf-VE", "strict-origin"},
		{"youtube escaped path", "https://www.youtube-nocookie.com/em%62ed/M7lc1UVf-VE", "strict-origin"},
		{"vimeo", "https://player.vimeo.com/video/123456", "no-referrer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, supplied := range []string{"", ` referrerpolicy="no-referrer"`, ` referrerpolicy="unsafe-url"`} {
				source := `<iframe src="` + tc.src + `"` + supplied + ` onload="bad()"></iframe>`
				want := `<iframe src="` + tc.src + `" title="Video" loading="lazy" referrerpolicy="` + tc.policy + `" allowfullscreen></iframe>`
				clean, visible := SanitizeHTML(source)
				if !visible || clean != want || string(RenderHTML(source)) != want {
					t.Fatalf("provider policy not applied on write and read: %s", clean)
				}
			}
		})
	}
	for _, src := range []string{"https://www.youtube-nocookie.com.evil.invalid/embed/M7lc1UVf-VE", "https://www.youtube-nocookie.com/embed/M7lc1UVf-VE?x=1", "http://www.youtube-nocookie.com/embed/M7lc1UVf-VE"} {
		if clean, visible := SanitizeHTML(`<iframe src="` + src + `" referrerpolicy="strict-origin"></iframe>`); visible || clean != "" {
			t.Fatal("referrer policy must not expand the player allowlist")
		}
	}
}
