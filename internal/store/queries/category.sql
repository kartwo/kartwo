-- Category Queries
-- Purpose: category create, product-category link
-- Author: daxing  Email: 3442535897@qq.com  Time: 2026-06-17 18:13:43
-- NOTE: ASCII-only comments here (sqlc v1.30 multibyte-span bug; see DECISIONS.md).

-- name: CreateCategory :execlastid
INSERT INTO category (public_id, name, slug, parent_id, position) VALUES (?, ?, ?, ?, ?);

-- name: LinkProductCategory :exec
INSERT OR IGNORE INTO product_category (product_id, category_id) VALUES (?, ?);
