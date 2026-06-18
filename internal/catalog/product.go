// 商品 CRUD 领域逻辑 / Product CRUD
// 功能：建/列/取/改/软删商品（含变体校验）、改库存、分类增列；通用双轴变体
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-18 00:00:00
package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

// ErrNotFound 表示目标资源不存在（或已软删）。
var ErrNotFound = errors.New("catalog: 资源不存在")

// ValidationError 为可回传给客户端的人话校验错误。
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func vErr(format string, a ...any) error { return &ValidationError{Msg: fmt.Sprintf(format, a...)} }

var validStatuses = map[string]bool{"draft": true, "active": true, "archived": true}

// ---- 入参类型 ----

type ProductInput struct {
	Title             string
	Slug              string
	Description       string
	Status            string
	Options           []OptionInput
	Variants          []VariantInput
	CategoryPublicIDs []string
}

type OptionInput struct {
	Name   string
	Values []string
}

type VariantInput struct {
	SKU        string
	PriceCents int64
	Quantity   int64
	Selections []Selection // 每个轴恰好一项
}

type Selection struct {
	Option string
	Value  string
}

// ---- 出参类型 ----

type ProductSummary struct {
	PublicID  string
	Title     string
	Slug      string
	Status    string
	UpdatedAt string
}

type ProductDetail struct {
	PublicID    string
	Title       string
	Slug        string
	Description string
	Status      string
	Variants    []VariantView
}

type CategorySummary struct {
	PublicID string
	Name     string
	Slug     string
}

// CreateProduct 在事务内建商品 + 变体轴 + 变体 + 库存 + 分类关联，返回新商品 public_id。
func (s *Service) CreateProduct(ctx context.Context, in ProductInput) (string, error) {
	if err := validateProductInput(in); err != nil {
		return "", err
	}
	status := in.Status
	if status == "" {
		status = "draft"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("catalog: 开启事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := s.q.WithTx(tx)

	publicID := uuid.Must(uuid.NewV7()).String()
	pid, err := q.CreateProduct(ctx, sqlcgen.CreateProductParams{
		PublicID: publicID, Title: in.Title, Slug: in.Slug, Description: in.Description, Status: status,
	})
	if err != nil {
		if isUnique(err) {
			return "", vErr("slug %q 已存在", in.Slug)
		}
		return "", fmt.Errorf("catalog: 建商品失败: %w", err)
	}

	// 建轴与轴值，构建 name->optionID、name->value->valueID。
	optionID := map[string]int64{}
	valueID := map[string]map[string]int64{}
	for i, opt := range in.Options {
		oid, err := q.CreateOption(ctx, sqlcgen.CreateOptionParams{ProductID: pid, Name: opt.Name, Position: int64(i)})
		if err != nil {
			return "", fmt.Errorf("catalog: 建轴失败: %w", err)
		}
		optionID[opt.Name] = oid
		valueID[opt.Name] = map[string]int64{}
		for j, v := range opt.Values {
			vid, err := q.CreateOptionValue(ctx, sqlcgen.CreateOptionValueParams{OptionID: oid, Value: v, Position: int64(j)})
			if err != nil {
				return "", fmt.Errorf("catalog: 建轴值失败: %w", err)
			}
			valueID[opt.Name][v] = vid
		}
	}

	for pos, vin := range in.Variants {
		valIDs, err := resolveSelections(in.Options, vin.Selections, valueID)
		if err != nil {
			return "", err
		}
		var sku sql.NullString
		if strings.TrimSpace(vin.SKU) != "" {
			sku = sql.NullString{String: vin.SKU, Valid: true}
		}
		vid, err := q.CreateVariant(ctx, sqlcgen.CreateVariantParams{
			PublicID: uuid.Must(uuid.NewV7()).String(), ProductID: pid, Sku: sku,
			PriceCents: vin.PriceCents, OptionKey: optionKey(valIDs), Position: int64(pos),
		})
		if err != nil {
			if isUnique(err) {
				return "", vErr("第 %d 个变体的选项组合重复或 SKU 冲突", pos+1)
			}
			return "", fmt.Errorf("catalog: 建变体失败: %w", err)
		}
		for opt, vidVal := range selectionMap(vin.Selections) {
			if err := q.AddVariantOptionValue(ctx, sqlcgen.AddVariantOptionValueParams{
				VariantID: vid, OptionID: optionID[opt], ValueID: valueID[opt][vidVal],
			}); err != nil {
				return "", fmt.Errorf("catalog: 连接变体选项值失败: %w", err)
			}
		}
		if err := q.SetInventory(ctx, sqlcgen.SetInventoryParams{VariantID: vid, Quantity: vin.Quantity}); err != nil {
			return "", fmt.Errorf("catalog: 置库存失败: %w", err)
		}
	}

	for _, cpid := range in.CategoryPublicIDs {
		cat, err := q.GetCategoryByPublicID(ctx, cpid)
		if errors.Is(err, sql.ErrNoRows) {
			return "", vErr("分类 %q 不存在", cpid)
		} else if err != nil {
			return "", fmt.Errorf("catalog: 取分类失败: %w", err)
		}
		if err := q.LinkProductCategory(ctx, sqlcgen.LinkProductCategoryParams{ProductID: pid, CategoryID: cat.ID}); err != nil {
			return "", fmt.Errorf("catalog: 关联分类失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("catalog: 提交失败: %w", err)
	}
	return publicID, nil
}

// ProductIDByPublicID 把商品 public_id 解析为内部主键。不存在返回 ErrNotFound。
func (s *Service) ProductIDByPublicID(ctx context.Context, publicID string) (int64, error) {
	p, err := s.q.GetProductByPublicID(ctx, publicID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	} else if err != nil {
		return 0, fmt.Errorf("catalog: 解析商品失败: %w", err)
	}
	return p.ID, nil
}

// ListProducts 列出未删除商品（新建在前）。
func (s *Service) ListProducts(ctx context.Context) ([]ProductSummary, error) {
	rows, err := s.q.ListProducts(ctx)
	if err != nil {
		return nil, fmt.Errorf("catalog: 列商品失败: %w", err)
	}
	out := make([]ProductSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, ProductSummary{PublicID: r.PublicID, Title: r.Title, Slug: r.Slug, Status: r.Status, UpdatedAt: r.UpdatedAt})
	}
	return out, nil
}

// GetProduct 返回商品详情（含变体矩阵）。不存在返回 ErrNotFound。
func (s *Service) GetProduct(ctx context.Context, publicID string) (*ProductDetail, error) {
	p, err := s.q.GetProductByPublicID(ctx, publicID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("catalog: 取商品失败: %w", err)
	}
	matrix, err := s.GetVariantMatrix(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	return &ProductDetail{
		PublicID: p.PublicID, Title: p.Title, Slug: p.Slug, Description: p.Description, Status: p.Status, Variants: matrix,
	}, nil
}

// UpdateProduct 改商品标题/描述/状态。不存在返回 ErrNotFound。
func (s *Service) UpdateProduct(ctx context.Context, publicID, title, description, status string) error {
	if strings.TrimSpace(title) == "" {
		return vErr("标题不能为空")
	}
	if !validStatuses[status] {
		return vErr("状态非法（应为 draft/active/archived）")
	}
	p, err := s.q.GetProductByPublicID(ctx, publicID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("catalog: 取商品失败: %w", err)
	}
	return s.q.UpdateProduct(ctx, sqlcgen.UpdateProductParams{Title: title, Description: description, Status: status, ID: p.ID})
}

// DeleteProduct 软删商品及其变体。不存在返回 ErrNotFound。
func (s *Service) DeleteProduct(ctx context.Context, publicID string) error {
	p, err := s.q.GetProductByPublicID(ctx, publicID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("catalog: 取商品失败: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("catalog: 开启事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := s.q.WithTx(tx)
	if err := q.SoftDeleteVariantsByProduct(ctx, p.ID); err != nil {
		return fmt.Errorf("catalog: 软删变体失败: %w", err)
	}
	if err := q.SoftDeleteProduct(ctx, p.ID); err != nil {
		return fmt.Errorf("catalog: 软删商品失败: %w", err)
	}
	return tx.Commit()
}

// SetVariantInventory 按变体 public_id 设库存数量。不存在返回 ErrNotFound。
func (s *Service) SetVariantInventory(ctx context.Context, variantPublicID string, quantity int64) error {
	if quantity < 0 {
		return vErr("库存数量不能为负")
	}
	v, err := s.q.GetVariantByPublicID(ctx, variantPublicID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("catalog: 取变体失败: %w", err)
	}
	return s.q.SetInventory(ctx, sqlcgen.SetInventoryParams{VariantID: v.ID, Quantity: quantity})
}

// CreateCategory 建分类，返回 public_id。
func (s *Service) CreateCategory(ctx context.Context, name, slug string) (string, error) {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(slug) == "" {
		return "", vErr("分类名与 slug 不能为空")
	}
	publicID := uuid.Must(uuid.NewV7()).String()
	_, err := s.q.CreateCategory(ctx, sqlcgen.CreateCategoryParams{PublicID: publicID, Name: name, Slug: slug, Position: 0})
	if err != nil {
		if isUnique(err) {
			return "", vErr("分类 slug %q 已存在", slug)
		}
		return "", fmt.Errorf("catalog: 建分类失败: %w", err)
	}
	return publicID, nil
}

// ListCategories 列出未删除分类。
func (s *Service) ListCategories(ctx context.Context) ([]CategorySummary, error) {
	rows, err := s.q.ListCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("catalog: 列分类失败: %w", err)
	}
	out := make([]CategorySummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, CategorySummary{PublicID: r.PublicID, Name: r.Name, Slug: r.Slug})
	}
	return out, nil
}

// ---- 校验辅助 ----

func validateProductInput(in ProductInput) error {
	if strings.TrimSpace(in.Title) == "" {
		return vErr("标题不能为空")
	}
	if strings.TrimSpace(in.Slug) == "" {
		return vErr("slug 不能为空")
	}
	if in.Status != "" && !validStatuses[in.Status] {
		return vErr("状态非法（应为 draft/active/archived）")
	}
	if len(in.Options) == 0 {
		return vErr("至少需要一个变体轴")
	}
	seenOpt := map[string]bool{}
	for _, opt := range in.Options {
		if strings.TrimSpace(opt.Name) == "" {
			return vErr("变体轴名不能为空")
		}
		if seenOpt[opt.Name] {
			return vErr("变体轴 %q 重复", opt.Name)
		}
		seenOpt[opt.Name] = true
		if len(opt.Values) == 0 {
			return vErr("变体轴 %q 至少需要一个取值", opt.Name)
		}
		seenVal := map[string]bool{}
		for _, v := range opt.Values {
			if strings.TrimSpace(v) == "" {
				return vErr("变体轴 %q 的取值不能为空", opt.Name)
			}
			if seenVal[v] {
				return vErr("变体轴 %q 取值 %q 重复", opt.Name, v)
			}
			seenVal[v] = true
		}
	}
	if len(in.Variants) == 0 {
		return vErr("至少需要一个变体")
	}
	for i, vin := range in.Variants {
		if vin.PriceCents < 0 {
			return vErr("第 %d 个变体价格不能为负", i+1)
		}
		if vin.Quantity < 0 {
			return vErr("第 %d 个变体库存不能为负", i+1)
		}
	}
	return nil
}

// resolveSelections 校验变体的取值覆盖了每个轴恰好一次且取值合法，返回 value id 列表。
func resolveSelections(options []OptionInput, sels []Selection, valueID map[string]map[string]int64) ([]int64, error) {
	if len(sels) != len(options) {
		return nil, vErr("变体须对每个轴恰好选一个取值（期望 %d 个，得到 %d 个）", len(options), len(sels))
	}
	picked := map[string]bool{}
	ids := make([]int64, 0, len(sels))
	for _, sel := range sels {
		vals, ok := valueID[sel.Option]
		if !ok {
			return nil, vErr("未知变体轴 %q", sel.Option)
		}
		if picked[sel.Option] {
			return nil, vErr("变体轴 %q 被重复选择", sel.Option)
		}
		vid, ok := vals[sel.Value]
		if !ok {
			return nil, vErr("变体轴 %q 无取值 %q", sel.Option, sel.Value)
		}
		picked[sel.Option] = true
		ids = append(ids, vid)
	}
	return ids, nil
}

func selectionMap(sels []Selection) map[string]string {
	m := make(map[string]string, len(sels))
	for _, s := range sels {
		m[s.Option] = s.Value
	}
	return m
}

func isUnique(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
