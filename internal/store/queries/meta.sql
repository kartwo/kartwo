-- Meta Queries
-- Purpose: meta table read/write; sqlc selection validation sample
-- Author: daxing  Email: 3442535897@qq.com  Time: 2026-06-17 18:13:43
-- NOTE: ASCII-only comments in this dir (sqlc v1.30 mis-slices query spans on
-- multibyte comment bytes; see DECISIONS.md / CONVENTIONS.md).

-- name: GetMeta :one
SELECT key, value, created_at, updated_at FROM meta WHERE key = ?;

-- name: UpsertMeta :exec
INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now');
