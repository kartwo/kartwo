// 订单工作台 / Order Workbench
// 功能：在同一读取快照内提供有界搜索、稳定分页与资金/履约待处理视图，服务层限制客户搜索范围
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 13:42:16
package order

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

var ErrInvalidOrderSearch = errors.New("订单查询条件无效")

// OrderSearchFilter contains no permission flag; customer access comes from the authenticated caller.
type OrderSearchFilter struct {
	Term, View, Status, Provider, From, To string
	Page, PageSize                         int64
}

type WorkbenchOrder struct {
	OrderSummary
	Shipped, RefundPending, NeedsReview, ReadyToFulfill bool
}

type OrderWorkbench struct {
	Orders                []WorkbenchOrder
	Counts                map[string]int64
	Total, Page, PageSize int64
	View                  string
}

func normalizeOrderSearch(f OrderSearchFilter) (OrderSearchFilter, string, string, error) {
	invalid := func(message string) (OrderSearchFilter, string, string, error) {
		return f, "", "", fmt.Errorf("%w：%s", ErrInvalidOrderSearch, message)
	}
	f.Term = strings.ToLower(strings.TrimSpace(f.Term))
	if !utf8.ValidString(f.Term) || utf8.RuneCountInString(f.Term) > 160 || strings.ContainsFunc(f.Term, unicode.IsControl) {
		return invalid("搜索内容最多 160 字，不能包含控制字符")
	}
	if f.View == "" {
		f.View = "all"
	}
	switch f.View {
	case "all", "awaiting_payment", "ready_to_fulfill", "needs_review", "refunds_pending", "shipped":
	default:
		return invalid("待处理视图无效")
	}
	switch f.Status {
	case "", "pending", "paid", "fulfilled", "refunded", "cancelled", "payment_review":
	default:
		return invalid("订单状态无效")
	}
	if f.Provider != "" && f.Provider != "stripe" && f.Provider != "paypal" {
		return invalid("支付方式无效")
	}
	if f.Page < 0 || f.PageSize < 0 || f.PageSize > 100 {
		return invalid("页码须为正数，每页最多 100 条")
	}
	if f.Page == 0 {
		f.Page = 1
	}
	if f.PageSize == 0 {
		f.PageSize = 20
	}
	var from, to string
	for _, date := range []string{f.From, f.To} {
		if date == "" {
			continue
		}
		parsed, err := time.Parse("2006-01-02", date)
		if err != nil || parsed.Format("2006-01-02") != date {
			return invalid("日期须为有效的 YYYY-MM-DD")
		}
	}
	if f.From != "" && f.To != "" && f.From > f.To {
		return invalid("结束日期不能早于开始日期")
	}
	if f.From != "" {
		from = f.From + "T00:00:00.000Z"
	}
	if f.To != "" {
		date, _ := time.Parse("2006-01-02", f.To)
		next := date.AddDate(0, 0, 1)
		to = next.Format("2006-01-02T15:04:05.000Z")
		if next.Year() > 9999 {
			// An upper prefix bound includes the last representable UTC day without a five-digit year.
			to = f.To + "~"
		}
	}
	return f, from, to, nil
}

// AdminSearch keeps aggregate counts and rows in one read transaction. Unknown/empty results are never success fallbacks.
func (s *Service) AdminSearch(ctx context.Context, filter OrderSearchFilter, customerData bool) (*OrderWorkbench, error) {
	f, from, to, err := normalizeOrderSearch(filter)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var customers int64
	if customerData {
		customers = 1
	}
	q := s.q.WithTx(tx)
	counts, err := q.CountAdminOrderViews(ctx, sqlcgen.CountAdminOrderViewsParams{CustomerData: customers, Status: f.Status, Provider: f.Provider, FromTime: from, ToTime: to, Term: f.Term})
	if err != nil {
		return nil, err
	}
	out := &OrderWorkbench{Page: f.Page, PageSize: f.PageSize, View: f.View, Orders: []WorkbenchOrder{}, Counts: map[string]int64{
		"all": counts.AllOrders, "awaiting_payment": counts.AwaitingPayment, "ready_to_fulfill": counts.ReadyToFulfill,
		"needs_review": counts.NeedsReview, "refunds_pending": counts.RefundsPending, "shipped": counts.Shipped,
	}}
	out.Total = out.Counts[f.View]
	last := int64(1)
	if out.Total > 0 {
		last = (out.Total-1)/f.PageSize + 1
	}
	if out.Page > last {
		out.Page = last
	}
	rows, err := q.ListAdminOrderPage(ctx, sqlcgen.ListAdminOrderPageParams{View: f.View, PageOffset: (out.Page - 1) * f.PageSize, PageSize: f.PageSize, CustomerData: customers, Status: f.Status, Provider: f.Provider, FromTime: from, ToTime: to, Term: f.Term})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		email := "已隐藏"
		if customerData {
			email = row.Email
		}
		out.Orders = append(out.Orders, WorkbenchOrder{OrderSummary: OrderSummary{PublicID: row.PublicID, Status: row.Status, Email: email, Currency: row.Currency, TotalCents: row.TotalCents, PaymentProvider: row.PaymentProvider, CreatedAt: row.CreatedAt}, Shipped: row.Shipped == 1, RefundPending: row.RefundPending == 1, NeedsReview: row.NeedsReview == 1, ReadyToFulfill: row.ReadyToFulfill == 1})
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}
