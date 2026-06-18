-- 购物车 / Cart Schema
-- 功能：匿名购物车（cookie token 标识）+ 购物车行；状态机 active/converted/abandoned
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-06-18 12:40:00
-- 说明：纯 SQL 幂等迁移；单价不存快照（结账时于 order_item 落快照，M2.3）；金额分。

CREATE TABLE IF NOT EXISTS cart (
    id         INTEGER PRIMARY KEY,
    token      TEXT NOT NULL UNIQUE,
    status     TEXT NOT NULL DEFAULT 'active',   -- active | converted | abandoned
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS cart_item (
    id         INTEGER PRIMARY KEY,
    cart_id    INTEGER NOT NULL REFERENCES cart(id) ON DELETE CASCADE,
    variant_id INTEGER NOT NULL REFERENCES variant(id) ON DELETE CASCADE,
    quantity   INTEGER NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE (cart_id, variant_id),
    CHECK (quantity > 0)
);
CREATE INDEX IF NOT EXISTS ix_cart_item_cart ON cart_item(cart_id);
