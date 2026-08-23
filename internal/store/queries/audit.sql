-- Audit Event Queries
-- Purpose: append and list admin audit events
-- Author: daxing  Email: 3442535897@qq.com  Time: 2026-08-24 00:20:00
-- NOTE: ASCII-only comments here (sqlc v1.30 multibyte-span bug; see DECISIONS.md).

-- name: CreateAuditEvent :exec
INSERT INTO audit_event (public_id, admin_id, action, target_type, target_public_id)
VALUES (?, ?, ?, ?, ?);

-- name: ListAuditEvents :many
SELECT e.public_id, e.action, e.target_type, e.target_public_id, e.created_at,
       a.public_id AS admin_public_id, a.username AS admin_username
FROM audit_event e
JOIN admin_user a ON a.id = e.admin_id
ORDER BY e.created_at DESC, e.id DESC
LIMIT ?;
