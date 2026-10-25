// 售后审核与退回 / After Sale Review and Return
// 功能：版本比较、原动作重放、显式收货检查和已验证退款关联；保留每一步顾客可见说明
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 20:56:26
package order

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

// ReviewAfterSale requires merchant authorization at the HTTP boundary. This
// method never invokes a payment gateway, SMTP server or inventory adjustment.
func (s *Service) ReviewAfterSale(ctx context.Context, orderID, caseID string, input AfterSaleAction) (AfterSaleCase, error) {
	return s.actAfterSale(ctx, orderID, caseID, "", "merchant", input)
}

func (s *Service) UpdateGuestAfterSale(ctx context.Context, orderID, caseID, token string, input AfterSaleAction) (AfterSaleCase, error) {
	return s.actAfterSale(ctx, orderID, caseID, token, "customer", input)
}

func normalizeAfterSaleAction(input AfterSaleAction, actor string) (AfterSaleAction, error) {
	if !afterSaleKey(input.Key) || input.ExpectedVersion < 1 {
		return input, ErrAfterSaleInvalid
	}
	var err error
	input.Message, err = afterSaleText(input.Message, 4000, false)
	if err != nil {
		return input, err
	}
	if actor == "customer" {
		if input.Action != "withdraw" && input.Action != "return_tracking" || input.RefundRequestID != "" {
			return input, ErrAfterSaleInvalid
		}
	} else if input.Action != "approve" && input.Action != "reject" && input.Action != "receive_return" && input.Action != "complete" {
		return input, ErrAfterSaleInvalid
	}
	if input.Action == "return_tracking" {
		fields, err := shipmentFields(input.Carrier, input.TrackingNumber, "")
		if err != nil {
			return input, ErrAfterSaleInvalid
		}
		input.Carrier, input.TrackingNumber = fields.Carrier, fields.Number
	} else if input.Carrier != "" || input.TrackingNumber != "" {
		return input, ErrAfterSaleInvalid
	}
	if input.Action == "complete" {
		id, err := uuid.Parse(input.RefundRequestID)
		if err != nil || id.Version() != 7 || id.String() != input.RefundRequestID {
			return input, ErrAfterSaleInvalid
		}
	} else if input.RefundRequestID != "" {
		return input, ErrAfterSaleInvalid
	}
	if (input.Action == "approve" || input.Action == "reject" || input.Action == "receive_return") && input.Message == "" {
		return input, ErrAfterSaleInvalid
	}
	return input, nil
}

func (s *Service) actAfterSale(ctx context.Context, orderPublicID, caseID, token, actor string, input AfterSaleAction) (AfterSaleCase, error) {
	var empty AfterSaleCase
	input, err := normalizeAfterSaleAction(input, actor)
	if err != nil {
		return empty, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return empty, err
	}
	defer func() { _ = tx.Rollback() }()
	o, err := lockShipmentOrder(ctx, tx, orderPublicID)
	if err != nil {
		return empty, err
	}
	if actor == "customer" {
		if err = afterSaleGuest(ctx, tx, o.ID, token); err != nil {
			return empty, err
		}
	}
	c, err := loadAfterSale(ctx, tx, o.ID, caseID)
	if err != nil {
		return empty, err
	}
	var previousHash, previousActor string
	err = tx.QueryRowContext(ctx, `SELECT request_hash,actor FROM after_sale_event WHERE after_sale_id=? AND client_key=?`, c.id, input.Key).Scan(&previousHash, &previousActor)
	hash := afterSaleHash(input)
	if err == nil {
		if previousHash != hash || previousActor != actor {
			return empty, ErrAfterSaleChanged
		}
		if actor == "customer" {
			c.RefundRequestID = ""
		}
		return c.AfterSaleCase, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}
	if c.Version != input.ExpectedVersion {
		return empty, ErrAfterSaleChanged
	}
	if c.Version >= AfterSaleEventLimit {
		return empty, ErrAfterSaleLimit
	}
	// Customer tracking corrections must leave room for merchant receipt,
	// inspection approval and verified refund completion.
	if actor == "customer" && input.Action == "return_tracking" && c.Version >= AfterSaleEventLimit-3 {
		return empty, ErrAfterSaleLimit
	}
	next := ""
	var refundID sql.NullInt64
	instructions := c.ReturnInstructions
	switch input.Action {
	case "withdraw":
		if c.State == "requested" || c.State == "awaiting_return" && c.ReturnTrackingNumber == "" {
			next = "withdrawn"
		}
	case "return_tracking":
		if c.Kind == "return_refund" && c.State == "awaiting_return" {
			next = c.State
		}
	case "reject":
		if c.State == "requested" || c.Kind == "return_refund" && c.State == "returned" {
			next = "rejected"
		}
	case "approve":
		if c.State == "requested" {
			next = "approved"
			if c.Kind == "return_refund" {
				next, instructions = "awaiting_return", input.Message
			}
		} else if c.Kind == "return_refund" && c.State == "returned" {
			next = "approved"
		}
	case "receive_return":
		if c.Kind == "return_refund" && c.State == "awaiting_return" {
			next = "returned" // merchant confirmation, never inferred from tracking
		}
	case "complete":
		if c.State == "approved" {
			// A verified, successful refund intention must match this order, amount
			// and currency, postdate the request, and not already serve another case.
			// A historical/reconciliating or merely pending refund is insufficient.
			err = tx.QueryRowContext(ctx, `SELECT r.id FROM refund_request r
JOIN payment_receipt p ON p.id=r.receipt_id AND p.order_id=r.order_id AND p.provider=r.provider AND p.payment_ref=r.payment_ref AND p.currency=r.currency
JOIN payment_refund f ON f.receipt_id=p.id AND f.provider=r.provider AND f.refund_ref=r.gateway_ref AND f.amount_cents=r.amount_cents
WHERE r.public_id=? AND r.order_id=? AND r.amount_cents=? AND r.currency=? AND r.state='succeeded' AND r.created_at>=?
AND p.refunded_cents>=r.amount_cents
AND NOT EXISTS(SELECT 1 FROM refund_reconciliation rc WHERE rc.receipt_id=p.id AND rc.state='open')
AND NOT EXISTS(SELECT 1 FROM after_sale a WHERE a.refund_request_id=r.id)`, input.RefundRequestID, o.ID, c.AmountCents, c.Currency, c.CreatedAt).Scan(&refundID)
			if errors.Is(err, sql.ErrNoRows) {
				return empty, ErrAfterSaleChanged
			}
			if err != nil {
				return empty, err
			}
			next = "completed"
		}
	}
	if next == "" {
		return empty, ErrAfterSaleChanged
	}
	carrier, tracking := c.ReturnCarrier, c.ReturnTrackingNumber
	message := c.MerchantMessage
	if input.Action == "return_tracking" {
		carrier, tracking = input.Carrier, input.TrackingNumber
	}
	if actor == "merchant" {
		message = input.Message
	}
	_, err = tx.ExecContext(ctx, `UPDATE after_sale SET state=?,version=version+1,merchant_message=?,return_instructions=?,return_carrier=?,return_tracking_number=?,refund_request_id=?,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=?`, next, message, instructions, carrier, tracking, refundID, c.id)
	if err != nil {
		return empty, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO after_sale_event(after_sale_id,version,client_key,request_hash,actor,action,state,message,carrier,tracking_number) VALUES(?,?,?,?,?,?,?,?,?,?)`, c.id, c.Version+1, input.Key, hash, actor, input.Action, next, input.Message, input.Carrier, input.TrackingNumber)
	if err != nil {
		return empty, err
	}
	c, err = loadAfterSale(ctx, tx, o.ID, caseID)
	if err != nil {
		return empty, err
	}
	if actor == "customer" {
		c.RefundRequestID = ""
	}
	return c.AfterSaleCase, tx.Commit()
}
