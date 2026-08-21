// CSV 导入服务测试 / CSV Import Service Tests
// 功能：验证干跑不落商品、行级错误、整批执行与成功任务重试幂等
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-21 12:23:42
package importer

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/kartwo/kartwo/internal/catalog"
	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/redirect"
	"github.com/kartwo/kartwo/migrations"
)

const sample = `title,slug,status,description,option1_name,option1_value,option2_name,option2_value,sku,price_cents,quantity
T恤,tee,active,纯棉,尺码,S,颜色,黑,TS-B-S,9900,3
T恤,tee,active,纯棉,尺码,M,颜色,白,TS-W-M,10900,5
帽子,cap,draft,,尺寸,均码,,,CAP-ONE,2900,8
`

const shopifySample = `Handle,Title,Body (HTML),Option1 Name,Option1 Value,Option2 Name,Option2 Value,Option3 Name,Option3 Value,Variant SKU,Variant Inventory Qty,Variant Price,Status,Image Src
shopify-tee,Shopify T恤,<p>纯棉</p>,颜色,黑色,,, , ,SHOPIFY-BLACK,10,29.99,active,
shopify-tee,,,颜色,白色,,, , ,SHOPIFY-WHITE,8,29.99,active,
`

func newService(t *testing.T) (*Service, *catalog.Service) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/import.db?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := migrate.Run(context.Background(), db, migrations.FS); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	cat := catalog.New(db)
	return New(db, cat, nil, redirect.New(db)), cat
}

func TestPreviewAndExecuteCSV(t *testing.T) {
	svc, cat := newService(t)
	ctx := context.Background()
	p, err := svc.PreviewCSV(ctx, strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != "previewed" || p.ProductCount != 2 || p.VariantCount != 3 || len(p.Errors) != 0 {
		t.Fatalf("预览异常: %+v", p)
	}
	if got, _ := cat.ListProducts(ctx); len(got) != 0 {
		t.Fatalf("干跑不应写商品: %+v", got)
	}
	done, err := svc.Execute(ctx, p.PublicID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != "succeeded" {
		t.Fatalf("状态=%s", done.Status)
	}
	if got, _ := cat.ListProducts(ctx); len(got) != 2 {
		t.Fatalf("执行后商品数=%d", len(got))
	}
	if _, err := svc.Execute(ctx, p.PublicID); err != nil {
		t.Fatalf("成功任务重试应幂等: %v", err)
	}
	if got, _ := cat.ListProducts(ctx); len(got) != 2 {
		t.Fatalf("重试后商品数=%d", len(got))
	}
}

func TestPreviewCSVRowError(t *testing.T) {
	svc, cat := newService(t)
	ctx := context.Background()
	bad := strings.Replace(sample, "TS-B-S,9900,3", "TS-B-S,nope,3", 1)
	p, err := svc.PreviewCSV(ctx, strings.NewReader(bad))
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != "rejected" || len(p.Errors) != 1 || p.Errors[0].Row != 2 {
		t.Fatalf("错误报告异常: %+v", p)
	}
	if _, err := svc.Execute(ctx, p.PublicID); err == nil {
		t.Fatal("含行错误任务不应执行")
	}
	if got, _ := cat.ListProducts(ctx); len(got) != 0 {
		t.Fatal("拒绝任务不应写商品")
	}
}

func TestPreviewAndExecuteShopifyCSV(t *testing.T) {
	svc, cat := newService(t)
	ctx := context.Background()
	p, err := svc.PreviewShopifyCSV(ctx, strings.NewReader(shopifySample))
	if err != nil {
		t.Fatal(err)
	}
	if p.Format != FormatShopify || p.Status != "previewed" || p.ProductCount != 1 || p.VariantCount != 2 || len(p.Errors) != 0 {
		t.Fatalf("Shopify 预览异常: %+v", p)
	}
	if _, err := svc.Execute(ctx, p.PublicID); err != nil {
		t.Fatal(err)
	}
	products, err := cat.ListProducts(ctx)
	if err != nil || len(products) != 1 || products[0].Slug != "shopify-tee" {
		t.Fatalf("Shopify 执行商品异常: %+v, %v", products, err)
	}
	if slug, err := svc.redirect.ResolveShopifyHandle(ctx, "shopify-tee"); err != nil || slug != "shopify-tee" {
		t.Fatalf("Shopify 导入应保存旧链接映射，slug=%q err=%v", slug, err)
	}
}

func TestPreviewShopifyCSVAcceptsPublicImageURL(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	withImage := strings.Replace(shopifySample, "SHOPIFY-WHITE,8,29.99,active,", "SHOPIFY-WHITE,8,29.99,active,https://cdn.example.test/tee.jpg", 1)
	p, err := svc.PreviewShopifyCSV(ctx, strings.NewReader(withImage))
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != "previewed" || len(p.Errors) != 0 || p.ProductCount != 1 || p.VariantCount != 2 {
		t.Fatalf("公开 HTTPS 图片应可预览: %+v", p)
	}
}

func TestPreviewShopifyCSVRejectsUnsafeImageURL(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()
	withImage := strings.Replace(shopifySample, "SHOPIFY-WHITE,8,29.99,active,", "SHOPIFY-WHITE,8,29.99,active,http://127.0.0.1/secret.png", 1)
	p, err := svc.PreviewShopifyCSV(ctx, strings.NewReader(withImage))
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != "rejected" || len(p.Errors) != 1 || p.Errors[0].Row != 3 || !strings.Contains(p.Errors[0].Message, "HTTPS") {
		t.Fatalf("不安全图片地址应被拒绝: %+v", p)
	}
}

func TestFormatHashKey(t *testing.T) {
	if got := formatHashKey(FormatShopify); got != "shopify-v2-images" {
		t.Fatalf("Shopify 解析版本=%q", got)
	}
	if got := formatHashKey(FormatGeneric); got != FormatGeneric {
		t.Fatalf("通用格式不应变更 hash key: %q", got)
	}
}
