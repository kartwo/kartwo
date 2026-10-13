-- 博客富文本 / Blog Rich Text
-- 功能：为所见即所得正文增加安全 HTML 字段，旧 Markdown 原样保留
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-10-13 22:00:00

ALTER TABLE blog_post ADD COLUMN body_html TEXT NOT NULL DEFAULT '';
