-- 商品精选与内容页 / Merchandising and Content Pages
-- 功能：保存首页手动精选商品及安全受限 Markdown 内容页
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-09-07 12:00:00

ALTER TABLE product ADD COLUMN featured INTEGER NOT NULL DEFAULT 0 CHECK (featured IN (0, 1));

CREATE TABLE IF NOT EXISTS content_page (
    id              INTEGER PRIMARY KEY,
    public_id       TEXT NOT NULL UNIQUE,
    title           TEXT NOT NULL,
    slug            TEXT NOT NULL,
    body_markdown   TEXT NOT NULL DEFAULT '',
    seo_description TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active')),
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    deleted_at      TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_content_page_slug ON content_page(slug) WHERE deleted_at IS NULL;
