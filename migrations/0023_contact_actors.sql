-- 客服处理账号 / Contact Handling Actors
-- 功能：保存最近接手与结案账号的稳定标识、用户名快照及处理时间
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-09-28 16:20:00

ALTER TABLE contact_message ADD COLUMN processing_by_id TEXT NOT NULL DEFAULT '';
ALTER TABLE contact_message ADD COLUMN processing_by_name TEXT NOT NULL DEFAULT '';
ALTER TABLE contact_message ADD COLUMN processing_at TEXT;
ALTER TABLE contact_message ADD COLUMN closed_by_id TEXT NOT NULL DEFAULT '';
ALTER TABLE contact_message ADD COLUMN closed_by_name TEXT NOT NULL DEFAULT '';
