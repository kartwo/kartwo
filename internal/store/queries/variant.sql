-- Variant Queries
-- Purpose: variant create/update, option-value links, variant matrix
-- Author: daxing  Email: 3442535897@qq.com  Time: 2026-06-17 18:13:43
-- NOTE: ASCII-only comments here (sqlc v1.30 multibyte-span bug; see DECISIONS.md).

-- name: CreateVariant :execlastid
INSERT INTO variant (public_id, product_id, sku, price_cents, option_key, position) VALUES (?, ?, ?, ?, ?, ?);

-- name: AddVariantOptionValue :exec
INSERT INTO variant_option_value (variant_id, option_id, value_id) VALUES (?, ?, ?);

-- name: ListVariantsByProduct :many
SELECT id, public_id, sku, price_cents, option_key, position FROM variant WHERE product_id = ? AND deleted_at IS NULL ORDER BY position, id;

-- name: ListVariantOptionValuesByProduct :many
SELECT v.id AS variant_id, po.name AS option_name, pov.value AS option_value, po.position AS option_position FROM variant v JOIN variant_option_value vov ON vov.variant_id = v.id JOIN product_option po ON po.id = vov.option_id JOIN product_option_value pov ON pov.id = vov.value_id WHERE v.product_id = ? AND v.deleted_at IS NULL ORDER BY v.position, v.id, po.position;

-- name: GetVariantByPublicID :one
SELECT id, public_id, product_id, sku, price_cents FROM variant WHERE public_id = ? AND deleted_at IS NULL;

-- name: SoftDeleteVariantsByProduct :exec
UPDATE variant SET deleted_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE product_id = ? AND deleted_at IS NULL;
