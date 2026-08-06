-- 邮件发件箱 / Email Outbox Schema
-- 功能：异步邮件队列（不阻塞下单）——支付确认后入队顾客下单确认信，后台 worker 轮询发送 + 重试
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-07-28 13:27:02
-- 说明：纯 SQL 幂等迁移；金额已在 body 内展示层转换；UNIQUE(order_id,kind) 保幂等（PayPal 同步 capture + webhook 备份双触发只一封）。

CREATE TABLE IF NOT EXISTS email_outbox (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    order_id        INTEGER NOT NULL REFERENCES "order"(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL,                       -- order_confirmation（v1 仅此一种）
    to_addr         TEXT NOT NULL,                       -- 收件人（下单时的顾客邮箱快照）
    subject         TEXT NOT NULL,
    body            TEXT NOT NULL,                       -- 入队时已组好的纯文本正文
    status          TEXT NOT NULL DEFAULT 'pending',     -- pending | sending | sent | failed | skipped
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    last_error      TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    sent_at         TEXT
);
-- 幂等锚点：同一订单同一信种只入队一次（INSERT OR IGNORE 依赖此约束）。
CREATE UNIQUE INDEX IF NOT EXISTS ux_email_outbox_order_kind ON email_outbox(order_id, kind);
-- worker 认领到期待发邮件的检索路径。
CREATE INDEX IF NOT EXISTS ix_email_outbox_due ON email_outbox(status, next_attempt_at);
