// 购物车服务 / Cart Service
// 功能：匿名购物车 get-or-create、加/改/删行、查看（含选项/缩略图/合计）；状态机 active
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-18 12:40:00
package cart

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

const maxQtyPerLine = 999

var (
	// ErrVariantNotFound 指定变体不存在。
	ErrVariantNotFound = errors.New("cart: 变体不存在")
	// ErrInvalidQty 数量非法。
	ErrInvalidQty = errors.New("cart: 数量非法")
)

// Service 承载购物车逻辑。
type Service struct {
	db *sql.DB
	q  *sqlcgen.Queries
}

// New 构造购物车服务。
func New(db *sql.DB) *Service { return &Service{db: db, q: sqlcgen.New(db)} }

// OptionPair 变体某轴取值。
type OptionPair struct{ Name, Value string }

// CartLine 购物车一行。
type CartLine struct {
	VariantPublicID string
	ProductTitle    string
	ProductSlug     string
	SKU             string
	Options         []OptionPair
	UnitCents       int64
	Quantity        int64
	LineCents       int64
	ThumbURL        string
}

// CartView 购物车整体视图。
type CartView struct {
	Lines      []CartLine
	TotalCents int64
	Count      int64
}

// GetOrCreate 按 token 取活跃购物车；无则新建并返回新 token。
func (s *Service) GetOrCreate(ctx context.Context, token string) (cartID int64, cartToken string, err error) {
	if token != "" {
		c, e := s.q.GetActiveCartByToken(ctx, token)
		if e == nil {
			return c.ID, c.Token, nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return 0, "", fmt.Errorf("cart: 取购物车失败: %w", e)
		}
	}
	newTok, err := randToken()
	if err != nil {
		return 0, "", err
	}
	id, err := s.q.CreateCart(ctx, newTok)
	if err != nil {
		return 0, "", fmt.Errorf("cart: 建购物车失败: %w", err)
	}
	return id, newTok, nil
}

// AddItem 加入（或累加）一个变体。
func (s *Service) AddItem(ctx context.Context, cartID int64, variantPublicID string, qty int64) error {
	if qty <= 0 || qty > maxQtyPerLine {
		return ErrInvalidQty
	}
	v, err := s.variant(ctx, variantPublicID)
	if err != nil {
		return err
	}
	if err := s.q.AddCartItem(ctx, sqlcgen.AddCartItemParams{CartID: cartID, VariantID: v.ID, Quantity: qty}); err != nil {
		return fmt.Errorf("cart: 加入失败: %w", err)
	}
	_ = s.q.TouchCart(ctx, cartID)
	return nil
}

// SetQty 设置某行数量；qty<=0 视为删除。
func (s *Service) SetQty(ctx context.Context, cartID int64, variantPublicID string, qty int64) error {
	if qty > maxQtyPerLine {
		return ErrInvalidQty
	}
	v, err := s.variant(ctx, variantPublicID)
	if err != nil {
		return err
	}
	if qty <= 0 {
		if err := s.q.RemoveCartItem(ctx, sqlcgen.RemoveCartItemParams{CartID: cartID, VariantID: v.ID}); err != nil {
			return fmt.Errorf("cart: 删除失败: %w", err)
		}
	} else if err := s.q.SetCartItemQty(ctx, sqlcgen.SetCartItemQtyParams{Quantity: qty, CartID: cartID, VariantID: v.ID}); err != nil {
		return fmt.Errorf("cart: 改量失败: %w", err)
	}
	_ = s.q.TouchCart(ctx, cartID)
	return nil
}

// RemoveItem 删除某行。
func (s *Service) RemoveItem(ctx context.Context, cartID int64, variantPublicID string) error {
	v, err := s.variant(ctx, variantPublicID)
	if err != nil {
		return err
	}
	if err := s.q.RemoveCartItem(ctx, sqlcgen.RemoveCartItemParams{CartID: cartID, VariantID: v.ID}); err != nil {
		return fmt.Errorf("cart: 删除失败: %w", err)
	}
	_ = s.q.TouchCart(ctx, cartID)
	return nil
}

// Count 返回购物车内商品总件数。
func (s *Service) Count(ctx context.Context, cartID int64) (int64, error) {
	n, err := s.q.CountCartItems(ctx, cartID)
	if err != nil {
		return 0, fmt.Errorf("cart: 统计失败: %w", err)
	}
	return toInt64(n), nil
}

// View 组装购物车视图（含选项、缩略图、行/总计）。
func (s *Service) View(ctx context.Context, cartID int64) (*CartView, error) {
	rows, err := s.q.ListCartItems(ctx, cartID)
	if err != nil {
		return nil, fmt.Errorf("cart: 列行失败: %w", err)
	}
	view := &CartView{}
	for _, r := range rows {
		line := CartLine{
			VariantPublicID: r.VariantPublicID, ProductTitle: r.ProductTitle, ProductSlug: r.ProductSlug,
			UnitCents: r.PriceCents, Quantity: r.Quantity, LineCents: r.PriceCents * r.Quantity,
		}
		if r.Sku.Valid {
			line.SKU = r.Sku.String
		}
		opts, err := s.q.ListOptionsForVariant(ctx, r.VariantID)
		if err != nil {
			return nil, fmt.Errorf("cart: 列选项失败: %w", err)
		}
		for _, o := range opts {
			line.Options = append(line.Options, OptionPair{Name: o.OptionName, Value: o.OptionValue})
		}
		line.ThumbURL = s.firstThumb(ctx, r.ProductID)
		view.Lines = append(view.Lines, line)
		view.TotalCents += line.LineCents
		view.Count += line.Quantity
	}
	return view, nil
}

func (s *Service) variant(ctx context.Context, publicID string) (sqlcgen.GetVariantByPublicIDRow, error) {
	v, err := s.q.GetVariantByPublicID(ctx, publicID)
	if errors.Is(err, sql.ErrNoRows) {
		return v, ErrVariantNotFound
	} else if err != nil {
		return v, fmt.Errorf("cart: 取变体失败: %w", err)
	}
	return v, nil
}

func (s *Service) firstThumb(ctx context.Context, productID int64) string {
	assets, err := s.q.ListMediaByProduct(ctx, productID)
	if err != nil || len(assets) == 0 {
		return ""
	}
	ds, err := s.q.ListDerivativesByAsset(ctx, assets[0].ID)
	if err != nil {
		return ""
	}
	var thumb, any string
	for _, d := range ds {
		any = "/media/" + d.Path
		if d.Label == "thumb" {
			thumb = "/media/" + d.Path
		}
	}
	if thumb != "" {
		return thumb
	}
	return any
}

func randToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("cart: 生成 token 失败: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// toInt64 把 COUNT/SUM 的结果（interface{}）安全转 int64。
func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return 0
	}
}
