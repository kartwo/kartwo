-- 支付通道切换 / Checkout Provider Switching
-- 功能：保留已停用会话历史，串行建单、切换和扣款，记录未知扣款结果以防重复收款
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-10-24 16:58:12
CREATE TABLE IF NOT EXISTS payment_attempt_v2 (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    public_id TEXT NOT NULL UNIQUE,
    order_id INTEGER NOT NULL REFERENCES "order"(id),
    provider TEXT NOT NULL CHECK(provider IN ('stripe','paypal')),
    config_fingerprint TEXT NOT NULL,
    request_json TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','ready','retired')),
    provider_reference TEXT NOT NULL DEFAULT '',
    redirect_url TEXT NOT NULL DEFAULT '',
    lease_token TEXT NOT NULL DEFAULT '',
    lease_until INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
INSERT INTO payment_attempt_v2(id,public_id,order_id,provider,config_fingerprint,request_json,state,provider_reference,redirect_url,lease_token,lease_until,created_at,updated_at)
SELECT id,public_id,order_id,provider,config_fingerprint,request_json,state,provider_reference,redirect_url,lease_token,lease_until,created_at,updated_at FROM payment_attempt WHERE true
ON CONFLICT(public_id) DO NOTHING;
DROP TABLE payment_attempt;
ALTER TABLE payment_attempt_v2 RENAME TO payment_attempt;
CREATE UNIQUE INDEX IF NOT EXISTS ix_payment_attempt_reference ON payment_attempt(provider,provider_reference) WHERE provider_reference<>'';
CREATE UNIQUE INDEX IF NOT EXISTS ix_payment_attempt_current ON payment_attempt(order_id,provider) WHERE state<>'retired';
CREATE TABLE IF NOT EXISTS checkout_lock (
    order_id INTEGER PRIMARY KEY REFERENCES "order"(id),
    lease_token TEXT NOT NULL DEFAULT '',
    lease_until INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS checkout_capture (
    attempt_id TEXT PRIMARY KEY,
    order_id INTEGER NOT NULL REFERENCES "order"(id),
    state TEXT NOT NULL CHECK(state IN ('pending','complete','failed')),
    first_sent_at INTEGER NOT NULL,
    created_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
