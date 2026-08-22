-- Shopify 旧链接映射 / Shopify Legacy URL Redirect Schema
-- 功能：保存 Shopify Handle 到商品的永久重定向映射；仅由 Shopify 导入事务写入
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-08-21 14:30:00

CREATE TABLE IF NOT EXISTS shopify_redirect (
    legacy_handle TEXT PRIMARY KEY,
    product_id    INTEGER NOT NULL REFERENCES product(id) ON DELETE CASCADE,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS ix_shopify_redirect_product_id ON shopify_redirect(product_id);
