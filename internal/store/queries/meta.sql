-- name: GetMeta :one
SELECT key, value, created_at, updated_at FROM meta WHERE key = ?;

-- name: UpsertMeta :exec
INSERT INTO meta (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now');
