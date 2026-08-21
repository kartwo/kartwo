-- 导入来源格式 / Import Source Format
-- 功能：标记导入任务的解析协议，确保确认执行时使用与预览一致的来源适配器
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-08-21 16:00:24

ALTER TABLE import_job ADD COLUMN source_format TEXT NOT NULL DEFAULT 'generic';
