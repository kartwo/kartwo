-- 退款 / Refund Schema
-- 功能：订单加支付引用列(payment_intent 等，退款退到 intent 非 session)；退款记录表(为部分退款留结构，v1 只全额)
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-06-22 11:20:11
-- 说明：纯 SQL 迁移；金额分；订单状态机新增 refunded；payment_ref=网关支付引用(Stripe payment_intent)。

ALTER TABLE "order" ADD COLUMN payment_provider TEXT NOT NULL DEFAULT '';
ALTER TABLE "order" ADD COLUMN payment_ref TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS ix_order_payment_ref ON "order"(payment_ref);

CREATE TABLE IF NOT EXISTS refund (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    order_id           INTEGER NOT NULL REFERENCES "order"(id) ON DELETE CASCADE,
    provider           TEXT NOT NULL,
    provider_refund_id TEXT NOT NULL,
    amount_cents       INTEGER NOT NULL,
    created_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE (provider, provider_refund_id)
);
CREATE INDEX IF NOT EXISTS ix_refund_order ON refund(order_id);
