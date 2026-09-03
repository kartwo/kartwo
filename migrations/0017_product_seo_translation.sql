-- 商品翻译与 SEO 字段 / Product Translation and SEO Fields
-- 功能：保存中文辅助标题与 slug、英文 SEO 描述及图片替代文本
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-09-03 10:30:00

ALTER TABLE product ADD COLUMN title_zh TEXT NOT NULL DEFAULT '';
ALTER TABLE product ADD COLUMN slug_zh TEXT NOT NULL DEFAULT '';
ALTER TABLE product ADD COLUMN seo_description TEXT NOT NULL DEFAULT '';
ALTER TABLE product ADD COLUMN seo_description_zh TEXT NOT NULL DEFAULT '';

ALTER TABLE media_asset ADD COLUMN alt_text TEXT NOT NULL DEFAULT '';
