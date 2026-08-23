-- 升级迁移索引 / Upgrade Migration Index
-- 功能：为迁移历史的运维查询提供时间索引，并作为升级保护首个真实迁移验证点
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-08-23 14:10:00

CREATE INDEX IF NOT EXISTS ix_schema_migrations_applied_at
ON schema_migrations (applied_at);
