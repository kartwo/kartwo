-- 退款核对历史 / Refund Check History
-- 功能：保存完整平台退款快照与金额校正历史，启用后禁止旧累计事件覆盖校正结果
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-10-24 16:43:04
CREATE TABLE IF NOT EXISTS refund_check (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    public_id TEXT NOT NULL UNIQUE,
    receipt_id INTEGER NOT NULL REFERENCES payment_receipt(id),
    before_cents INTEGER NOT NULL CHECK(before_cents>=0),
    after_cents INTEGER NOT NULL CHECK(after_cents>=0),
    pending INTEGER NOT NULL CHECK(pending IN (0,1)),
    refunds_json TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS ix_refund_check_receipt ON refund_check(receipt_id,id);
