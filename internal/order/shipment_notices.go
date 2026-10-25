// 发货通知核对 / Shipment Notice Recovery
// 功能：有界运单与邮件历史、原请求幂等重试；保留未知投递证据且不自动重发
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 15:39:07
package order

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"time"
)

var (
	ErrShipmentNoticeChanged  = errors.New("order: 发货通知已变化，请刷新核对")
	ErrShipmentNoticeCooldown = errors.New("order: 请至少间隔一分钟再重试发货通知")
	shipmentRequestKey        = regexp.MustCompile(`^[a-zA-Z0-9_-]{16,128}$`)
)

type ShipmentRevision struct {
	PublicID string
	Version  int64
	ShipmentFields
	Legacy    bool
	CreatedAt string
}
type ShipmentNotice struct {
	PublicID, RevisionID                                                             string
	Version, Sequence, Attempts                                                      int64
	Status, Origin, Resolution, Reason, CreatedAt, NextAttemptAt, SentAt, ResolvedAt string
}
type ShipmentHistory struct {
	HasShipment                bool
	CurrentRevision            string
	CurrentVersion             int64
	ShippedAt                  string
	RevisionCount, NoticeCount int64
	Revisions                  []ShipmentRevision
	Notices                    []ShipmentNotice
}

// ShipmentHistory reads one consistent snapshot, bounded to 20 revisions and 50
// notices. Counts explicitly expose truncation. No recipient/body/SMTP error or
// internal row ID is returned. The HTTP caller must still enforce customer data.
func (s *Service) ShipmentHistory(ctx context.Context, publicID string) (ShipmentHistory, error) {
	out := ShipmentHistory{Revisions: []ShipmentRevision{}, Notices: []ShipmentNotice{}}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback() }()
	var orderID int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM "order" WHERE public_id=?`, publicID).Scan(&orderID); err != nil {
		return out, err
	}
	current, err := currentShipment(ctx, tx, orderID)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.HasShipment = true
	out.CurrentRevision = current.Revision
	out.CurrentVersion = current.Version
	out.ShippedAt = current.ShippedAt
	if out.Revisions, err = loadShipmentRevisions(ctx, tx, current.ID); err != nil {
		return out, err
	}
	if out.Notices, err = loadShipmentNotices(ctx, tx, current.ID); err != nil {
		return out, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM shipment_revision WHERE shipment_id=?),(SELECT COUNT(*) FROM shipment_notice n JOIN shipment_revision r ON r.id=n.revision_id WHERE r.shipment_id=?)`, current.ID, current.ID).Scan(&out.RevisionCount, &out.NoticeCount); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func loadShipmentRevisions(ctx context.Context, tx *sql.Tx, shipmentID int64) ([]ShipmentRevision, error) {
	rows, err := tx.QueryContext(ctx, `SELECT public_id,version,carrier,tracking_number,tracking_url,legacy,created_at FROM shipment_revision WHERE shipment_id=? ORDER BY version DESC LIMIT 20`, shipmentID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []ShipmentRevision{}
	for rows.Next() {
		var r ShipmentRevision
		if err = rows.Scan(&r.PublicID, &r.Version, &r.Carrier, &r.Number, &r.URL, &r.Legacy, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func loadShipmentNotices(ctx context.Context, tx *sql.Tx, shipmentID int64) ([]ShipmentNotice, error) {
	rows, err := tx.QueryContext(ctx, `SELECT n.public_id,r.public_id,r.version,n.sequence,e.attempts,e.status,n.origin,n.resolution,e.last_error,n.created_at,e.next_attempt_at,COALESCE(e.sent_at,''),n.resolved_at FROM shipment_notice n JOIN shipment_revision r ON r.id=n.revision_id JOIN email_outbox e ON e.id=n.outbox_id WHERE r.shipment_id=? ORDER BY r.version DESC,n.sequence DESC LIMIT 50`, shipmentID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []ShipmentNotice{}
	for rows.Next() {
		var n ShipmentNotice
		var savedError string
		if err = rows.Scan(&n.PublicID, &n.RevisionID, &n.Version, &n.Sequence, &n.Attempts, &n.Status, &n.Origin, &n.Resolution, &savedError, &n.CreatedAt, &n.NextAttemptAt, &n.SentAt, &n.ResolvedAt); err != nil {
			return nil, err
		}
		n.Reason = shipmentMailReason(savedError)
		out = append(out, n)
	}
	return out, rows.Err()
}

func shipmentMailReason(saved string) string {
	switch saved {
	case "":
		return ""
	case "SMTP 未配置":
		return "smtp_unconfigured"
	case "shipment superseded":
		return "shipment_superseded"
	case "email delivery result unknown", "delivery interrupted":
		return "delivery_unknown"
	case "shipment status unavailable":
		return "status_unavailable"
	default:
		return "delivery_failed"
	}
}

type shipmentNoticeTarget struct {
	ID, RevisionID, Version, Sequence                                  int64
	ShipmentID                                                         int64
	Fields                                                             ShipmentFields
	ChangedAt, Status, Resolution, ResolutionKey, CreatedAt, Recipient string
}

// ManageShipmentNotice accepts only a current notice. "resend_after_check" is
// an explicit merchant acknowledgement of the duplicate-delivery risk; unknown
// SMTP status remains unchanged in the old evidence. "received" does not invent
// an SMTP success or overwrite timestamps. Each retry gets a separate outbox row.
func (s *Service) ManageShipmentNotice(ctx context.Context, publicID, noticeID, requestKey, action string) (string, error) {
	if !shipmentRequestKey.MatchString(requestKey) || (action != "retry" && action != "resend_after_check" && action != "received") {
		return "", ErrInvalidInfo
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	o, err := lockShipmentOrder(ctx, tx, publicID)
	if err != nil {
		return "", err
	}
	var n shipmentNoticeTarget
	err = tx.QueryRowContext(ctx, `SELECT n.id,r.id,r.version,n.sequence,r.shipment_id,r.carrier,r.tracking_number,r.tracking_url,r.created_at,e.status,n.resolution,n.resolution_key,n.created_at,e.to_addr FROM shipment_notice n JOIN shipment_revision r ON r.id=n.revision_id JOIN shipment s ON s.id=r.shipment_id JOIN email_outbox e ON e.id=n.outbox_id WHERE n.public_id=? AND s.order_id=? AND e.order_id=s.order_id AND e.kind='shipment:'||n.public_id`, noticeID, o.ID).Scan(&n.ID, &n.RevisionID, &n.Version, &n.Sequence, &n.ShipmentID, &n.Fields.Carrier, &n.Fields.Number, &n.Fields.URL, &n.ChangedAt, &n.Status, &n.Resolution, &n.ResolutionKey, &n.CreatedAt, &n.Recipient)
	if err != nil {
		return "", err
	}
	var replay, origin string
	var previous sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT public_id,origin,previous_id FROM shipment_notice WHERE revision_id=? AND request_key=?`, n.RevisionID, requestKey).Scan(&replay, &origin, &previous)
	if err == nil {
		if previous.Valid && previous.Int64 == n.ID && origin == action {
			return replay, nil
		}
		return "", ErrShipmentNoticeChanged
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if action == "received" && n.Resolution == "received" && n.ResolutionKey == requestKey {
		return noticeID, nil
	}
	var current bool
	if err = tx.QueryRowContext(ctx, `SELECT NOT EXISTS(SELECT 1 FROM shipment_revision WHERE shipment_id=? AND version>?) AND NOT EXISTS(SELECT 1 FROM shipment_notice WHERE revision_id=? AND sequence>?)`, n.ShipmentID, n.Version, n.RevisionID, n.Sequence).Scan(&current); err != nil {
		return "", err
	}
	if !current || n.Resolution != "" {
		return "", ErrShipmentNoticeChanged
	}
	if n.Status == "sending" {
		return "", ErrShipmentSending
	}
	if action == "received" {
		if n.Status != "delivery_unknown" {
			return "", ErrShipmentNoticeChanged
		}
		_, err = tx.ExecContext(ctx, `UPDATE shipment_notice SET resolution='received',resolution_key=?,resolved_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=? AND resolution=''`, requestKey, n.ID)
		if err != nil {
			return "", err
		}
		return noticeID, tx.Commit()
	}
	resolution := "retried"
	if action == "resend_after_check" {
		if n.Status != "delivery_unknown" {
			return "", ErrShipmentNoticeChanged
		}
		resolution = "not_received"
	} else if n.Status != "failed" && n.Status != "skipped" {
		return "", ErrShipmentNoticeChanged
	}
	created, err := time.Parse(reservationTimeLayout, n.CreatedAt)
	if err != nil {
		return "", ErrShipmentNoticeChanged
	}
	if time.Now().Before(created.Add(time.Minute)) {
		return "", ErrShipmentNoticeCooldown
	}
	if _, err = tx.ExecContext(ctx, `UPDATE shipment_notice SET resolution=?,resolution_key=?,resolved_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=? AND resolution=''`, resolution, requestKey, n.ID); err != nil {
		return "", err
	}
	// Retrying one immutable notification must not silently target an email
	// address that was changed after the original notification was prepared.
	o.Email = n.Recipient
	newID, err := s.addShipmentNotice(ctx, tx, o, n.RevisionID, n.Version, n.Sequence+1, n.Fields, n.ChangedAt, requestKey, action)
	if err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE shipment_notice SET previous_id=? WHERE public_id=?`, n.ID, newID); err != nil {
		return "", err
	}
	return newID, tx.Commit()
}
