-- 客服留言 / Contact Messages
-- 功能：私密客服收件箱及到期匿名化，普通后台不提供删除操作
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-09-28 14:00:00

CREATE TABLE IF NOT EXISTS contact_message (
    id INTEGER PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    email TEXT NOT NULL,
    topic TEXT NOT NULL CHECK (topic IN ('product','shipping','order','returns','other')),
    body TEXT NOT NULL,
    order_ref TEXT NOT NULL DEFAULT '',
    ip_address TEXT NOT NULL DEFAULT '',
    country_code TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'new' CHECK (status IN ('new','processing','closed')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    closed_at TEXT,
    anonymized_at TEXT
);
CREATE INDEX IF NOT EXISTS ix_contact_status_created ON contact_message(status, created_at DESC);
CREATE INDEX IF NOT EXISTS ix_contact_closed ON contact_message(closed_at) WHERE closed_at IS NOT NULL;
