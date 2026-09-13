-- 配送与发货 / Shipping and Fulfillment
-- 功能：配送分区、订单配送金额快照及单次发货追踪信息
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-09-07 13:00:00
CREATE TABLE IF NOT EXISTS shipping_zone (id INTEGER PRIMARY KEY, public_id TEXT NOT NULL UNIQUE, name TEXT NOT NULL, countries TEXT NOT NULL DEFAULT '', rate_cents INTEGER NOT NULL CHECK(rate_cents>=0), free_over_cents INTEGER NOT NULL DEFAULT 0 CHECK(free_over_cents>=0), active INTEGER NOT NULL DEFAULT 1 CHECK(active IN(0,1)), created_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now')), updated_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now')), deleted_at TEXT);
ALTER TABLE "order" ADD COLUMN shipping_cents INTEGER NOT NULL DEFAULT 0;
ALTER TABLE "order" ADD COLUMN shipping_rule_name TEXT NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS shipment (id INTEGER PRIMARY KEY, order_id INTEGER NOT NULL UNIQUE REFERENCES "order"(id) ON DELETE CASCADE, carrier TEXT NOT NULL DEFAULT '', tracking_number TEXT NOT NULL DEFAULT '', tracking_url TEXT NOT NULL DEFAULT '', shipped_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now')));
