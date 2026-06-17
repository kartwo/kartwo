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
