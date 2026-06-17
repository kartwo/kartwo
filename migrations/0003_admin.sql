-- 管理员与会话 / Admin & Session Schema
-- 功能：单管理员账号 + 服务端会话（可主动失效/轮换）；主口令 KEK 盐存 meta 表
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-06-17 23:18:17
-- 说明：纯 SQL 幂等迁移；口令哈希为 argon2id PHC 串；会话 token/csrf 为高熵随机；时间 UTC ISO8601。

CREATE TABLE IF NOT EXISTS admin_user (
    id            INTEGER PRIMARY KEY,
    public_id     TEXT NOT NULL UNIQUE,
    username      TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_admin_username ON admin_user(username);

CREATE TABLE IF NOT EXISTS session (
    id         INTEGER PRIMARY KEY,
    token      TEXT NOT NULL UNIQUE,
    admin_id   INTEGER NOT NULL REFERENCES admin_user(id) ON DELETE CASCADE,
    csrf_token TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS ix_session_admin ON session(admin_id);
