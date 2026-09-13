// 跑步演示目录测试 / Running Demo Catalog Tests
// 功能：验证五分类二十商品、幂等补齐、已有内容保护及封面导入
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-09-12 21:57:14
package catalog

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/kartwo/kartwo/internal/media"
)

func TestSeedRunningDemoCreatesFiveCategoriesAndTwentyProductsIdempotently(t *testing.T) {
	db := newDB(t)
	svc := New(db)
	ctx := context.Background()

	first, err := svc.SeedRunningDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.CategoriesCreated != 5 || first.ProductsCreated != 20 || first.LinksCreated != 20 {
		t.Fatalf("首次装入结果异常: %+v", first)
	}
	categories, err := svc.ListCategories(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(categories) != 5 {
		t.Fatalf("分类数=%d，期望 5", len(categories))
	}
	for _, category := range categories {
		if category.ProductCount != 4 {
			t.Fatalf("分类 %s 商品数=%d，期望 4", category.Slug, category.ProductCount)
		}
	}
	products, err := svc.ListProducts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != 20 {
		t.Fatalf("商品数=%d，期望 20", len(products))
	}

	second, err := svc.SeedRunningDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second != (RunningDemoResult{}) {
		t.Fatalf("重复执行应无变化: %+v", second)
	}
}

func TestImportRunningDemoImagesImportsBySlugAndPreservesExisting(t *testing.T) {
	db := newDB(t)
	svc := New(db)
	ctx := context.Background()
	if _, err := svc.SeedRunningDemo(ctx); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	imageDir := filepath.Join(root, "assets", "generated-products")
	if err := os.MkdirAll(imageDir, 0o750); err != nil {
		t.Fatal(err)
	}
	product := runningDemoCatalog()[0].Products[0]
	f, err := os.Create(filepath.Join(imageDir, product.Slug+".png")) //nolint:gosec // 测试临时目录。
	if err != nil {
		t.Fatal(err)
	}
	cover := image.NewRGBA(image.Rect(0, 0, 200, 200))
	draw.Draw(cover, cover.Bounds(), image.NewUniform(color.RGBA{R: 36, G: 84, B: 128, A: 255}), image.Point{}, draw.Src)
	if err := png.Encode(f, cover); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	mediaRoot := filepath.Join(root, "media")
	mediaSvc := media.New(db, media.NewLocalBackend(mediaRoot), media.NewDefaultPolicy(mediaRoot, 10<<20, 0), 20)
	first, err := svc.ImportRunningDemoImages(ctx, imageDir, mediaSvc)
	if err != nil {
		t.Fatal(err)
	}
	if !first.DirectoryFound || first.Imported != 1 || first.Existing != 0 || first.Missing != 19 {
		t.Fatalf("首次导入结果异常: %+v", first)
	}
	row, err := svc.q.GetProductBySlug(ctx, product.Slug)
	if err != nil {
		t.Fatal(err)
	}
	assets, err := mediaSvc.ListByProduct(ctx, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0].AltText != product.Title || len(assets[0].Derivatives) == 0 {
		t.Fatalf("封面或派生图异常: %+v", assets)
	}

	second, err := svc.ImportRunningDemoImages(ctx, imageDir, mediaSvc)
	if err != nil {
		t.Fatal(err)
	}
	if second.Imported != 0 || second.Existing != 1 || second.Missing != 19 {
		t.Fatalf("重复导入应保留已有封面: %+v", second)
	}

	notFound, err := svc.ImportRunningDemoImages(ctx, filepath.Join(root, "not-found"), mediaSvc)
	if err != nil || notFound.DirectoryFound {
		t.Fatalf("不存在目录应安全跳过: result=%+v err=%v", notFound, err)
	}
}

func TestSeedRunningDemoPreservesExistingProduct(t *testing.T) {
	svc := New(newDB(t))
	ctx := context.Background()
	existing := runningDemoCatalog()[0].Products[0]
	existing.Title = "Merchant Custom Title"
	if _, err := svc.CreateProduct(ctx, existing); err != nil {
		t.Fatal(err)
	}
	result, err := svc.SeedRunningDemo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.ProductsCreated != 19 || result.LinksCreated != 20 {
		t.Fatalf("增量补齐异常: %+v", result)
	}
	products, err := svc.ListProducts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, product := range products {
		if product.Slug == existing.Slug && product.Title != "Merchant Custom Title" {
			t.Fatalf("已有商品被覆盖: %+v", product)
		}
	}
}
