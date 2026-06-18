// 购物车测试 / Cart Tests
// 功能：get-or-create、加/累加/改/删、合计与件数、缺货变体、非法数量
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-18 12:40:00
package cart

import (
	"context"
	"database/sql"
	"testing"

	"github.com/kartwo/kartwo/internal/catalog"
	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/migrations"

	_ "modernc.org/sqlite"
)

func setup(t *testing.T) (*Service, *catalog.Service) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/t.db?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := migrate.Run(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	return New(db), catalog.New(db)
}

// 建一个商品，返回其两个变体的 public_id（S=9900, M=12900）。
func seedProduct(t *testing.T, cat *catalog.Service) (vS, vM string) {
	t.Helper()
	ctx := context.Background()
	ppid, err := cat.CreateProduct(ctx, catalog.ProductInput{
		Title: "T恤", Slug: "tee", Status: "active",
		Options: []catalog.OptionInput{{Name: "尺码", Values: []string{"S", "M"}}},
		Variants: []catalog.VariantInput{
			{PriceCents: 9900, Quantity: 5, Selections: []catalog.Selection{{Option: "尺码", Value: "S"}}},
			{PriceCents: 12900, Quantity: 3, Selections: []catalog.Selection{{Option: "尺码", Value: "M"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	d, err := cat.GetProduct(ctx, ppid)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range d.Variants {
		if v.PriceCents == 9900 {
			vS = v.PublicID
		} else {
			vM = v.PublicID
		}
	}
	return vS, vM
}

func TestCart_AddIncrementSetRemove(t *testing.T) {
	svc, cat := setup(t)
	ctx := context.Background()
	vS, vM := seedProduct(t, cat)

	id, tok, err := svc.GetOrCreate(ctx, "")
	if err != nil || tok == "" {
		t.Fatalf("建购物车失败: %v", err)
	}

	// 同 token 再取应是同一车。
	id2, _, _ := svc.GetOrCreate(ctx, tok)
	if id2 != id {
		t.Fatalf("同 token 应取同车: %d vs %d", id, id2)
	}

	if err := svc.AddItem(ctx, id, vS, 2); err != nil {
		t.Fatal(err)
	}
	if err := svc.AddItem(ctx, id, vS, 3); err != nil { // 累加 → 5
		t.Fatal(err)
	}
	if err := svc.AddItem(ctx, id, vM, 1); err != nil {
		t.Fatal(err)
	}

	view, err := svc.View(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if view.Count != 6 {
		t.Fatalf("件数 = %d，期望 6", view.Count)
	}
	// 合计 = 5*9900 + 1*12900 = 62400
	if view.TotalCents != 5*9900+12900 {
		t.Fatalf("合计 = %d，期望 62400", view.TotalCents)
	}
	if len(view.Lines) != 2 {
		t.Fatalf("行数 = %d", len(view.Lines))
	}

	// 改量到 1。
	if err := svc.SetQty(ctx, id, vS, 1); err != nil {
		t.Fatal(err)
	}
	// 删除 M。
	if err := svc.SetQty(ctx, id, vM, 0); err != nil {
		t.Fatal(err)
	}
	view, _ = svc.View(ctx, id)
	if view.Count != 1 || len(view.Lines) != 1 || view.TotalCents != 9900 {
		t.Fatalf("改删后异常: count=%d lines=%d total=%d", view.Count, len(view.Lines), view.TotalCents)
	}

	if err := svc.RemoveItem(ctx, id, vS); err != nil {
		t.Fatal(err)
	}
	if n, _ := svc.Count(ctx, id); n != 0 {
		t.Fatalf("清空后件数 = %d", n)
	}
}

func TestCart_Errors(t *testing.T) {
	svc, cat := setup(t)
	ctx := context.Background()
	vS, _ := seedProduct(t, cat)
	id, _, _ := svc.GetOrCreate(ctx, "")

	if err := svc.AddItem(ctx, id, "nope", 1); err != ErrVariantNotFound {
		t.Fatalf("未知变体应 ErrVariantNotFound: %v", err)
	}
	if err := svc.AddItem(ctx, id, vS, 0); err != ErrInvalidQty {
		t.Fatalf("数量 0 应 ErrInvalidQty: %v", err)
	}
	if err := svc.AddItem(ctx, id, vS, 1000); err != ErrInvalidQty {
		t.Fatalf("数量超上限应 ErrInvalidQty: %v", err)
	}
}

func TestCart_OptionsAndThumb(t *testing.T) {
	svc, cat := setup(t)
	ctx := context.Background()
	vS, _ := seedProduct(t, cat)
	id, _, _ := svc.GetOrCreate(ctx, "")
	_ = svc.AddItem(ctx, id, vS, 1)

	view, _ := svc.View(ctx, id)
	if len(view.Lines) != 1 || len(view.Lines[0].Options) != 1 {
		t.Fatalf("应有 1 行 1 个选项: %+v", view.Lines)
	}
	if view.Lines[0].Options[0].Name != "尺码" || view.Lines[0].Options[0].Value != "S" {
		t.Fatalf("选项不对: %+v", view.Lines[0].Options)
	}
}
