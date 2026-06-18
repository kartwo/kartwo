// 商品目录测试 / Catalog Tests
// 功能：验证演示装载、变体矩阵、组合唯一约束、库存（核心逻辑必须单测）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 18:13:43
package catalog

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kartwo/kartwo/internal/migrate"
	"github.com/kartwo/kartwo/internal/store/sqlcgen"
	"github.com/kartwo/kartwo/migrations"

	_ "modernc.org/sqlite"
)

func newDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/t.db?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("打开测试库失败: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := migrate.Run(context.Background(), db, migrations.FS); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return db
}

func TestSeedDemo_CreatesSixVariantMatrix(t *testing.T) {
	svc := New(newDB(t))
	ctx := context.Background()

	pid, created, err := svc.SeedDemo(ctx)
	if err != nil {
		t.Fatalf("SeedDemo 失败: %v", err)
	}
	if !created {
		t.Fatal("首次 SeedDemo 应创建（created=true）")
	}

	matrix, err := svc.GetVariantMatrix(ctx, pid)
	if err != nil {
		t.Fatalf("取矩阵失败: %v", err)
	}
	if len(matrix) != 6 {
		t.Fatalf("变体数 = %d，期望 6（尺码3×颜色2）", len(matrix))
	}

	// 每个变体应有两个轴取值、库存 100、金额 9900 分、唯一组合。
	seen := map[string]bool{}
	for _, v := range matrix {
		if len(v.Options) != 2 {
			t.Fatalf("变体 %s 轴数 = %d，期望 2", v.SKU, len(v.Options))
		}
		if v.Quantity != 100 {
			t.Fatalf("变体 %s 库存 = %d，期望 100", v.SKU, v.Quantity)
		}
		if v.PriceCents != 9900 {
			t.Fatalf("变体 %s 金额 = %d 分，期望 9900", v.SKU, v.PriceCents)
		}
		key := v.Options[0].Value + "|" + v.Options[1].Value
		if seen[key] {
			t.Fatalf("出现重复组合: %s", key)
		}
		seen[key] = true
	}
	if len(seen) != 6 {
		t.Fatalf("去重组合数 = %d，期望 6", len(seen))
	}
}

func TestSeedDemo_Idempotent(t *testing.T) {
	svc := New(newDB(t))
	ctx := context.Background()

	pid1, created1, err := svc.SeedDemo(ctx)
	if err != nil || !created1 {
		t.Fatalf("首次 SeedDemo 失败: created=%v err=%v", created1, err)
	}
	pid2, created2, err := svc.SeedDemo(ctx)
	if err != nil {
		t.Fatalf("二次 SeedDemo 失败: %v", err)
	}
	if created2 {
		t.Fatal("二次 SeedDemo 应跳过（created=false）")
	}
	if pid1 != pid2 {
		t.Fatalf("两次返回 product_id 不一致: %d vs %d", pid1, pid2)
	}

	matrix, _ := svc.GetVariantMatrix(ctx, pid2)
	if len(matrix) != 6 {
		t.Fatalf("幂等后变体数 = %d，期望仍为 6（无重复装入）", len(matrix))
	}
}

// 变体组合唯一约束：同一商品下相同选项值组合的第二个变体应被拒绝。
func TestVariantComboUniqueness(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	q := sqlcgen.New(db)

	pid, err := q.CreateProduct(ctx, sqlcgen.CreateProductParams{
		PublicID: uuid.Must(uuid.NewV7()).String(), Title: "T", Slug: "t", Status: "draft",
	})
	if err != nil {
		t.Fatalf("建商品失败: %v", err)
	}
	optID, _ := q.CreateOption(ctx, sqlcgen.CreateOptionParams{ProductID: pid, Name: "尺码"})
	valID, _ := q.CreateOptionValue(ctx, sqlcgen.CreateOptionValueParams{OptionID: optID, Value: "S"})
	key := optionKey([]int64{valID})

	mk := func() error {
		_, err := q.CreateVariant(ctx, sqlcgen.CreateVariantParams{
			PublicID: uuid.Must(uuid.NewV7()).String(), ProductID: pid, PriceCents: 100, OptionKey: key,
		})
		return err
	}
	if err := mk(); err != nil {
		t.Fatalf("首个变体应成功: %v", err)
	}
	if err := mk(); err == nil {
		t.Fatal("相同组合第二个变体应被唯一约束拒绝，却成功了")
	}
}

func TestOptionKey_OrderIndependent(t *testing.T) {
	if optionKey([]int64{3, 1, 2}) != optionKey([]int64{1, 2, 3}) {
		t.Fatal("option_key 应与入参顺序无关（规范化排序）")
	}
	if optionKey([]int64{1, 2}) == optionKey([]int64{1, 3}) {
		t.Fatal("不同组合的 option_key 不应相等")
	}
}

func TestGetInventory_AbsentIsZero(t *testing.T) {
	db := newDB(t)
	q := sqlcgen.New(db)
	_, err := q.GetInventory(context.Background(), 99999)
	if err == nil || !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("缺库存行应返回 ErrNoRows，得到: %v", err)
	}
}
