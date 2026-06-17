-- 元信息键值表 / Meta Key-Value Table
-- 功能：存放店铺级元信息（如 schema 版本、安装标识等），并作为 sqlc 选型落地的验证样例
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-06-17 17:05:46
-- 说明：纯 SQL 迁移、幂等可重入（IF NOT EXISTS）；时间戳用 UTC ISO8601 文本。

CREATE TABLE IF NOT EXISTS meta (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
