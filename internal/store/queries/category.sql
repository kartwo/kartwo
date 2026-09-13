-- Category Queries
-- Purpose: category CRUD, ordering, product links and storefront collection reads
-- Author: daxing  Email: 3442535897@qq.com  Time: 2026-06-17 18:13:43
-- NOTE: ASCII-only comments here (sqlc v1.30 multibyte-span bug; see DECISIONS.md).

-- name: CreateCategory :execlastid
INSERT INTO category (public_id, name, slug, parent_id, position) VALUES (?, ?, ?, ?, ?);

-- name: LinkProductCategory :exec
INSERT OR IGNORE INTO product_category (product_id, category_id) VALUES (?, ?);

-- name: ListCategories :many
SELECT id, public_id, name, slug, position FROM category WHERE deleted_at IS NULL ORDER BY position, id;

-- name: GetCategoryByPublicID :one
SELECT id, public_id, name, slug, position FROM category WHERE public_id = ? AND deleted_at IS NULL;

-- name: GetCategoryBySlug :one
SELECT id, public_id, name, slug, position FROM category WHERE slug = ? AND deleted_at IS NULL;

-- name: UpdateCategory :exec
UPDATE category SET name = ?, slug = ?, position = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ? AND deleted_at IS NULL;

-- name: SoftDeleteCategory :exec
UPDATE category SET deleted_at = strftime('%Y-%m-%dT%H:%M:%fZ','now'), updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ? AND deleted_at IS NULL;

-- name: CountProductsByCategory :one
SELECT COUNT(*) FROM product_category pc JOIN product p ON p.id = pc.product_id WHERE pc.category_id = ? AND p.deleted_at IS NULL;

-- name: ListCategoryPublicIDsByProduct :many
SELECT c.public_id FROM category c JOIN product_category pc ON pc.category_id = c.id WHERE pc.product_id = ? AND c.deleted_at IS NULL ORDER BY c.position, c.id;

-- name: DeleteProductCategories :exec
DELETE FROM product_category WHERE product_id = ?;

-- name: ListStorefrontCategories :many
SELECT c.public_id, c.name, c.slug, c.position, COUNT(DISTINCT p.id) AS product_count
FROM category c
JOIN product_category pc ON pc.category_id = c.id
JOIN product p ON p.id = pc.product_id AND p.status = 'active' AND p.deleted_at IS NULL
WHERE c.deleted_at IS NULL
GROUP BY c.id
ORDER BY c.position, c.id;
