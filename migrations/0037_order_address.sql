-- 订单结构化地址 / Structured Order Address
-- 功能：新订单保存独立地址字段；旧地址保持原文，不猜测拆分或改写历史
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-10-25 17:18:27
ALTER TABLE "order" ADD COLUMN ship_address_version INTEGER NOT NULL DEFAULT 0 CHECK(ship_address_version IN (0,1));
ALTER TABLE "order" ADD COLUMN ship_address_line1 TEXT NOT NULL DEFAULT '';
ALTER TABLE "order" ADD COLUMN ship_address_line2 TEXT NOT NULL DEFAULT '';
ALTER TABLE "order" ADD COLUMN ship_city TEXT NOT NULL DEFAULT '';
ALTER TABLE "order" ADD COLUMN ship_district TEXT NOT NULL DEFAULT '';
ALTER TABLE "order" ADD COLUMN ship_region TEXT NOT NULL DEFAULT '';
ALTER TABLE "order" ADD COLUMN ship_postal_code TEXT NOT NULL DEFAULT '';
