// 店铺政策资料服务测试 / Store Policy Profile Service Tests
// 功能：验证资料校验、持久化及内容页幂等生成和安全覆盖
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-12 15:30:00
package policy

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/kartwo/kartwo/internal/catalog"
	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/settings"
	"github.com/kartwo/kartwo/migrations"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/policy.db?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := migrate.Run(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSaveAndGenerateFooterPages(t *testing.T) {
	db := openTestDB(t)
	svc := New(settings.New(db), catalog.New(db))
	ctx := context.Background()
	p := KartwoDemoProfile()
	if err := svc.Save(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, configured, err := svc.Get(ctx)
	if err != nil || !configured || got.SupportEmail != "info@kartwo.com" {
		t.Fatalf("资料未保存: %+v %v %v", got, configured, err)
	}
	first, err := svc.Generate(ctx, true, false)
	if err != nil || first.Created != 7 {
		t.Fatalf("首次生成异常: %+v %v", first, err)
	}
	second, err := svc.Generate(ctx, true, false)
	if err != nil || second.Skipped != 7 {
		t.Fatalf("幂等生成异常: %+v %v", second, err)
	}
	pages, err := catalog.New(db).ListContentPages(ctx)
	if err != nil || len(pages) != 7 {
		t.Fatalf("页面数量异常: %d %v", len(pages), err)
	}
	for _, page := range pages {
		if page.Status != "active" {
			t.Fatalf("页面未发布: %+v", page)
		}
		if strings.HasPrefix(page.BodyMarkdown, "# ") {
			t.Fatalf("正文不应重复模板已有的一级标题: %+v", page)
		}
	}
}

func TestGeneratePreservesExistingStatusOnOverwrite(t *testing.T) {
	db := openTestDB(t)
	cat := catalog.New(db)
	svc := New(settings.New(db), cat)
	ctx := context.Background()
	if err := svc.Save(ctx, KartwoDemoProfile()); err != nil {
		t.Fatal(err)
	}
	id, err := cat.CreateContentPage(ctx, "Shipping", "shipping-policy", "hand edited", "seo", "draft")
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.Generate(ctx, true, true)
	if err != nil || result.Created != 6 || result.Updated != 1 {
		t.Fatalf("覆盖结果异常: %+v %v", result, err)
	}
	page, err := cat.GetContentPage(ctx, id)
	if err != nil || page.Status != "draft" || page.BodyMarkdown == "hand edited" {
		t.Fatalf("覆盖未保留状态: %+v %v", page, err)
	}
}

func TestRejectsInvalidProfile(t *testing.T) {
	db := openTestDB(t)
	svc := New(settings.New(db), catalog.New(db))
	p := KartwoDemoProfile()
	p.SupportEmail = "not-an-email"
	if err := svc.Save(context.Background(), p); err == nil {
		t.Fatal("应拒绝非法邮箱")
	}
}
