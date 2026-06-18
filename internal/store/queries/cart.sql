-- Cart Queries
-- Purpose: anonymous cart + items read/write
-- Author: daxing  Email: 3442535897@qq.com  Time: 2026-06-18 12:40:00
-- NOTE: ASCII-only comments here (sqlc v1.30 multibyte-span bug; see DECISIONS.md).

-- name: CreateCart :execlastid
INSERT INTO cart (token, status) VALUES (?, 'active');

-- name: GetActiveCartByToken :one
SELECT id, token, status FROM cart WHERE token = ? AND status = 'active';

-- name: TouchCart :exec
UPDATE cart SET updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ?;

-- name: AddCartItem :exec
INSERT INTO cart_item (cart_id, variant_id, quantity) VALUES (?, ?, ?) ON CONFLICT(cart_id, variant_id) DO UPDATE SET quantity = quantity + excluded.quantity, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now');

-- name: SetCartItemQty :exec
UPDATE cart_item SET quantity = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE cart_id = ? AND variant_id = ?;

-- name: RemoveCartItem :exec
DELETE FROM cart_item WHERE cart_id = ? AND variant_id = ?;

-- name: ListCartItems :many
SELECT ci.variant_id, ci.quantity, v.public_id AS variant_public_id, v.sku, v.price_cents, p.id AS product_id, p.title AS product_title, p.slug AS product_slug, p.status AS product_status FROM cart_item ci JOIN variant v ON v.id = ci.variant_id JOIN product p ON p.id = v.product_id WHERE ci.cart_id = ? ORDER BY ci.id;

-- name: CountCartItems :one
SELECT COALESCE(SUM(quantity), 0) FROM cart_item WHERE cart_id = ?;

-- name: ListOptionsForVariant :many
SELECT po.name AS option_name, pov.value AS option_value, po.position AS option_position FROM variant_option_value vov JOIN product_option po ON po.id = vov.option_id JOIN product_option_value pov ON pov.id = vov.value_id WHERE vov.variant_id = ? ORDER BY po.position;
