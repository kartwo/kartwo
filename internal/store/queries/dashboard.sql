-- Dashboard Aggregate Queries
-- Purpose: overview stats - order counts/sales by time window, pending fulfillment, product count, stock alerts
-- Author: daxing  Email: 3442535897@qq.com  Time: 2026-07-07 18:47:01
-- NOTE: ASCII-only comments here (sqlc v1.30 multibyte-span bug; see DECISIONS.md). All aggregation in SQL (no N+1).

-- name: DashboardOrderWindow :one
-- Orders created at/after a boundary: total count (any status) + sales sum counting paid/fulfilled only (D6). Refunded orders leave paid/fulfilled so are fully excluded. CAST keeps sqlc types as int64.
SELECT COUNT(*) AS order_count, CAST(COALESCE(SUM(CASE WHEN status IN ('paid', 'fulfilled') THEN total_cents ELSE 0 END), 0) AS INTEGER) AS sales_cents FROM "order" WHERE created_at >= ?;

-- name: DashboardPendingFulfillment :one
-- Backlog needing action: paid orders not yet fulfilled (D2). Not time-windowed.
SELECT COUNT(*) FROM "order" WHERE status = 'paid';

-- name: CountActiveProducts :one
-- Products not soft-deleted.
SELECT COUNT(*) FROM product WHERE deleted_at IS NULL;

-- name: DashboardStockAlerts :one
-- Sellable = quantity - reserved; zero (<=0) and low (1..5) counts over non-deleted variants of non-deleted products (D3, N=5 fixed). CAST keeps sqlc types as int64.
SELECT CAST(COALESCE(SUM(CASE WHEN inv.quantity - inv.reserved <= 0 THEN 1 ELSE 0 END), 0) AS INTEGER) AS zero_stock, CAST(COALESCE(SUM(CASE WHEN inv.quantity - inv.reserved > 0 AND inv.quantity - inv.reserved <= 5 THEN 1 ELSE 0 END), 0) AS INTEGER) AS low_stock FROM inventory inv JOIN variant v ON v.id = inv.variant_id AND v.deleted_at IS NULL JOIN product p ON p.id = v.product_id AND p.deleted_at IS NULL;
