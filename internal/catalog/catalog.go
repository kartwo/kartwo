// 商品目录服务 / Catalog Service
// 功能：变体内核领域逻辑——option_key 规范化、变体矩阵读取（通用双轴 option×option）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-17 18:13:43
package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

// Service 封装商品目录的读写。底层走 sqlc 生成代码 + 事务。
type Service struct {
	db *sql.DB
	q  *sqlcgen.Queries
}

// New 构建目录服务。
func New(db *sql.DB) *Service {
	return &Service{db: db, q: sqlcgen.New(db)}
}

// OptionPair 为变体在某个轴上的取值（如 尺码=S）。
type OptionPair struct {
	Name  string
	Value string
}

// VariantView 是面向展示/校验的变体视图：含其各轴取值与库存。
type VariantView struct {
	PublicID   string
	SKU        string
	PriceCents int64
	Quantity   int64
	Options    []OptionPair
}

// optionKey 由变体所含选项值 id 规范化而来：升序去重后以 '-' 连接。
// 用于在商品内强制"选项值组合唯一"，与具体品类无关（通用双轴/多轴皆适用）。
func optionKey(valueIDs []int64) string {
	ids := append([]int64(nil), valueIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, "-")
}

// GetVariantMatrix 返回某商品的全部变体（含各轴取值与库存），用于校验/展示。
func (s *Service) GetVariantMatrix(ctx context.Context, productID int64) ([]VariantView, error) {
	variants, err := s.q.ListVariantsByProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("catalog: 列变体失败: %w", err)
	}

	pairs, err := s.q.ListVariantOptionValuesByProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("catalog: 列变体选项值失败: %w", err)
	}
	byVariant := make(map[int64][]OptionPair, len(variants))
	for _, p := range pairs {
		byVariant[p.VariantID] = append(byVariant[p.VariantID], OptionPair{Name: p.OptionName, Value: p.OptionValue})
	}

	out := make([]VariantView, 0, len(variants))
	for _, v := range variants {
		inv, err := s.q.GetInventory(ctx, v.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("catalog: 读库存失败: %w", err)
		}
		sku := ""
		if v.Sku.Valid {
			sku = v.Sku.String
		}
		out = append(out, VariantView{
			PublicID:   v.PublicID,
			SKU:        sku,
			PriceCents: v.PriceCents,
			Quantity:   inv.Quantity, // 无库存行时 inv 为零值，Quantity=0
			Options:    byVariant[v.ID],
		})
	}
	return out, nil
}
