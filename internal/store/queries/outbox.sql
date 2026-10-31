-- Email Outbox Queries
-- Purpose: idempotent enqueue, bounded retry and hold uncertain/interrupted deliveries
-- Author: daxing  Email: 3442535897@qq.com  Time: 2026-07-28 13:27:02
-- NOTE: ASCII-only comments here (sqlc v1.30 multibyte-span bug; see DECISIONS.md).

-- name: EnqueueEmail :execrows
-- Idempotent: UNIQUE(order_id,kind) makes a duplicate enqueue a no-op (PayPal sync capture + webhook backup).
INSERT OR IGNORE INTO email_outbox (order_id, kind, to_addr, subject, body) VALUES (?, ?, ?, ?, ?);

-- name: EnqueueShipmentEmail :execlastid
-- Each immutable notice has its own kind; duplicate intent is handled before insertion.
INSERT INTO email_outbox (order_id,kind,to_addr,subject,body) VALUES (?,?,?,?,?);

-- name: ShipmentEmailCurrent :one
-- Snapshot may have become stale between listing and claiming. Never send an obsolete revision/copy.
SELECT CAST(EXISTS(
 SELECT 1 FROM shipment_notice n JOIN shipment_revision r ON r.id=n.revision_id
 JOIN shipment s ON s.id=r.shipment_id JOIN email_outbox e ON e.id=n.outbox_id
 WHERE e.id=? AND e.order_id=s.order_id AND e.kind='shipment:'||n.public_id AND n.resolution=''
 AND s.carrier=r.carrier AND s.tracking_number=r.tracking_number AND s.tracking_url=r.tracking_url
 AND NOT EXISTS(SELECT 1 FROM shipment_revision newer WHERE newer.shipment_id=r.shipment_id AND newer.version>r.version)
 AND NOT EXISTS(SELECT 1 FROM shipment_notice newer WHERE newer.revision_id=r.id AND newer.sequence>n.sequence)
) AS INTEGER);

-- name: MarkEmailSuperseded :exec
UPDATE email_outbox SET status='superseded',last_error='shipment superseded' WHERE id=? AND status='sending';

-- name: MarkShipmentClaimTime :exec
UPDATE shipment_notice SET sending_at=? WHERE outbox_id=?;

-- name: HoldStaleShipmentEmails :exec
-- SMTP dial+protocol is bounded by 40s; a 2m claim cannot be presumed undelivered.
UPDATE email_outbox SET status='delivery_unknown',attempts=attempts+1,last_error='delivery interrupted'
WHERE status='sending' AND id IN(SELECT outbox_id FROM shipment_notice WHERE sending_at<=?);

-- name: ClaimDueEmails :many
-- Due pending emails (next_attempt_at reached), oldest first, limited batch. Worker sends outside any tx.
SELECT id, order_id, kind, to_addr, subject, body, attempts FROM email_outbox WHERE status = 'pending' AND next_attempt_at <= ? ORDER BY id LIMIT ?;

-- name: MarkEmailSending :execrows
-- Claim one row pending->sending (rows=1 confirms claim). Quick single-statement; no network I/O held.
UPDATE email_outbox SET status = 'sending' WHERE id = ? AND status = 'pending';

-- name: MarkEmailSent :exec
UPDATE email_outbox SET status = 'sent', sent_at = strftime('%Y-%m-%dT%H:%M:%fZ','now'), last_error = '' WHERE id = ?;

-- name: MarkEmailRetry :exec
-- Send failed but attempts left: back to pending with backoff + bumped attempts + last error.
UPDATE email_outbox SET status = 'pending', attempts = ?, next_attempt_at = ?, last_error = ? WHERE id = ?;

-- name: MarkEmailFailed :exec
-- Send failed and attempts exhausted: dead-letter.
UPDATE email_outbox SET status = 'failed', attempts = ?, last_error = ? WHERE id = ?;

-- name: MarkEmailSkipped :exec
-- SMTP not configured: skip (no retry, no backfill on later config).
UPDATE email_outbox SET status = 'skipped', last_error = ? WHERE id = ?;

-- name: MarkEmailUncertain :exec
UPDATE email_outbox SET status = 'delivery_unknown', attempts = ?, last_error = ? WHERE id = ? AND status = 'sending';

-- name: HoldInterruptedEmails :exec
-- A crash cannot prove that SMTP rejected DATA. Do not blindly resend.
UPDATE email_outbox SET status = 'delivery_unknown', attempts = attempts + 1, last_error = 'delivery interrupted' WHERE status = 'sending';
