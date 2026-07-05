-- 设置 / Settings Schema
-- 功能：键值设置；敏感值(如支付密钥)用 KEK 加密后存(encrypted=1)，非敏感(如市场代码)明文
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-06-19 21:22:05
-- 说明：纯 SQL 幂等迁移；加密值为 base64(nonce||ciphertext)，密钥绝不明文落库(见 ARCHITECTURE §14/§15)。

CREATE TABLE IF NOT EXISTS setting (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    encrypted  INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
