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
	"time"

	"github.com/google/uuid"

	"github.com/kartwo/kartwo/internal/settings"
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
	settings *settings.Service
}

// New 构造下单服务（货币按当前主攻市场解析）。
func New(db *sql.DB, settingsSvc *settings.Service) *Service {
	return &Service{db: db, q: sqlcgen.New(db), settings: settingsSvc}
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
	PublicID        string
	Status          string
	Email           string
	ShipName        string
	ShipPhone       string
	ShipAddress     string
	ShipCountry     string
	Currency        string
	SubtotalCents   int64
	TotalCents      int64
	PaymentProvider string
	CreatedAt       string
	Lines           []OrderLine
	Refunds         []RefundView // 仅后台详情填充
}

// OrderSummary 后台订单列表项。
type OrderSummary struct {
	PublicID        string
	Status          string
	Email           string
	Currency        string
	TotalCents      int64
	PaymentProvider string
	CreatedAt       string
}

// RefundView 退款记录视图。
type RefundView struct {
	Provider         string
	ProviderRefundID string
	AmountCents      int64
	CreatedAt        string
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

	// 货币在开事务前解析（事务持有唯一连接时再查会死锁）。
	currency := s.settings.Currency(ctx)

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
		Currency: currency, SubtotalCents: subtotal, TotalCents: subtotal, // v1: 税/运 = 0
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
		SubtotalCents: o.SubtotalCents, TotalCents: o.TotalCents, PaymentProvider: o.PaymentProvider, CreatedAt: o.CreatedAt,
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

// AdminList 返回后台订单列表（按时间倒序）。
func (s *Service) AdminList(ctx context.Context) ([]OrderSummary, error) {
	rows, err := s.q.ListOrdersForAdmin(ctx)
	if err != nil {
		return nil, fmt.Errorf("order: 列订单失败: %w", err)
	}
	out := make([]OrderSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, OrderSummary{
			PublicID: r.PublicID, Status: r.Status, Email: r.Email, Currency: r.Currency,
			TotalCents: r.TotalCents, PaymentProvider: r.PaymentProvider, CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}

// AdminGet 返回后台订单详情（含行与退款记录）。不存在返回 sql.ErrNoRows。
func (s *Service) AdminGet(ctx context.Context, publicID string) (*Order, error) {
	out, err := s.Get(ctx, publicID)
	if err != nil {
		return nil, err
	}
	o, err := s.q.GetOrderByPublicID(ctx, publicID)
	if err != nil {
		return nil, fmt.Errorf("order: 取订单失败: %w", err)
	}
	rfs, err := s.q.ListRefundsByOrder(ctx, o.ID)
	if err != nil {
		return nil, fmt.Errorf("order: 列退款失败: %w", err)
	}
	for _, r := range rfs {
		out.Refunds = append(out.Refunds, RefundView{
			Provider: r.Provider, ProviderRefundID: r.ProviderRefundID, AmountCents: r.AmountCents, CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}

// DashboardStats 概览订单聚合：今日 / 近 7 日订单数与销售额、待处理数、展示货币。
//   - 时间口径（D1）：按服务器本地自然日——今日=[本地今日零点, 现在]；近 7 日=[本地(今日-6天)零点, 现在]。
//     本地零点换算为 UTC ISO8601 与库内 created_at（UTC）按字符串字典序比较（下界 .000，词法比较安全）。
//   - 销售额口径（D6）：仅计 paid/fulfilled 合计；refunded 单转 refunded 状态、天然不计入（=全额扣除），
//     且不依赖 refund 记录（webhook 同步的退款可能无记录）。部分退款是 v1 之后，届时改按 refund.amount_cents 扣减。
//   - 只读、无事务：单连接上顺序跑 3 个聚合查询（不开事务再发独立查询，见 DECISIONS 单连接纪律）。
type DashboardStats struct {
	TodayCount         int64
	TodaySalesCents    int64
	WeekCount          int64
	WeekSalesCents     int64
	PendingFulfillment int64
	Currency           string
}

// dashboardWindowBounds 把"服务器本地自然日"的今日/近7日下界，换算为对 UTC 存储的 created_at 可做词法比较的 UTC 串。
//
//	正确性关键（跨时区+跨 UTC 日界）：先在本地时区取零点 time.Date(...,now.Location())，再 .UTC() 落到真实 UTC
//	瞬间、按库内同格式（毫秒+Z）输出。故东八区下"本地今日但 UTC 仍是昨日"的订单（如本地 02:00 = UTC 前一日 18:00），
//	其 created_at 仍 >= todayBound（本地零点对应的 UTC 前一日 16:00），被正确计入今日——而非按 UTC 自然日误判为昨日。
//	纯函数、注入 now 便于确定性单测（固定时区+固定 now，不依赖运行环境时区）。
func dashboardWindowBounds(now time.Time) (todayBound, weekBound string) {
	const layout = "2006-01-02T15:04:05.000Z" // 匹配库内 strftime('%Y-%m-%dT%H:%M:%fZ') 的 UTC 形态
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	weekStart := todayStart.AddDate(0, 0, -6) // 含今日的近 7 个自然日
	return todayStart.UTC().Format(layout), weekStart.UTC().Format(layout)
}

// DashboardStats 计算并返回概览订单聚合。
func (s *Service) DashboardStats(ctx context.Context) (DashboardStats, error) {
	todayBound, weekBound := dashboardWindowBounds(time.Now())
	today, err := s.q.DashboardOrderWindow(ctx, todayBound)
	if err != nil {
		return DashboardStats{}, fmt.Errorf("order: 概览今日聚合失败: %w", err)
	}
	week, err := s.q.DashboardOrderWindow(ctx, weekBound)
	if err != nil {
		return DashboardStats{}, fmt.Errorf("order: 概览近7日聚合失败: %w", err)
	}
	pending, err := s.q.DashboardPendingFulfillment(ctx)
	if err != nil {
		return DashboardStats{}, fmt.Errorf("order: 概览待处理聚合失败: %w", err)
	}
	return DashboardStats{
		TodayCount: today.OrderCount, TodaySalesCents: today.SalesCents,
		WeekCount: week.OrderCount, WeekSalesCents: week.SalesCents,
		PendingFulfillment: pending,
		Currency:           s.settings.Currency(ctx),
	}, nil
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
