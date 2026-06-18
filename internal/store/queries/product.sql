-- Product & Option Queries
-- Purpose: product / option axis / option value read & write
-- Author: daxing  Email: 3442535897@qq.com  Time: 2026-06-17 18:13:43
-- NOTE: ASCII-only comments here (sqlc v1.30 multibyte-span bug; see DECISIONS.md).

-- name: CreateProduct :execlastid
INSERT INTO product (public_id, title, slug, description, status) VALUES (?, ?, ?, ?, ?);

-- name: GetProductBySlug :one
SELECT id, public_id, title, slug, description, status, created_at, updated_at FROM product WHERE slug = ? AND deleted_at IS NULL;

-- name: CreateOption :execlastid
INSERT INTO product_option (product_id, name, position) VALUES (?, ?, ?);

-- name: CreateOptionValue :execlastid
INSERT INTO product_option_value (option_id, value, position) VALUES (?, ?, ?);

-- name: ListProducts :many
SELECT id, public_id, title, slug, status, created_at, updated_at FROM product WHERE deleted_at IS NULL ORDER BY id DESC;

-- name: GetProductByPublicID :one
SELECT id, public_id, title, slug, description, status, created_at, updated_at FROM product WHERE public_id = ? AND deleted_at IS NULL;

-- name: UpdateProduct :exec
UPDATE product SET title = ?, description = ?, status = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ? AND deleted_at IS NULL;

-- name: SoftDeleteProduct :exec
UPDATE product SET deleted_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ? AND deleted_at IS NULL;
