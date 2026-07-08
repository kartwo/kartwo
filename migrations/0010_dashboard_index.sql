-- 概览索引 / Dashboard Index
-- 功能：为概览「今日 / 近 7 日」按 created_at 范围聚合加索引（面向订单表增长的廉价保险，M4.2.2 D4）
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-07-07 18:47:01
-- 说明：纯 SQL 幂等迁移；仅加索引、无数据变更；概览按 created_at 做今日/近7日范围过滤。

CREATE INDEX IF NOT EXISTS ix_order_created_at ON "order"(created_at);
