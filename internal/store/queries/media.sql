-- Media Queries
-- Purpose: media asset + derivative read/write, orphan cleanup
-- Author: daxing  Email: 3442535897@qq.com  Time: 2026-06-18 09:48:36
-- NOTE: ASCII-only comments here (sqlc v1.30 multibyte-span bug; see DECISIONS.md).

-- name: CreateMediaAsset :execlastid
INSERT INTO media_asset (public_id, product_id, content_hash, original_path, mime, width, height, size_bytes, position) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: AddDerivative :exec
INSERT INTO media_derivative (asset_id, label, path, format, width, height, size_bytes) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: CountMediaByProduct :one
SELECT COUNT(*) FROM media_asset WHERE product_id = ? AND deleted_at IS NULL;

-- name: ListMediaByProduct :many
SELECT id, public_id, content_hash, original_path, mime, width, height, size_bytes, position FROM media_asset WHERE product_id = ? AND deleted_at IS NULL ORDER BY position, id;

-- name: ListDerivativesByAsset :many
SELECT label, path, format, width, height, size_bytes FROM media_derivative WHERE asset_id = ? ORDER BY width;

-- name: GetMediaByPublicID :one
SELECT id, public_id, product_id, content_hash, original_path FROM media_asset WHERE public_id = ? AND deleted_at IS NULL;

-- name: SoftDeleteMedia :exec
UPDATE media_asset SET deleted_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ? AND deleted_at IS NULL;

-- name: ListOrphanMedia :many
SELECT m.id, m.original_path FROM media_asset m JOIN product p ON p.id = m.product_id WHERE m.deleted_at IS NOT NULL OR p.deleted_at IS NOT NULL;

-- name: ListDerivativePathsByAsset :many
SELECT path FROM media_derivative WHERE asset_id = ?;

-- name: HardDeleteMedia :exec
DELETE FROM media_asset WHERE id = ?;
