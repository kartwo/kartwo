-- 支付尝试 / Payment Attempts
-- 功能：持久化收银台请求快照、幂等键与会话，租约限制同订单同通道的并发出站请求
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-10-24 13:16:58
CREATE TABLE IF NOT EXISTS payment_attempt (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    public_id TEXT NOT NULL UNIQUE,
    order_id INTEGER NOT NULL REFERENCES "order"(id),
    provider TEXT NOT NULL CHECK (provider IN ('stripe','paypal')),
    config_fingerprint TEXT NOT NULL,
    request_json TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','ready')),
    provider_reference TEXT NOT NULL DEFAULT '',
    redirect_url TEXT NOT NULL DEFAULT '',
    lease_token TEXT NOT NULL DEFAULT '',
    lease_until INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE(order_id, provider)
);
CREATE UNIQUE INDEX IF NOT EXISTS ix_payment_attempt_reference
    ON payment_attempt(provider, provider_reference) WHERE provider_reference <> '';
