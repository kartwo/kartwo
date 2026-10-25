// 运单版本服务 / Shipment Revision Service
// 功能：整单发货、运单更正与通知原子入队，使用版本比较防止旧页面覆盖新信息
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 15:35:58
package order

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/kartwo/kartwo/internal/mail"
	"github.com/kartwo/kartwo/internal/store/sqlcgen"
)

var (
	ErrShipmentChanged = errors.New("order: 运单版本已变化，请刷新核对")
	ErrShipmentSending = errors.New("order: 发货通知正在投递，请稍后再核对")
)

type ShipmentFields struct{ Carrier, Number, URL string }
type shipmentOrder struct {
	ID                      int64
	PublicID, Status, Email string
}
type shipmentSnapshot struct {
	ID, Version         int64
	Revision, ShippedAt string
	ShipmentFields
}

func shipmentFields(carrier, number, link string) (ShipmentFields, error) {
	for _, field := range []string{carrier, number, link} {
		if !utf8.ValidString(field) || strings.ContainsFunc(field, func(r rune) bool { return unicode.IsControl(r) || r == '\u2028' || r == '\u2029' }) {
			return ShipmentFields{}, ErrInvalidInfo
		}
	}
	f := ShipmentFields{strings.TrimSpace(carrier), strings.TrimSpace(number), strings.TrimSpace(link)}
	if f.Carrier == "" || f.Number == "" || len(f.Carrier) > 128 || len(f.Number) > 128 || len(f.URL) > 2048 {
		return ShipmentFields{}, ErrInvalidInfo
	}
	if f.URL != "" {
		u, err := url.Parse(f.URL)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || strings.ContainsAny(f.URL, "<>\"'\\") || strings.ContainsFunc(f.URL, unicode.IsSpace) {
			return ShipmentFields{}, ErrInvalidInfo
		}
	}
	return f, nil
}

// Acquire the SQLite writer before reading mutable state, including with more
// than one connection/process. No-op preserves the original order timestamp.
func lockShipmentOrder(ctx context.Context, tx *sql.Tx, publicID string) (shipmentOrder, error) {
	o := shipmentOrder{PublicID: publicID}
	err := tx.QueryRowContext(ctx, `UPDATE "order" SET updated_at=updated_at WHERE public_id=? RETURNING id,status,email`, publicID).Scan(&o.ID, &o.Status, &o.Email)
	return o, err
}

func currentShipment(ctx context.Context, tx *sql.Tx, orderID int64) (shipmentSnapshot, error) {
	var s shipmentSnapshot
	err := tx.QueryRowContext(ctx, `SELECT s.id,s.carrier,s.tracking_number,s.tracking_url,s.shipped_at,COALESCE(r.public_id,''),COALESCE(r.version,0) FROM shipment s LEFT JOIN shipment_revision r ON r.shipment_id=s.id AND r.version=(SELECT MAX(version) FROM shipment_revision WHERE shipment_id=s.id) WHERE s.order_id=?`, orderID).Scan(&s.ID, &s.Carrier, &s.Number, &s.URL, &s.ShippedAt, &s.Revision, &s.Version)
	return s, err
}

// Fulfill commits shipment, immutable revision and notification together. A lost
// response can be replayed with identical details without creating another mail.
func (s *Service) Fulfill(ctx context.Context, publicID, carrier, number, link string) error {
	f, err := shipmentFields(carrier, number, link)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	o, err := lockShipmentOrder(ctx, tx, publicID)
	if err != nil {
		return err
	}
	current, err := currentShipment(ctx, tx, o.ID)
	if err == nil {
		if o.Status == "fulfilled" && current.ShipmentFields == f {
			return nil
		}
		return sql.ErrNoRows
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE "order" SET status='fulfilled',updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=? AND status='paid'
	AND NOT EXISTS(SELECT 1 FROM refund_request WHERE order_id=? AND state IN ('prepared','pending','review'))
	AND NOT EXISTS(SELECT 1 FROM payment_receipt WHERE order_id=? AND review_reason<>'' AND refunded_cents<amount_cents)
	AND NOT EXISTS(SELECT 1 FROM refund_reconciliation rc JOIN payment_receipt p ON p.id=rc.receipt_id WHERE p.order_id=? AND rc.state='open')
	AND (NOT EXISTS(SELECT 1 FROM payment_receipt WHERE order_id=?) OR EXISTS(SELECT 1 FROM payment_receipt WHERE order_id=? AND refunded_cents<amount_cents))`, o.ID, o.ID, o.ID, o.ID, o.ID, o.ID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	var shipmentID int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO shipment(order_id,carrier,tracking_number,tracking_url) VALUES(?,?,?,?) RETURNING id`, o.ID, f.Carrier, f.Number, f.URL).Scan(&shipmentID); err != nil {
		return err
	}
	if _, err = s.addShipmentRevision(ctx, tx, o, shipmentID, 1, f, false, time.Now().UTC().Format(reservationTimeLayout)); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateShipment changes tracking, never payment/inventory/fulfillment facts.
// Empty expectedRevision is valid only for a pre-upgrade shipment with no history.
func (s *Service) UpdateShipment(ctx context.Context, publicID, expectedRevision, carrier, number, link string) error {
	f, err := shipmentFields(carrier, number, link)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	o, err := lockShipmentOrder(ctx, tx, publicID)
	if err != nil {
		return err
	}
	current, err := currentShipment(ctx, tx, o.ID)
	if err != nil {
		return err
	}
	// An identical replay after a committed update is safe, while an older page
	// with different content must never replace the current revision.
	if current.Revision != expectedRevision {
		if current.ShipmentFields == f {
			return nil
		}
		return ErrShipmentChanged
	}
	if current.ShipmentFields == f {
		return nil
	}
	var sending bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM shipment_notice n JOIN shipment_revision r ON r.id=n.revision_id JOIN email_outbox e ON e.id=n.outbox_id WHERE r.shipment_id=? AND e.status='sending')`, current.ID).Scan(&sending); err != nil {
		return err
	}
	if sending {
		return ErrShipmentSending
	}
	if current.Version == 0 {
		// Capture legacy tracking only when the merchant explicitly edits it.
		// Reading/upgrading alone never creates a notice or resends old mail.
		if _, err = s.addShipmentRevision(ctx, tx, o, current.ID, 1, current.ShipmentFields, true, current.ShippedAt); err != nil {
			return err
		}
		current.Version = 1
	}
	if _, err = tx.ExecContext(ctx, `UPDATE email_outbox SET status='superseded',last_error='shipment superseded' WHERE status='pending' AND id IN(SELECT n.outbox_id FROM shipment_notice n JOIN shipment_revision r ON r.id=n.revision_id WHERE r.shipment_id=?)`, current.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE shipment_notice SET resolution='superseded',resolved_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE resolution='' AND revision_id IN(SELECT id FROM shipment_revision WHERE shipment_id=?)`, current.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE shipment SET carrier=?,tracking_number=?,tracking_url=? WHERE id=?`, f.Carrier, f.Number, f.URL, current.ID); err != nil {
		return err
	}
	if _, err = s.addShipmentRevision(ctx, tx, o, current.ID, current.Version+1, f, false, time.Now().UTC().Format(reservationTimeLayout)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) addShipmentRevision(ctx context.Context, tx *sql.Tx, o shipmentOrder, shipmentID, version int64, f ShipmentFields, legacy bool, created string) (int64, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return 0, err
	}
	var revisionID int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO shipment_revision(public_id,shipment_id,version,carrier,tracking_number,tracking_url,legacy,created_at) VALUES(?,?,?,?,?,?,?,?) RETURNING id`, id.String(), shipmentID, version, f.Carrier, f.Number, f.URL, legacy, created).Scan(&revisionID); err != nil {
		return 0, err
	}
	if !legacy {
		origin := "shipped"
		if version > 1 {
			origin = "tracking_updated"
		}
		_, err = s.addShipmentNotice(ctx, tx, o, revisionID, version, 1, f, created, "initial", origin)
	}
	return revisionID, err
}

func (s *Service) addShipmentNotice(ctx context.Context, tx *sql.Tx, o shipmentOrder, revisionID, version, sequence int64, f ShipmentFields, changed, requestKey, origin string) (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	outboxID, err := mail.EnqueueShipment(ctx, sqlcgen.New(tx), mail.ShipmentInfo{OrderID: o.ID, Version: version, OrderPublicID: o.PublicID, NoticeID: id.String(), Email: o.Email, Carrier: f.Carrier, Number: f.Number, URL: f.URL, ChangedAt: changed})
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO shipment_notice(public_id,revision_id,sequence,request_key,origin,outbox_id) VALUES(?,?,?,?,?,?)`, id.String(), revisionID, sequence, requestKey, origin, outboxID)
	return id.String(), err
}
