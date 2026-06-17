-- Admin & Session Queries
-- Purpose: admin user + server-side session read/write
-- Author: daxing  Email: 3442535897@qq.com  Time: 2026-06-17 23:18:17
-- NOTE: ASCII-only comments here (sqlc v1.30 multibyte-span bug; see DECISIONS.md).

-- name: CountAdminUsers :one
SELECT COUNT(*) FROM admin_user;

-- name: CreateAdminUser :execlastid
INSERT INTO admin_user (public_id, username, password_hash) VALUES (?, ?, ?);

-- name: GetAdminUserByUsername :one
SELECT id, public_id, username, password_hash FROM admin_user WHERE username = ?;

-- name: CreateSession :exec
INSERT INTO session (token, admin_id, csrf_token, expires_at) VALUES (?, ?, ?, ?);

-- name: GetSessionByToken :one
SELECT s.token, s.admin_id, s.csrf_token, s.expires_at, a.username, a.public_id FROM session s JOIN admin_user a ON a.id = s.admin_id WHERE s.token = ? AND s.expires_at > ?;

-- name: DeleteSession :exec
DELETE FROM session WHERE token = ?;

-- name: DeleteSessionsByAdmin :exec
DELETE FROM session WHERE admin_id = ?;

-- name: DeleteExpiredSessions :exec
DELETE FROM session WHERE expires_at <= ?;
