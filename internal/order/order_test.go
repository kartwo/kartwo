// 下单测试 / Order Tests
// 功能：结算成功/快照、缺货拦截、空车、非法信息、并发不超卖（核心可靠性）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-18 13:40:31
package order

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/kartwo/kartwo/internal/cart"
	"github.com/kartwo/kartwo/internal/catalog"
	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/migrations"

	_ "modernc.org/sqlite"
)

func setup(t *testing.T) (*sql.DB, *Service, *cart.Service, *catalog.Service) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/t.db?_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := migrate.Run(context.Background(), db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	return db, New(db, "CNY"), cart.New(db), catalog.New(db)
}

// seedVariant 建一个单变体商品，库存 stock，返回变体 public_id。
func seedVariant(t *testing.T, cat *catalog.Service, slug string, price, stock int64) string {
	t.Helper()
	ctx := context.Background()
	ppid, err := cat.CreateProduct(ctx, catalog.ProductInput{
		Title: "T恤", Slug: slug, Status: "active",
		Options:  []catalog.OptionInput{{Name: "尺码", Values: []string{"S"}}},
		Variants: []catalog.VariantInput{{PriceCents: price, Quantity: stock, Selections: []catalog.Selection{{Option: "尺码", Value: "S"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	d, err := cat.GetProduct(ctx, ppid)
	if err != nil {
		t.Fatal(err)
	}
	return d.Variants[0].PublicID
}

func info() CheckoutInfo {
	return CheckoutInfo{Email: "a@b.com", Name: "张三", Address: "测试地址 1 号", Country: "CN"}
}

func reservedOf(t *testing.T, db *sql.DB, slug string) (qty, reserved int64) {
	t.Helper()
	row := db.QueryRow(`SELECT i.quantity, i.reserved FROM inventory i JOIN variant v ON v.id=i.variant_id JOIN product p ON p.id=v.product_id WHERE p.slug=?`, slug)
	if err := row.Scan(&qty, &reserved); err != nil {
		t.Fatal(err)
	}
	return qty, reserved
}

func TestCheckout_Success(t *testing.T) {
	db, ord, crt, cat := setup(t)
	ctx := context.Background()
	v := seedVariant(t, cat, "tee", 9900, 5)
	cid, _, _ := crt.GetOrCreate(ctx, "")
	if err := crt.AddItem(ctx, cid, v, 2); err != nil {
		t.Fatal(err)
	}

	pid, err := ord.Checkout(ctx, cid, info())
	if err != nil {
		t.Fatalf("结算失败: %v", err)
	}
	o, err := ord.Get(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if o.TotalCents != 19800 || len(o.Lines) != 1 || o.Lines[0].Quantity != 2 {
		t.Fatalf("订单异常: total=%d lines=%d", o.TotalCents, len(o.Lines))
	}
	if o.Lines[0].Spec != "尺码:S" {
		t.Fatalf("规格快照 = %q", o.Lines[0].Spec)
	}
	// 库存预留 = 2。
	if _, res := reservedOf(t, db, "tee"); res != 2 {
		t.Fatalf("预留 = %d，期望 2", res)
	}
}

func TestCheckout_OutOfStock(t *testing.T) {
	db, ord, crt, cat := setup(t)
	ctx := context.Background()
	v := seedVariant(t, cat, "tee", 9900, 1)
	cid, _, _ := crt.GetOrCreate(ctx, "")
	_ = crt.AddItem(ctx, cid, v, 2) // 要 2，只有 1

	if _, err := ord.Checkout(ctx, cid, info()); !errors.Is(err, ErrOutOfStock) {
		t.Fatalf("应 ErrOutOfStock: %v", err)
	}
	// 回滚：未预留。
	if _, res := reservedOf(t, db, "tee"); res != 0 {
		t.Fatalf("失败后预留应为 0，得 %d", res)
	}
}

func TestCheckout_EmptyAndInvalid(t *testing.T) {
	_, ord, crt, _ := setup(t)
	ctx := context.Background()
	cid, _, _ := crt.GetOrCreate(ctx, "")
	if _, err := ord.Checkout(ctx, cid, info()); !errors.Is(err, ErrEmptyCart) {
		t.Fatalf("空车应 ErrEmptyCart: %v", err)
	}
	bad := info()
	bad.Email = "no-at"
	if _, err := ord.Checkout(ctx, cid, bad); !errors.Is(err, ErrInvalidInfo) {
		t.Fatalf("非法邮箱应 ErrInvalidInfo: %v", err)
	}
}

// 核心：并发抢同一库存不超卖。库存 K，N 个并发各下 1 件，应恰好 K 成功、N-K 缺货，预留==K。
func TestCheckout_ConcurrentNoOversell(t *testing.T) {
	db, ord, crt, cat := setup(t)
	ctx := context.Background()
	const stock, n = 5, 20
	v := seedVariant(t, cat, "tee", 1000, stock)

	// 每个并发请求各自一个购物车，各装 1 件。
	carts := make([]int64, n)
	for i := 0; i < n; i++ {
		cid, _, _ := crt.GetOrCreate(ctx, "")
		if err := crt.AddItem(ctx, cid, v, 1); err != nil {
			t.Fatal(err)
		}
		carts[i] = cid
	}

	var ok, oos int64
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(cid int64) {
			defer wg.Done()
			if _, err := ord.Checkout(context.Background(), cid, info()); err == nil {
				atomic.AddInt64(&ok, 1)
			} else if errors.Is(err, ErrOutOfStock) {
				atomic.AddInt64(&oos, 1)
			} else {
				t.Errorf("意外错误: %v", err)
			}
		}(carts[i])
	}
	wg.Wait()

	if ok != stock {
		t.Fatalf("成功下单 = %d，期望 = 库存 %d（不能超卖）", ok, stock)
	}
	if oos != n-stock {
		t.Fatalf("缺货 = %d，期望 %d", oos, n-stock)
	}
	qty, res := reservedOf(t, db, "tee")
	if res != stock || res > qty {
		t.Fatalf("预留 = %d（库存 %d），超卖防护失效", res, qty)
	}
}
