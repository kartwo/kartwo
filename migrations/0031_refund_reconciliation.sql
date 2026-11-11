-- 退款异常对账 / Refund Reconciliation Issues
-- 功能：保存平台当前状态与已入账退款的矛盾，阻止未核对资金被继续退款或发货
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-10-24 23:55:00
CREATE TABLE IF NOT EXISTS refund_reconciliation (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    public_id TEXT NOT NULL UNIQUE,
    receipt_id INTEGER NOT NULL REFERENCES payment_receipt(id),
    provider TEXT NOT NULL CHECK(provider IN ('stripe','paypal')),
    refund_ref TEXT NOT NULL,
    amount_cents INTEGER NOT NULL CHECK(amount_cents>0),
    observed_state TEXT NOT NULL CHECK(observed_state IN ('pending','failed','succeeded')),
    state TEXT NOT NULL DEFAULT 'open' CHECK(state IN ('open','resolved')),
    created_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE(provider,refund_ref)
);
CREATE INDEX IF NOT EXISTS ix_refund_reconciliation_open ON refund_reconciliation(receipt_id) WHERE state='open';
