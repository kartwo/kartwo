-- 审计事件表 / Audit Event Table
-- 功能：持久化后台关键操作的只追加审计记录，供商家追溯管理动作
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-08-24 00:20:00

CREATE TABLE IF NOT EXISTS audit_event (
    id               INTEGER PRIMARY KEY,
    public_id        TEXT NOT NULL UNIQUE,
    admin_id         INTEGER NOT NULL REFERENCES admin_user(id),
    action           TEXT NOT NULL,
    target_type      TEXT NOT NULL DEFAULT '',
    target_public_id TEXT NOT NULL DEFAULT '',
    created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS ix_audit_event_created_at ON audit_event (created_at DESC, id DESC);
