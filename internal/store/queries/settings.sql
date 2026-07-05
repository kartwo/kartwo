-- Settings Queries
-- Purpose: key-value settings (plain or KEK-encrypted)
-- Author: daxing  Email: 3442535897@qq.com  Time: 2026-06-19 21:22:05
-- NOTE: ASCII-only comments here (sqlc v1.30 multibyte-span bug; see DECISIONS.md).

-- name: UpsertSetting :exec
INSERT INTO setting (key, value, encrypted) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value, encrypted = excluded.encrypted, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now');

-- name: GetSetting :one
SELECT value, encrypted FROM setting WHERE key = ?;
