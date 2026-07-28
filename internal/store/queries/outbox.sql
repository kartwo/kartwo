-- Email Outbox Queries
-- Purpose: enqueue (idempotent), claim due, mark sending/sent/failed/skipped, boot recovery
-- Author: daxing  Email: 3442535897@qq.com  Time: 2026-07-28 13:27:02
-- NOTE: ASCII-only comments here (sqlc v1.30 multibyte-span bug; see DECISIONS.md).

-- name: EnqueueEmail :execrows
-- Idempotent: UNIQUE(order_id,kind) makes a duplicate enqueue a no-op (PayPal sync capture + webhook backup).
INSERT OR IGNORE INTO email_outbox (order_id, kind, to_addr, subject, body) VALUES (?, ?, ?, ?, ?);

-- name: ClaimDueEmails :many
-- Due pending emails (next_attempt_at reached), oldest first, limited batch. Worker sends outside any tx.
SELECT id, order_id, to_addr, subject, body, attempts FROM email_outbox WHERE status = 'pending' AND next_attempt_at <= ? ORDER BY id LIMIT ?;

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

-- name: ResetStaleSending :exec
-- Boot recovery: any row stuck in 'sending' (crash mid-send) back to pending for re-attempt.
UPDATE email_outbox SET status = 'pending' WHERE status = 'sending';
