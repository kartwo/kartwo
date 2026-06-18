// 下单服务 / Checkout & Order Service
// 功能：由购物车结算生成订单（访客）；事务内原子预留库存防超卖；规格/单价快照
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-06-18 13:40:31
package order

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

var (
	// ErrEmptyCart 购物车为空，不能结算。
	ErrEmptyCart = errors.New("order: 购物车为空")
	// ErrOutOfStock 某变体库存不足（防超卖拦截）。
	ErrOutOfStock = errors.New("order: 库存不足")
	// ErrInvalidInfo 结算信息非法。
	ErrInvalidInfo = errors.New("order: 结算信息不完整")
)

// Service 承载结算/订单逻辑。
type Service struct {
	db       *sql.DB
	q        *sqlcgen.Queries
	currency string
}

// New 构造下单服务。
func New(db *sql.DB, currency string) *Service {
	return &Service{db: db, q: sqlcgen.New(db), currency: currency}
}

// CheckoutInfo 为访客结算填写的信息。
type CheckoutInfo struct {
	Email   string
	Name    string
	Phone   string
	Address string
	Country string
}

// OrderLine 订单行视图。
type OrderLine struct {
	Title     string
	Spec      string
	SKU       string
	UnitCents int64
	Quantity  int64
	LineCents int64
}

// Order 订单视图。
type Order struct {
	PublicID      string
	Status        string
	Email         string
	ShipName      string
	ShipPhone     string
	ShipAddress   string
	ShipCountry   string
	Currency      string
	SubtotalCents int64
	TotalCents    int64
	CreatedAt     string
	Lines         []OrderLine
}

func (in CheckoutInfo) validate() error {
	if !strings.Contains(in.Email, "@") || strings.TrimSpace(in.Email) == "" {
		return fmt.Errorf("%w: 邮箱非法", ErrInvalidInfo)
	}
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("%w: 收货人不能为空", ErrInvalidInfo)
	}
	if strings.TrimSpace(in.Address) == "" {
		return fmt.Errorf("%w: 收货地址不能为空", ErrInvalidInfo)
	}
	return nil
}

// Checkout 由购物车结算生成订单。事务内：原子预留库存（防超卖）→ 建客户/订单/订单行 → 转换购物车。
// 任一变体库存不足则整单回滚（释放本次已预留）。
func (s *Service) Checkout(ctx context.Context, cartID int64, info CheckoutInfo) (string, error) {
	if err := info.validate(); err != nil {
		return "", err
	}
	items, err := s.q.ListCartItems(ctx, cartID)
	if err != nil {
		return "", fmt.Errorf("order: 读购物车失败: %w", err)
	}
	if len(items) == 0 {
		return "", ErrEmptyCart
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("order: 开启事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := s.q.WithTx(tx)

	// 1) 原子预留库存（防超卖）。任一不足即整单失败、回滚释放。
	for _, it := range items {
		n, err := q.ReserveInventory(ctx, sqlcgen.ReserveInventoryParams{
			Reserved: it.Quantity, VariantID: it.VariantID, Quantity: it.Quantity,
		})
		if err != nil {
			return "", fmt.Errorf("order: 预留库存失败: %w", err)
		}
		if n == 0 {
			return "", fmt.Errorf("%w: 「%s」", ErrOutOfStock, it.ProductTitle)
		}
	}

	// 2) 客户（按邮箱 upsert）。
	if err := q.UpsertCustomer(ctx, sqlcgen.UpsertCustomerParams{
		PublicID: uuid.Must(uuid.NewV7()).String(), Email: info.Email, Name: info.Name,
	}); err != nil {
		return "", fmt.Errorf("order: 写客户失败: %w", err)
	}
	cust, err := q.GetCustomerByEmail(ctx, info.Email)
	if err != nil {
		return "", fmt.Errorf("order: 取客户失败: %w", err)
	}

	// 3) 合计 + 建订单。
	var subtotal int64
	for _, it := range items {
		subtotal += it.PriceCents * it.Quantity
	}
	publicID := uuid.Must(uuid.NewV7()).String()
	orderID, err := q.CreateOrder(ctx, sqlcgen.CreateOrderParams{
		PublicID: publicID, CustomerID: cust.ID, Email: info.Email,
		ShipName: info.Name, ShipPhone: info.Phone, ShipAddress: info.Address, ShipCountry: info.Country,
		Currency: s.currency, SubtotalCents: subtotal, TotalCents: subtotal, // v1: 税/运 = 0
	})
	if err != nil {
		return "", fmt.Errorf("order: 建订单失败: %w", err)
	}

	// 4) 订单行（快照：标题/规格/单价）。
	for _, it := range items {
		label, err := s.variantLabel(ctx, q, it.VariantID)
		if err != nil {
			return "", err
		}
		sku := ""
		if it.Sku.Valid {
			sku = it.Sku.String
		}
		if err := q.CreateOrderItem(ctx, sqlcgen.CreateOrderItemParams{
			OrderID: orderID, VariantID: it.VariantID, ProductTitle: it.ProductTitle,
			VariantLabel: label, Sku: sku, UnitCents: it.PriceCents, Quantity: it.Quantity,
			LineCents: it.PriceCents * it.Quantity,
		}); err != nil {
			return "", fmt.Errorf("order: 建订单行失败: %w", err)
		}
	}

	// 5) 购物车转换为已下单。
	if err := q.MarkCartConverted(ctx, cartID); err != nil {
		return "", fmt.Errorf("order: 转换购物车失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("order: 提交失败: %w", err)
	}
	return publicID, nil
}

// Get 返回订单详情（含行）。不存在返回 sql.ErrNoRows 包装。
func (s *Service) Get(ctx context.Context, publicID string) (*Order, error) {
	o, err := s.q.GetOrderByPublicID(ctx, publicID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	} else if err != nil {
		return nil, fmt.Errorf("order: 取订单失败: %w", err)
	}
	out := &Order{
		PublicID: o.PublicID, Status: o.Status, Email: o.Email, ShipName: o.ShipName, ShipPhone: o.ShipPhone,
		ShipAddress: o.ShipAddress, ShipCountry: o.ShipCountry, Currency: o.Currency,
		SubtotalCents: o.SubtotalCents, TotalCents: o.TotalCents, CreatedAt: o.CreatedAt,
	}
	its, err := s.q.ListOrderItems(ctx, o.ID)
	if err != nil {
		return nil, fmt.Errorf("order: 列订单行失败: %w", err)
	}
	for _, it := range its {
		out.Lines = append(out.Lines, OrderLine{
			Title: it.ProductTitle, Spec: it.VariantLabel, SKU: it.Sku,
			UnitCents: it.UnitCents, Quantity: it.Quantity, LineCents: it.LineCents,
		})
	}
	return out, nil
}

// variantLabel 把变体选项拼为快照文字（如 尺码:S · 颜色:黑）。
func (s *Service) variantLabel(ctx context.Context, q *sqlcgen.Queries, variantID int64) (string, error) {
	opts, err := q.ListOptionsForVariant(ctx, variantID)
	if err != nil {
		return "", fmt.Errorf("order: 取变体选项失败: %w", err)
	}
	parts := make([]string, 0, len(opts))
	for _, o := range opts {
		parts = append(parts, o.OptionName+":"+o.OptionValue)
	}
	return strings.Join(parts, " · "), nil
}
