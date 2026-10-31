-- 游客订单访问 / Guest Order Access
-- 功能：持久化浏览器访问凭证摘要及有时效、有次数限制的邮箱验证挑战
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-10-24 17:18:40
CREATE TABLE IF NOT EXISTS order_guest_access (
    token_hash TEXT PRIMARY KEY,
    order_id INTEGER NOT NULL REFERENCES "order"(id) ON DELETE CASCADE,
    expires_at INTEGER NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS ix_order_guest_access_order ON order_guest_access(order_id);
CREATE INDEX IF NOT EXISTS ix_order_guest_access_expiry ON order_guest_access(expires_at);

CREATE TABLE IF NOT EXISTS order_guest_challenge (
    public_id TEXT PRIMARY KEY,
    order_id INTEGER NOT NULL REFERENCES "order"(id) ON DELETE CASCADE,
    browser_hash TEXT NOT NULL,
    code_hash TEXT NOT NULL,
    outbox_id INTEGER NOT NULL UNIQUE REFERENCES email_outbox(id) ON DELETE CASCADE,
    expires_at INTEGER NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 5),
    consumed_at INTEGER NOT NULL DEFAULT 0,
    requested_at INTEGER NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS ix_order_guest_challenge_recent ON order_guest_challenge(order_id,requested_at);
CREATE INDEX IF NOT EXISTS ix_order_guest_challenge_browser ON order_guest_challenge(order_id,browser_hash,requested_at);

CREATE INDEX IF NOT EXISTS ix_email_guest_body ON email_outbox(id) WHERE kind GLOB 'guest_access:*' AND body<>'';

CREATE INDEX IF NOT EXISTS ix_order_guest_challenge_retention ON order_guest_challenge(requested_at);
