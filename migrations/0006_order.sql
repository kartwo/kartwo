-- 订单与客户 / Order & Customer Schema
-- 功能：访客下单（最小客户信息）+ 订单/订单行（金额与规格快照）；状态机 pending 起
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-06-18 13:40:31
-- 说明：纯 SQL 幂等迁移；金额分；外部 ID UUIDv7；order_item 落单价/标题/规格快照(防商品后续改动)。

CREATE TABLE IF NOT EXISTS customer (
    id         INTEGER PRIMARY KEY,
    public_id  TEXT NOT NULL UNIQUE,
    email      TEXT NOT NULL,
    name       TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_customer_email ON customer(email);

CREATE TABLE IF NOT EXISTS "order" (
    id             INTEGER PRIMARY KEY,
    public_id      TEXT NOT NULL UNIQUE,
    customer_id    INTEGER NOT NULL REFERENCES customer(id) ON DELETE RESTRICT,
    status         TEXT NOT NULL DEFAULT 'pending',   -- pending | paid | cancelled | fulfilled
    email          TEXT NOT NULL,
    ship_name      TEXT NOT NULL,
    ship_phone     TEXT NOT NULL DEFAULT '',
    ship_address   TEXT NOT NULL,
    ship_country   TEXT NOT NULL DEFAULT '',
    currency       TEXT NOT NULL DEFAULT 'CNY',
    subtotal_cents INTEGER NOT NULL,
    total_cents    INTEGER NOT NULL,
    created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS ix_order_customer ON "order"(customer_id);

CREATE TABLE IF NOT EXISTS order_item (
    id            INTEGER PRIMARY KEY,
    order_id      INTEGER NOT NULL REFERENCES "order"(id) ON DELETE CASCADE,
    variant_id    INTEGER NOT NULL REFERENCES variant(id) ON DELETE RESTRICT,
    product_title TEXT NOT NULL,                       -- 快照
    variant_label TEXT NOT NULL DEFAULT '',            -- 快照(规格文字，如 尺码:S · 颜色:黑)
    sku           TEXT NOT NULL DEFAULT '',            -- 快照
    unit_cents    INTEGER NOT NULL,                    -- 快照单价
    quantity      INTEGER NOT NULL,
    line_cents    INTEGER NOT NULL,
    CHECK (quantity > 0)
);
CREATE INDEX IF NOT EXISTS ix_order_item_order ON order_item(order_id);
