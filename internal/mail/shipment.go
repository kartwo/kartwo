// 发货通知入队 / Shipment Notice Enqueue
// 功能：按不可变运单版本生成英文纯文本通知，与发货事务原子提交且不包含访问凭证
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 15:33:44
package mail

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

// ShipmentKindPrefix binds an outbox message to one durable notice, never a URL.
const ShipmentKindPrefix = "shipment:"

type ShipmentInfo struct {
	OrderID, Version                                                int64
	OrderPublicID, NoticeID, Email, Carrier, Number, URL, ChangedAt string
}

// EnqueueShipment uses the caller's transaction. No SMTP or second DB connection.
func EnqueueShipment(ctx context.Context, q *sqlcgen.Queries, info ShipmentInfo) (int64, error) {
	if info.Version < 1 || info.NoticeID == "" || strings.ContainsFunc(info.OrderPublicID, unicode.IsControl) {
		return 0, ErrInvalidMessage
	}
	subject, body := composeShipment(info)
	return q.EnqueueShipmentEmail(ctx, sqlcgen.EnqueueShipmentEmailParams{OrderID: info.OrderID, Kind: ShipmentKindPrefix + info.NoticeID, ToAddr: info.Email, Subject: subject, Body: body})
}

func composeShipment(info ShipmentInfo) (string, string) {
	short := info.OrderPublicID
	if len(short) > 8 {
		short = short[:8]
	}
	subject := "Your order " + short + " has shipped"
	intro := "Your order has shipped."
	if info.Version > 1 {
		subject = "Tracking update for your order " + short
		intro = "The tracking details for your shipment have been updated. This replaces any earlier tracking details for this order."
	}
	body := fmt.Sprintf("Hi,\n\n%s\n\nOrder: %s\nTracking version: %d\nUpdated (UTC): %s\nCarrier: %s\nTracking number: %s\n", intro, info.OrderPublicID, info.Version, info.ChangedAt, info.Carrier, info.Number)
	if info.URL != "" {
		body += "Track your shipment: " + info.URL + "\n"
	}
	return subject, body + "\nPlease allow time for the carrier to show the first tracking scan.\n"
}
