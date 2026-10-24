// 订单资金视图 / Order Financial View
// 功能：为后台展示独立收款、退款及履约状态，不向顾客公开网关引用
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-24 17:15:00
package order

import "context"

// ReceiptView is a verified or explicitly marked legacy payment, never a new
// payment inferred from fulfillment. Reference is restricted by the admin API.
type ReceiptView struct {
	Provider, Reference, Currency, ReviewReason, Source, CreatedAt string
	AmountCents, RefundedCents                                     int64
}

func (s *Service) loadFinancials(ctx context.Context, orderID int64, out *Order) error {
	var shipped bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM shipment WHERE order_id=?)`, orderID).Scan(&shipped); err != nil {
		return err
	}
	out.FulfillmentStatus = "unfulfilled"
	if shipped {
		out.FulfillmentStatus = "fulfilled"
	}
	rows, err := s.db.QueryContext(ctx, `SELECT provider,payment_ref,currency,review_reason,source,created_at,amount_cents,refunded_cents FROM payment_receipt WHERE order_id=? ORDER BY id`, orderID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	out.PaymentStatus = "unrecorded"
	for rows.Next() {
		var p ReceiptView
		if err := rows.Scan(&p.Provider, &p.Reference, &p.Currency, &p.ReviewReason, &p.Source, &p.CreatedAt, &p.AmountCents, &p.RefundedCents); err != nil {
			return err
		}
		out.Receipts = append(out.Receipts, p)
		out.PaidCents += p.AmountCents
		out.RefundedCents += p.RefundedCents
		out.PaymentReview = out.PaymentReview || p.ReviewReason != "" && p.RefundedCents < p.AmountCents
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(out.Receipts) > 0 {
		switch {
		case out.RefundedCents == out.PaidCents:
			out.PaymentStatus = "refunded"
		case out.RefundedCents > 0:
			out.PaymentStatus = "partially_refunded"
		default:
			out.PaymentStatus = "paid"
		}
	} else if out.Status == "pending" || out.Status == "cancelled" {
		out.PaymentStatus = "unpaid"
	}
	return nil
}

// RefundRequestView exposes saved intentions and status, separately from
// completed refund transactions. Internal row IDs and credentials stay private.
type RefundRequestView struct {
	ID, Provider, PaymentRef, Currency, State, GatewayRef, Reason, CreatedAt string
	AmountCents                                                              int64
}

func (s *Service) loadRefundRequests(ctx context.Context, orderID int64, out *Order) error {
	rows, err := s.db.QueryContext(ctx, `SELECT public_id,provider,payment_ref,currency,state,gateway_ref,reason,created_at,amount_cents FROM refund_request WHERE order_id=? ORDER BY id DESC`, orderID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var r RefundRequestView
		if err := rows.Scan(&r.ID, &r.Provider, &r.PaymentRef, &r.Currency, &r.State, &r.GatewayRef, &r.Reason, &r.CreatedAt, &r.AmountCents); err != nil {
			return err
		}
		out.RefundRequests = append(out.RefundRequests, r)
	}
	return rows.Err()
}
