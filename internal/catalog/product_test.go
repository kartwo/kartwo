// 商品 CRUD 测试 / Product CRUD Tests
// 功能：建/取/改/软删商品、变体校验、库存、分类（核心逻辑必须单测）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-18 00:00:00
package catalog

import (
	"context"
	"errors"
	"testing"
)

func tee() ProductInput {
	return ProductInput{
		Title: "T恤", Slug: "tee", Status: "active",
		Options: []OptionInput{
			{Name: "尺码", Values: []string{"S", "M"}},
			{Name: "颜色", Values: []string{"黑", "白"}},
		},
		Variants: []VariantInput{
			{PriceCents: 9900, Quantity: 10, Selections: []Selection{{"尺码", "S"}, {"颜色", "黑"}}},
			{PriceCents: 9900, Quantity: 20, Selections: []Selection{{"尺码", "M"}, {"颜色", "白"}}},
		},
	}
}

func TestCreateAndGetProduct(t *testing.T) {
	svc := New(newDB(t))
	ctx := context.Background()

	pid, err := svc.CreateProduct(ctx, tee())
	if err != nil {
		t.Fatalf("建商品失败: %v", err)
	}
	d, err := svc.GetProduct(ctx, pid)
	if err != nil {
		t.Fatalf("取商品失败: %v", err)
	}
	if len(d.Variants) != 2 {
		t.Fatalf("变体数 = %d，期望 2", len(d.Variants))
	}
	for _, v := range d.Variants {
		if len(v.Options) != 2 {
			t.Fatalf("变体应有 2 个轴取值，得 %d", len(v.Options))
		}
	}
}

func TestCreateProductValidation(t *testing.T) {
	svc := New(newDB(t))
	ctx := context.Background()

	cases := map[string]func(ProductInput) ProductInput{
		"空标题": func(p ProductInput) ProductInput { p.Title = ""; return p },
		"无轴":  func(p ProductInput) ProductInput { p.Options = nil; return p },
		"无变体": func(p ProductInput) ProductInput { p.Variants = nil; return p },
		"变体缺一个轴": func(p ProductInput) ProductInput {
			p.Variants[0].Selections = []Selection{{"尺码", "S"}}
			return p
		},
		"变体取值非法": func(p ProductInput) ProductInput {
			p.Variants[0].Selections = []Selection{{"尺码", "XL"}, {"颜色", "黑"}}
			return p
		},
		"负价格": func(p ProductInput) ProductInput { p.Variants[0].PriceCents = -1; return p },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			var ve *ValidationError
			if _, err := svc.CreateProduct(ctx, mut(tee())); !errors.As(err, &ve) {
				t.Fatalf("应返回 ValidationError，得到: %v", err)
			}
		})
	}
}

func TestCreateProduct_DuplicateCombo(t *testing.T) {
	svc := New(newDB(t))
	ctx := context.Background()
	in := tee()
	// 两个变体相同组合 → 唯一约束。
	in.Variants = []VariantInput{
		{PriceCents: 100, Quantity: 1, Selections: []Selection{{"尺码", "S"}, {"颜色", "黑"}}},
		{PriceCents: 200, Quantity: 2, Selections: []Selection{{"尺码", "S"}, {"颜色", "黑"}}},
	}
	var ve *ValidationError
	if _, err := svc.CreateProduct(ctx, in); !errors.As(err, &ve) {
		t.Fatalf("重复组合应被拒为校验错误，得到: %v", err)
	}
}

func TestCreateProduct_DuplicateSlug(t *testing.T) {
	svc := New(newDB(t))
	ctx := context.Background()
	if _, err := svc.CreateProduct(ctx, tee()); err != nil {
		t.Fatalf("首个失败: %v", err)
	}
	var ve *ValidationError
	if _, err := svc.CreateProduct(ctx, tee()); !errors.As(err, &ve) {
		t.Fatalf("重复 slug 应被拒，得到: %v", err)
	}
}

func TestUpdateAndDeleteProduct(t *testing.T) {
	svc := New(newDB(t))
	ctx := context.Background()
	pid, _ := svc.CreateProduct(ctx, tee())

	if err := svc.UpdateProduct(ctx, pid, "新标题", "desc", "archived"); err != nil {
		t.Fatalf("改商品失败: %v", err)
	}
	d, _ := svc.GetProduct(ctx, pid)
	if d.Title != "新标题" || d.Status != "archived" {
		t.Fatalf("更新未生效: %+v", d)
	}

	if err := svc.UpdateProduct(ctx, "nope", "x", "", "draft"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("改不存在商品应 ErrNotFound，得到: %v", err)
	}

	if err := svc.DeleteProduct(ctx, pid); err != nil {
		t.Fatalf("软删失败: %v", err)
	}
	if _, err := svc.GetProduct(ctx, pid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("软删后取应 ErrNotFound，得到: %v", err)
	}
	list, _ := svc.ListProducts(ctx)
	if len(list) != 0 {
		t.Fatalf("软删后列表应为空，得 %d", len(list))
	}
}

func TestSetVariantInventory(t *testing.T) {
	svc := New(newDB(t))
	ctx := context.Background()
	pid, _ := svc.CreateProduct(ctx, tee())
	d, _ := svc.GetProduct(ctx, pid)
	target := d.Variants[0]

	if err := svc.SetVariantInventory(ctx, target.PublicID, 555); err != nil {
		t.Fatalf("改库存失败: %v", err)
	}
	d2, _ := svc.GetProduct(ctx, pid)
	var found bool
	for _, v := range d2.Variants {
		if v.PublicID == target.PublicID {
			found = true
			if v.Quantity != 555 {
				t.Fatalf("库存 = %d，期望 555", v.Quantity)
			}
		}
	}
	if !found {
		t.Fatal("未找到目标变体")
	}

	if err := svc.SetVariantInventory(ctx, "nope", 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不存在变体应 ErrNotFound，得到: %v", err)
	}
	var ve *ValidationError
	if err := svc.SetVariantInventory(ctx, target.PublicID, -1); !errors.As(err, &ve) {
		t.Fatalf("负库存应校验错误，得到: %v", err)
	}
}

func TestSetVariantPrice(t *testing.T) {
	svc := New(newDB(t))
	ctx := context.Background()
	pid, _ := svc.CreateProduct(ctx, tee())
	d, _ := svc.GetProduct(ctx, pid)
	target := d.Variants[0]

	priceOf := func(pubID string) int64 {
		dd, _ := svc.GetProduct(ctx, pid)
		for _, v := range dd.Variants {
			if v.PublicID == pubID {
				return v.PriceCents
			}
		}
		t.Fatalf("未找到变体 %s", pubID)
		return -1
	}

	// 幸福路径：改成正数。
	if err := svc.SetVariantPrice(ctx, target.PublicID, 12345); err != nil {
		t.Fatalf("改价失败: %v", err)
	}
	if got := priceOf(target.PublicID); got != 12345 {
		t.Fatalf("价格 = %d，期望 12345", got)
	}

	// 允许 0（免费/赠品）。
	if err := svc.SetVariantPrice(ctx, target.PublicID, 0); err != nil {
		t.Fatalf("0 价应允许，得到: %v", err)
	}
	if got := priceOf(target.PublicID); got != 0 {
		t.Fatalf("价格 = %d，期望 0", got)
	}

	// 负数拒绝（校验错误）。
	var ve *ValidationError
	if err := svc.SetVariantPrice(ctx, target.PublicID, -1); !errors.As(err, &ve) {
		t.Fatalf("负价应校验错误，得到: %v", err)
	}

	// 变体不存在拒绝。
	if err := svc.SetVariantPrice(ctx, "nope", 100); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不存在变体应 ErrNotFound，得到: %v", err)
	}
}

func TestCategoriesAndLink(t *testing.T) {
	svc := New(newDB(t))
	ctx := context.Background()
	cpid, err := svc.CreateCategory(ctx, "服装", "apparel")
	if err != nil {
		t.Fatalf("建分类失败: %v", err)
	}
	cats, _ := svc.ListCategories(ctx)
	if len(cats) != 1 || cats[0].PublicID != cpid {
		t.Fatalf("分类列表异常: %+v", cats)
	}

	in := tee()
	in.CategoryPublicIDs = []string{cpid}
	if _, err := svc.CreateProduct(ctx, in); err != nil {
		t.Fatalf("带分类建商品失败: %v", err)
	}

	// 关联不存在分类应校验错误。
	in2 := tee()
	in2.Slug = "tee2"
	in2.CategoryPublicIDs = []string{"nope"}
	var ve *ValidationError
	if _, err := svc.CreateProduct(ctx, in2); !errors.As(err, &ve) {
		t.Fatalf("关联不存在分类应被拒，得到: %v", err)
	}
}
