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
