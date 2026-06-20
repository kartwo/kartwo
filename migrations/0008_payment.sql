-- 支付 / Payment Schema
-- 功能：Webhook 事件去重表（回调幂等地基）；(provider,event_id) 唯一约束保证同一事件只处理一次
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-06-20 20:26:08
-- 说明：纯 SQL 幂等迁移；幂等靠 UNIQUE 冲突——去重 INSERT 与订单 pending->paid 更新须同一事务（见 ARCHITECTURE §11）。

CREATE TABLE IF NOT EXISTS webhook_event (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    provider    TEXT NOT NULL,
    event_id    TEXT NOT NULL,
    event_type  TEXT NOT NULL DEFAULT '',
    order_ref   TEXT NOT NULL DEFAULT '',
    received_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE (provider, event_id)
);
