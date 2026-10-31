-- 退款请求 / Durable Refund Requests
-- 功能：保存退款意图、网关幂等键、处理中结果与恢复租约，避免重复或超额退款
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-10-24 22:12:00
CREATE TABLE IF NOT EXISTS refund_request (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    public_id TEXT NOT NULL UNIQUE,
    client_key TEXT NOT NULL,
    order_id INTEGER NOT NULL REFERENCES "order"(id),
    receipt_id INTEGER NOT NULL REFERENCES payment_receipt(id),
    provider TEXT NOT NULL CHECK(provider IN ('stripe','paypal')),
    payment_ref TEXT NOT NULL,
    amount_cents INTEGER NOT NULL CHECK(amount_cents>0),
    currency TEXT NOT NULL,
    initial_refunded_cents INTEGER NOT NULL CHECK(initial_refunded_cents>=0),
    config_fingerprint TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'prepared' CHECK(state IN ('prepared','pending','succeeded','failed','review')),
    gateway_ref TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    first_sent_at INTEGER NOT NULL DEFAULT 0,
    lease_token TEXT NOT NULL DEFAULT '',
    lease_until INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE(order_id,client_key)
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_refund_request_active ON refund_request(receipt_id) WHERE state IN ('prepared','pending','review');
CREATE UNIQUE INDEX IF NOT EXISTS ux_refund_request_gateway ON refund_request(provider,gateway_ref) WHERE gateway_ref<>'';
