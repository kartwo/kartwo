-- Storefront Queries
-- Purpose: public storefront reads (active products only)
-- Author: daxing  Email: 3442535897@qq.com  Time: 2026-06-18 11:20:00
-- NOTE: ASCII-only comments here (sqlc v1.30 multibyte-span bug; see DECISIONS.md).

-- name: ListActiveProducts :many
SELECT id, public_id, title, slug, description, seo_description, updated_at FROM product WHERE status = 'active' AND deleted_at IS NULL ORDER BY id DESC;

-- name: GetActiveProductBySlug :one
SELECT id, public_id, title, slug, description, seo_description, updated_at FROM product WHERE slug = ? AND status = 'active' AND deleted_at IS NULL;

-- name: ListFeaturedActiveProducts :many
SELECT id, public_id, title, slug, description, seo_description, updated_at FROM product WHERE status = 'active' AND featured = 1 AND deleted_at IS NULL ORDER BY updated_at DESC;

-- name: ListActiveProductsByCategory :many
SELECT p.id, p.public_id, p.title, p.slug, p.description, p.seo_description, p.updated_at
FROM product p
JOIN product_category pc ON pc.product_id = p.id
JOIN category c ON c.id = pc.category_id
WHERE c.slug = ? AND c.deleted_at IS NULL AND p.status = 'active' AND p.deleted_at IS NULL
ORDER BY p.updated_at DESC;

-- name: SearchActiveProducts :many
SELECT id, public_id, title, slug, description, seo_description, updated_at
FROM product
WHERE status = 'active' AND deleted_at IS NULL
  AND (instr(lower(title), lower(sqlc.arg(term))) > 0 OR instr(lower(description), lower(sqlc.arg(term))) > 0)
ORDER BY updated_at DESC
LIMIT 60;

-- name: GetActiveContentPageBySlug :one
SELECT id, public_id, title, slug, body_markdown, seo_description, updated_at FROM content_page WHERE slug = ? AND status = 'active' AND deleted_at IS NULL;

-- name: ListActiveContentPages :many
SELECT id, public_id, title, slug, body_markdown, seo_description, updated_at FROM content_page WHERE status = 'active' AND deleted_at IS NULL ORDER BY title;
