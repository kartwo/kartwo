// 确认信入队 / Order Confirmation Enqueue
// 功能：支付确认后组装顾客下单确认信（纯文本，单语言 en，D9）并 INSERT OR IGNORE 入 outbox（幂等）
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-07-28 13:27:02
package mail

import (
	"context"
	"fmt"
	"strings"

	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

// KindOrderConfirmation 是 outbox 信种（v1 仅此一种）。
const KindOrderConfirmation = "order_confirmation"

// OrderInfo 是组装确认信所需的订单头（调用方已在支付流里持有，避免重复查订单）。
type OrderInfo struct {
	ID         int64
	PublicID   string
	Email      string
	Currency   string
	TotalCents int64
}

// EnqueueOrderConfirmation 组装并 INSERT OR IGNORE 入队一封确认信（幂等：UNIQUE(order_id,kind)）。
// 用传入的 q（可为事务绑定的 Queries），故可与 pending->paid 在同一事务内原子入队。
// 读订单行用同一 q（同连接/同事务，不新开连接，守单连接纪律）。
func EnqueueOrderConfirmation(ctx context.Context, q *sqlcgen.Queries, o OrderInfo) error {
	if strings.TrimSpace(o.Email) == "" {
		return fmt.Errorf("mail: 订单 %s 无收件邮箱，不入队", o.PublicID)
	}
	items, err := q.ListOrderItems(ctx, o.ID)
	if err != nil {
		return fmt.Errorf("mail: 取订单行失败: %w", err)
	}
	subject, body := composeConfirmation(o, items)
	if _, err := q.EnqueueEmail(ctx, sqlcgen.EnqueueEmailParams{
		OrderID: o.ID, Kind: KindOrderConfirmation, ToAddr: o.Email, Subject: subject, Body: body,
	}); err != nil {
		return fmt.Errorf("mail: 入队确认信失败: %w", err)
	}
	return nil
}

// composeConfirmation 组装纯文本确认信（英文，v1 单语言、无营销）。金额整数分→展示层转。
func composeConfirmation(o OrderInfo, items []sqlcgen.ListOrderItemsRow) (subject, body string) {
	short := o.PublicID
	if len(short) > 8 {
		short = short[:8]
	}
	subject = "Your order " + short + " is confirmed"

	var b strings.Builder
	b.WriteString("Hi,\n\n")
	b.WriteString("Thanks for your order — we've received your payment.\n\n")
	b.WriteString("Order: " + o.PublicID + "\n")
	b.WriteString("Total: " + money(o.TotalCents, o.Currency) + "\n\n")
	b.WriteString("Items:\n")
	for _, it := range items {
		line := "- " + it.ProductTitle
		if strings.TrimSpace(it.VariantLabel) != "" {
			line += " (" + it.VariantLabel + ")"
		}
		line += fmt.Sprintf(" x%d  %s", it.Quantity, money(it.LineCents, o.Currency))
		b.WriteString(line + "\n")
	}
	b.WriteString("\nWe'll email you again when your order ships.\n")
	return subject, b.String()
}

// money 整数分 → 带币种的展示串（如 "USD 99.00"）。
func money(cents int64, currency string) string {
	return fmt.Sprintf("%s %d.%02d", currency, cents/100, abs(cents%100))
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
