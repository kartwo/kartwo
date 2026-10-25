-- 基础售后申请 / After Sale Requests
-- 功能：独立保存顾客申请、政策资料快照和审核事件，不改资金、库存或历史履约
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-10-25 20:55:43
CREATE TABLE IF NOT EXISTS after_sale (
    id INTEGER PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    order_id INTEGER NOT NULL REFERENCES "order"(id),
    client_key TEXT NOT NULL,
    kind TEXT NOT NULL CHECK(kind IN ('refund','return_refund')),
    state TEXT NOT NULL DEFAULT 'requested' CHECK(state IN ('requested','awaiting_return','returned','approved','completed','rejected','withdrawn')),
    version INTEGER NOT NULL DEFAULT 1 CHECK(version>0),
    amount_cents INTEGER NOT NULL CHECK(amount_cents>0),
    currency TEXT NOT NULL,
    reason TEXT NOT NULL,
    policy_configured INTEGER NOT NULL CHECK(policy_configured IN (0,1)),
    return_window_days INTEGER NOT NULL DEFAULT 0 CHECK(return_window_days>=0),
    return_shipping_payer TEXT NOT NULL DEFAULT '',
    support_email TEXT NOT NULL DEFAULT '',
    merchant_message TEXT NOT NULL DEFAULT '',
    return_instructions TEXT NOT NULL DEFAULT '',
    return_carrier TEXT NOT NULL DEFAULT '',
    return_tracking_number TEXT NOT NULL DEFAULT '',
    refund_request_id INTEGER REFERENCES refund_request(id),
    created_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE(order_id,client_key),
    CHECK((state='completed')=(refund_request_id IS NOT NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_after_sale_active ON after_sale(order_id)
WHERE state IN ('requested','awaiting_return','returned','approved');
CREATE UNIQUE INDEX IF NOT EXISTS ux_after_sale_refund ON after_sale(refund_request_id) WHERE refund_request_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS ix_after_sale_order ON after_sale(order_id,id);
CREATE TABLE IF NOT EXISTS after_sale_event (
    id INTEGER PRIMARY KEY,
    after_sale_id INTEGER NOT NULL REFERENCES after_sale(id),
    version INTEGER NOT NULL CHECK(version>0),
    client_key TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    actor TEXT NOT NULL CHECK(actor IN ('customer','merchant')),
    action TEXT NOT NULL CHECK(action IN ('request','approve','reject','receive_return','complete','withdraw','return_tracking')),
    state TEXT NOT NULL CHECK(state IN ('requested','awaiting_return','returned','approved','completed','rejected','withdrawn')),
    message TEXT NOT NULL DEFAULT '',
    carrier TEXT NOT NULL DEFAULT '',
    tracking_number TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE(after_sale_id,version),
    UNIQUE(after_sale_id,client_key)
);
