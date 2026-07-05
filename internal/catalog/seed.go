// 演示数据装载 / Demo Data Seeder
// 功能：幂等装入一件"服装"商品演示通用双轴变体（尺码×颜色=6 变体），验证 M1.1 数据模型
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 18:13:43
package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

// demoProductSlug 为演示商品的稳定 slug，作幂等判定依据。
const demoProductSlug = "demo-tee"

// SeedDemo 幂等装入演示商品。已存在则直接返回其 id（created=false），不重复创建。
// 演示用"服装"举例，但数据模型为通用 option×option，不写死品类（见 DECISIONS）。
func (s *Service) SeedDemo(ctx context.Context) (productID int64, created bool, err error) {
	if p, e := s.q.GetProductBySlug(ctx, demoProductSlug); e == nil {
		return p.ID, false, nil
	} else if !errors.Is(e, sql.ErrNoRows) {
		return 0, false, fmt.Errorf("catalog: 查演示商品失败: %w", e)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, fmt.Errorf("catalog: 开启事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := s.q.WithTx(tx)

	pid, err := q.CreateProduct(ctx, sqlcgen.CreateProductParams{
		PublicID:    uuid.Must(uuid.NewV7()).String(),
		Title:       "Demo Tee",
		Slug:        demoProductSlug,
		Description: "A sample product demonstrating two-axis variants (Size × Color).",
		Status:      "active",
	})
	if err != nil {
		return 0, false, fmt.Errorf("catalog: 建商品失败: %w", err)
	}

	// 轴 1：Size；轴 2：Color。两轴笛卡尔积 = 6 个变体（演示数据用英文，店面默认英文）。
	sizeOptID, sizeValIDs, err := s.createAxis(ctx, q, pid, 0, "Size", []string{"S", "M", "L"})
	if err != nil {
		return 0, false, err
	}
	colorOptID, colorValIDs, err := s.createAxis(ctx, q, pid, 1, "Color", []string{"Black", "White"})
	if err != nil {
		return 0, false, err
	}

	pos := 0
	for _, sv := range sizeValIDs {
		for _, cv := range colorValIDs {
			vid, err := q.CreateVariant(ctx, sqlcgen.CreateVariantParams{
				PublicID:   uuid.Must(uuid.NewV7()).String(),
				ProductID:  pid,
				Sku:        sql.NullString{String: fmt.Sprintf("DEMO-TEE-%02d", pos+1), Valid: true},
				PriceCents: 9900, // 99.00，金额整数(分)
				OptionKey:  optionKey([]int64{sv, cv}),
				Position:   int64(pos),
			})
			if err != nil {
				return 0, false, fmt.Errorf("catalog: 建变体失败: %w", err)
			}
			for optID, valID := range map[int64]int64{sizeOptID: sv, colorOptID: cv} {
				if err := q.AddVariantOptionValue(ctx, sqlcgen.AddVariantOptionValueParams{
					VariantID: vid, OptionID: optID, ValueID: valID,
				}); err != nil {
					return 0, false, fmt.Errorf("catalog: 连接变体选项值失败: %w", err)
				}
			}
			if err := q.SetInventory(ctx, sqlcgen.SetInventoryParams{VariantID: vid, Quantity: 100}); err != nil {
				return 0, false, fmt.Errorf("catalog: 置库存失败: %w", err)
			}
			pos++
		}
	}

	// 分类：服装 / Apparel，并关联商品。
	cid, err := q.CreateCategory(ctx, sqlcgen.CreateCategoryParams{
		PublicID: uuid.Must(uuid.NewV7()).String(),
		Name:     "Apparel",
		Slug:     "apparel",
		ParentID: sql.NullInt64{},
		Position: 0,
	})
	if err != nil {
		return 0, false, fmt.Errorf("catalog: 建分类失败: %w", err)
	}
	if err := q.LinkProductCategory(ctx, sqlcgen.LinkProductCategoryParams{ProductID: pid, CategoryID: cid}); err != nil {
		return 0, false, fmt.Errorf("catalog: 关联分类失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, false, fmt.Errorf("catalog: 提交失败: %w", err)
	}
	return pid, true, nil
}

// createAxis 建一个变体轴及其取值，返回轴 id 与各取值 id（顺序与入参一致）。
func (s *Service) createAxis(ctx context.Context, q *sqlcgen.Queries, productID int64, pos int, name string, values []string) (optID int64, valIDs []int64, err error) {
	optID, err = q.CreateOption(ctx, sqlcgen.CreateOptionParams{ProductID: productID, Name: name, Position: int64(pos)})
	if err != nil {
		return 0, nil, fmt.Errorf("catalog: 建轴 %s 失败: %w", name, err)
	}
	valIDs = make([]int64, len(values))
	for i, v := range values {
		vid, err := q.CreateOptionValue(ctx, sqlcgen.CreateOptionValueParams{OptionID: optID, Value: v, Position: int64(i)})
		if err != nil {
			return 0, nil, fmt.Errorf("catalog: 建轴值 %s=%s 失败: %w", name, v, err)
		}
		valIDs[i] = vid
	}
	return optID, valIDs, nil
}
