// 基础售后服务 / After Sale Service
// 功能：凭证限定申请、政策快照、审核状态机及版本幂等；不自动退款、发信或回补库存
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 20:55:43
package order

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/kartwo/kartwo/internal/policy"
)

const AfterSaleLimit = 20
const AfterSaleEventLimit = 50

var (
	ErrAfterSaleInvalid    = errors.New("order: 售后资料无效")
	ErrAfterSaleChanged    = errors.New("order: 售后状态或请求已变化")
	ErrAfterSaleIneligible = errors.New("order: 当前订单不能新建此售后申请，请联系店铺")
	ErrAfterSaleLimit      = errors.New("order: 售后记录达到上限，请联系店铺")
)

type AfterSaleInput struct {
	Key, Kind, Reason string
	AmountCents       int64
}

type AfterSaleTerms struct {
	Configured    bool   `json:"configured"`
	WindowDays    int    `json:"return_window_days"`
	ShippingPayer string `json:"return_shipping_payer"`
	SupportEmail  string `json:"support_email"`
}

type AfterSaleCase struct {
	PublicID             string           `json:"public_id"`
	Kind                 string           `json:"kind"`
	State                string           `json:"state"`
	Version              int64            `json:"version"`
	AmountCents          int64            `json:"amount_cents"`
	Currency             string           `json:"currency"`
	Reason               string           `json:"reason"`
	Terms                AfterSaleTerms   `json:"terms"`
	MerchantMessage      string           `json:"merchant_message"`
	ReturnInstructions   string           `json:"return_instructions"`
	ReturnCarrier        string           `json:"return_carrier"`
	ReturnTrackingNumber string           `json:"return_tracking_number"`
	RefundRequestID      string           `json:"refund_request_id,omitempty"`
	RefundNeedsReview    bool             `json:"refund_needs_review"`
	CreatedAt            string           `json:"created_at"`
	UpdatedAt            string           `json:"updated_at"`
	Events               []AfterSaleEvent `json:"events"`
}

type AfterSaleEvent struct {
	Version        int64  `json:"version"`
	Actor          string `json:"actor"`
	Action         string `json:"action"`
	State          string `json:"state"`
	Message        string `json:"message"`
	Carrier        string `json:"carrier"`
	TrackingNumber string `json:"tracking_number"`
	CreatedAt      string `json:"created_at"`
}

type AfterSaleSnapshot struct {
	Cases            []AfterSaleCase `json:"cases"`
	MaxRequestCents  int64           `json:"max_request_cents"`
	CanRequestRefund bool            `json:"can_request_refund"`
	CanRequestReturn bool            `json:"can_request_return"`
}

type AfterSaleAction struct {
	Key             string
	ExpectedVersion int64
	Action          string
	Message         string
	Carrier         string
	TrackingNumber  string
	RefundRequestID string
}

type afterSaleRecord struct {
	AfterSaleCase
	id        int64
	clientKey string
}

func afterSaleText(value string, max int, required bool) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n"))
	if !utf8.ValidString(value) || len(value) > max || required && value == "" || strings.ContainsFunc(value, func(r rune) bool {
		return (unicode.IsControl(r) && r != '\n' && r != '\t') || r == '\u2028' || r == '\u2029'
	}) {
		return "", ErrAfterSaleInvalid
	}
	return value, nil
}

func afterSaleKey(key string) bool {
	id, err := uuid.Parse(key)
	return err == nil && id.Version() == 7 && id.String() == key
}

// Validate the guest grant in the transaction after acquiring the writer. A
// revoked/expired grant cannot replay a previous successful request either.
func afterSaleGuest(ctx context.Context, tx *sql.Tx, orderID int64, token string) error {
	digest := guestTokenHash(token)
	if digest == "" {
		return ErrGuestVerification
	}
	var allowed bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM order_guest_access WHERE order_id=? AND token_hash=? AND expires_at>?)`, orderID, digest, time.Now().Unix()).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrGuestVerification
	}
	return nil
}

func afterSaleHash(value any) string {
	b, _ := json.Marshal(value) // fixed structs below contain only strings/int64
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// remainingAfterSale caps requests at the order value and verified remaining
// receipts. Duplicate/late receipts never increase the customer's order value.
// Legacy paid orders without a receipt remain reviewable, never auto-refunded.
func remainingAfterSale(ctx context.Context, tx *sql.Tx, o shipmentOrder) (int64, bool, error) {
	var total int64
	var shipped bool
	if err := tx.QueryRowContext(ctx, `SELECT total_cents,EXISTS(SELECT 1 FROM shipment WHERE order_id=?) FROM "order" WHERE id=?`, o.ID, o.ID).Scan(&total, &shipped); err != nil {
		return 0, false, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT amount_cents,refunded_cents FROM payment_receipt WHERE order_id=?`, o.ID)
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = rows.Close() }()
	var remaining int64
	var recorded bool
	for rows.Next() {
		var paid, refunded int64
		if err := rows.Scan(&paid, &refunded); err != nil {
			return 0, false, err
		}
		recorded = true
		net := paid - refunded
		// Clamp before adding to avoid overflow even with many duplicate payments.
		if net > 0 && total > remaining {
			remaining += min(net, total-remaining)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, false, err
	}
	if !recorded && (o.Status == "paid" || o.Status == "fulfilled") {
		remaining = total
	}
	return max(remaining, 0), shipped, nil
}

func (s *Service) CreateAfterSale(ctx context.Context, orderPublicID, token string, input AfterSaleInput) (AfterSaleCase, error) {
	var empty AfterSaleCase
	var err error
	input.Reason, err = afterSaleText(input.Reason, 2000, true)
	if err != nil || !afterSaleKey(input.Key) || input.AmountCents <= 0 || (input.Kind != "refund" && input.Kind != "return_refund") {
		return empty, ErrAfterSaleInvalid
	}
	profile, configured, err := policy.LoadProfile(ctx, s.settings)
	if err != nil {
		return empty, err
	}
	terms := AfterSaleTerms{Configured: configured}
	if configured {
		terms.WindowDays, terms.ShippingPayer, terms.SupportEmail = profile.ReturnWindowDays, profile.ReturnShippingPayer, profile.SupportEmail
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
	if err = afterSaleGuest(ctx, tx, o.ID, token); err != nil {
		return empty, err
	}
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT public_id FROM after_sale WHERE order_id=? AND client_key=?`, o.ID, input.Key).Scan(&existing)
	if err == nil {
		record, err := loadAfterSale(ctx, tx, o.ID, existing)
		if err != nil {
			return empty, err
		}
		if record.Kind != input.Kind || record.Reason != input.Reason || record.AmountCents != input.AmountCents {
			return empty, ErrAfterSaleChanged
		}
		record.RefundRequestID = ""
		return record.AfterSaleCase, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}
	var count, active int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(state IN ('requested','awaiting_return','returned','approved')),0) FROM after_sale WHERE order_id=?`, o.ID).Scan(&count, &active); err != nil {
		return empty, err
	}
	if active > 0 {
		return empty, ErrAfterSaleChanged
	}
	if count >= AfterSaleLimit {
		return empty, ErrAfterSaleLimit
	}
	remaining, shipped, err := remainingAfterSale(ctx, tx, o)
	if err != nil {
		return empty, err
	}
	if input.AmountCents > remaining || input.Kind == "return_refund" && !shipped {
		return empty, ErrAfterSaleIneligible
	}
	var currency string
	if err = tx.QueryRowContext(ctx, `SELECT currency FROM "order" WHERE id=?`, o.ID).Scan(&currency); err != nil {
		return empty, err
	}
	publicID := uuid.Must(uuid.NewV7()).String()
	var caseID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO after_sale(public_id,order_id,client_key,kind,amount_cents,currency,reason,policy_configured,return_window_days,return_shipping_payer,support_email) VALUES(?,?,?,?,?,?,?,?,?,?,?) RETURNING id`, publicID, o.ID, input.Key, input.Kind, input.AmountCents, currency, input.Reason, terms.Configured, terms.WindowDays, terms.ShippingPayer, terms.SupportEmail).Scan(&caseID)
	if err != nil {
		return empty, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO after_sale_event(after_sale_id,version,client_key,request_hash,actor,action,state,message) VALUES(?,1,?,?,'customer','request','requested',?)`, caseID, input.Key, afterSaleHash(input), input.Reason); err != nil {
		return empty, err
	}
	record, err := loadAfterSale(ctx, tx, o.ID, publicID)
	if err != nil {
		return empty, err
	}
	return record.AfterSaleCase, tx.Commit()
}

func loadAfterSale(ctx context.Context, tx *sql.Tx, orderID int64, publicID string) (afterSaleRecord, error) {
	var c afterSaleRecord
	err := tx.QueryRowContext(ctx, `SELECT a.id,a.public_id,a.client_key,a.kind,a.state,a.version,a.amount_cents,a.currency,a.reason,a.policy_configured,a.return_window_days,a.return_shipping_payer,a.support_email,a.merchant_message,a.return_instructions,a.return_carrier,a.return_tracking_number,COALESCE(r.public_id,''),a.created_at,a.updated_at,
CAST(a.state='completed' AND (r.id IS NULL OR r.state<>'succeeded' OR r.order_id<>a.order_id OR r.currency<>a.currency OR r.amount_cents<>a.amount_cents
OR NOT EXISTS(SELECT 1 FROM payment_receipt p WHERE p.id=r.receipt_id AND p.order_id=a.order_id AND p.currency=a.currency AND p.provider=r.provider AND p.payment_ref=r.payment_ref AND p.refunded_cents>=a.amount_cents)
OR EXISTS(SELECT 1 FROM refund_reconciliation rc WHERE rc.receipt_id=r.receipt_id AND rc.state='open')
OR NOT EXISTS(SELECT 1 FROM payment_refund f WHERE f.receipt_id=r.receipt_id AND f.provider=r.provider AND f.refund_ref=r.gateway_ref AND f.amount_cents=a.amount_cents)) AS INTEGER)
FROM after_sale a LEFT JOIN refund_request r ON r.id=a.refund_request_id WHERE a.order_id=? AND a.public_id=?`, orderID, publicID).Scan(&c.id, &c.PublicID, &c.clientKey, &c.Kind, &c.State, &c.Version, &c.AmountCents, &c.Currency, &c.Reason, &c.Terms.Configured, &c.Terms.WindowDays, &c.Terms.ShippingPayer, &c.Terms.SupportEmail, &c.MerchantMessage, &c.ReturnInstructions, &c.ReturnCarrier, &c.ReturnTrackingNumber, &c.RefundRequestID, &c.CreatedAt, &c.UpdatedAt, &c.RefundNeedsReview)
	if err != nil {
		return c, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT version,actor,action,state,message,carrier,tracking_number,created_at FROM after_sale_event WHERE after_sale_id=? ORDER BY version LIMIT ?`, c.id, AfterSaleEventLimit)
	if err != nil {
		return c, err
	}
	defer func() { _ = rows.Close() }()
	c.Events = []AfterSaleEvent{}
	for rows.Next() {
		var e AfterSaleEvent
		if err = rows.Scan(&e.Version, &e.Actor, &e.Action, &e.State, &e.Message, &e.Carrier, &e.TrackingNumber, &e.CreatedAt); err != nil {
			return c, err
		}
		c.Events = append(c.Events, e)
	}
	return c, rows.Err()
}

// AdminAfterSales is a private projection: the HTTP layer must enforce order
// viewing plus customer-data permission. No raw provider reference is included.
func (s *Service) AdminAfterSales(ctx context.Context, orderPublicID string) (AfterSaleSnapshot, error) {
	return s.afterSales(ctx, orderPublicID, "", false)
}

func (s *Service) GuestAfterSales(ctx context.Context, orderPublicID, token string) (AfterSaleSnapshot, error) {
	return s.afterSales(ctx, orderPublicID, token, true)
}

func (s *Service) afterSales(ctx context.Context, orderPublicID, token string, guest bool) (AfterSaleSnapshot, error) {
	out := AfterSaleSnapshot{Cases: []AfterSaleCase{}}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback() }()
	o := shipmentOrder{PublicID: orderPublicID}
	if err = tx.QueryRowContext(ctx, `SELECT id,status,email FROM "order" WHERE public_id=?`, orderPublicID).Scan(&o.ID, &o.Status, &o.Email); err != nil {
		return out, err
	}
	if guest {
		if err = afterSaleGuest(ctx, tx, o.ID, token); err != nil {
			return out, err
		}
	}
	remaining, shipped, err := remainingAfterSale(ctx, tx, o)
	if err != nil {
		return out, err
	}
	// The helper closes rows before loading cases; one connection is sufficient.
	ids, err := afterSaleIDs(ctx, tx, o.ID)
	if err != nil {
		return out, err
	}
	var active bool
	for _, id := range ids {
		c, err := loadAfterSale(ctx, tx, o.ID, id)
		if err != nil {
			return out, err
		}
		active = active || c.State == "requested" || c.State == "awaiting_return" || c.State == "returned" || c.State == "approved"
		if guest {
			c.RefundRequestID = ""
		}
		out.Cases = append(out.Cases, c.AfterSaleCase)
	}
	out.MaxRequestCents = remaining
	out.CanRequestRefund = !active && len(ids) < AfterSaleLimit && remaining > 0
	out.CanRequestReturn = out.CanRequestRefund && shipped
	return out, tx.Commit()
}

func afterSaleIDs(ctx context.Context, tx *sql.Tx, orderID int64) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT public_id FROM after_sale WHERE order_id=? ORDER BY id DESC LIMIT ?`, orderID, AfterSaleLimit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
