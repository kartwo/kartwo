-- Storefront Queries
-- Purpose: public storefront reads (active products only)
-- Author: daxing  Email: 3442535897@qq.com  Time: 2026-06-18 11:20:00
-- NOTE: ASCII-only comments here (sqlc v1.30 multibyte-span bug; see DECISIONS.md).

-- name: ListActiveProducts :many
SELECT id, public_id, title, slug, description, updated_at FROM product WHERE status = 'active' AND deleted_at IS NULL ORDER BY id DESC;

-- name: GetActiveProductBySlug :one
SELECT id, public_id, title, slug, description, updated_at FROM product WHERE slug = ? AND status = 'active' AND deleted_at IS NULL;
