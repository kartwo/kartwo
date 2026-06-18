-- Order & Checkout Queries
-- Purpose: customer upsert, order/order_item create, atomic inventory reserve, order read
-- Author: daxing  Email: 3442535897@qq.com  Time: 2026-06-18 13:40:31
-- NOTE: ASCII-only comments here (sqlc v1.30 multibyte-span bug; see DECISIONS.md).

-- name: UpsertCustomer :exec
INSERT INTO customer (public_id, email, name) VALUES (?, ?, ?) ON CONFLICT(email) DO UPDATE SET name = excluded.name, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now');

-- name: GetCustomerByEmail :one
SELECT id, public_id, email, name FROM customer WHERE email = ?;

-- name: CreateOrder :execlastid
INSERT INTO "order" (public_id, customer_id, status, email, ship_name, ship_phone, ship_address, ship_country, currency, subtotal_cents, total_cents) VALUES (?, ?, 'pending', ?, ?, ?, ?, ?, ?, ?, ?);

-- name: CreateOrderItem :exec
INSERT INTO order_item (order_id, variant_id, product_title, variant_label, sku, unit_cents, quantity, line_cents) VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: ReserveInventory :execrows
UPDATE inventory SET reserved = reserved + ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE variant_id = ? AND quantity - reserved >= ?;

-- name: MarkCartConverted :exec
UPDATE cart SET status = 'converted', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ?;

-- name: GetOrderByPublicID :one
SELECT id, public_id, status, email, ship_name, ship_phone, ship_address, ship_country, currency, subtotal_cents, total_cents, created_at FROM "order" WHERE public_id = ?;

-- name: ListOrderItems :many
SELECT product_title, variant_label, sku, unit_cents, quantity, line_cents FROM order_item WHERE order_id = ? ORDER BY id;
