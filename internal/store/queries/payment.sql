-- Payment Queries
-- Purpose: webhook idempotency (dedup insert) and order pending->paid transition
-- Author: daxing  Email: 3442535897@qq.com  Time: 2026-06-20 20:26:08
-- NOTE: ASCII-only comments here (sqlc v1.30 multibyte-span bug; see DECISIONS.md).

-- name: InsertWebhookEvent :exec
INSERT INTO webhook_event (provider, event_id, event_type, order_ref) VALUES (?, ?, ?, ?);

-- name: MarkOrderPaidByPublicID :execrows
UPDATE "order" SET status = 'paid', payment_provider = ?, payment_ref = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id = ? AND status = 'pending';

-- name: MarkOrderRefundedByPublicID :execrows
UPDATE "order" SET status = 'refunded', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id = ? AND status = 'paid';

-- name: MarkOrderRefundedByPaymentRef :execrows
UPDATE "order" SET status = 'refunded', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE payment_ref = ? AND status = 'paid';

-- name: InsertRefund :exec
INSERT INTO refund (order_id, provider, provider_refund_id, amount_cents) VALUES (?, ?, ?, ?);
