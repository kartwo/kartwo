-- Inventory Queries
-- Purpose: per-variant inventory set/read (reserved seeds M2 oversell guard)
-- Author: daxing  Email: 3442535897@qq.com  Time: 2026-06-17 18:13:43
-- NOTE: ASCII-only comments here (sqlc v1.30 multibyte-span bug; see DECISIONS.md).

-- name: SetInventory :exec
INSERT INTO inventory (variant_id, quantity) VALUES (?, ?) ON CONFLICT(variant_id) DO UPDATE SET quantity = excluded.quantity, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now');

-- name: GetInventory :one
SELECT variant_id, quantity, reserved, updated_at FROM inventory WHERE variant_id = ?;
