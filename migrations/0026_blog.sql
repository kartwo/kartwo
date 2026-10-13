-- 博客文章 / Blog Posts
-- 功能：独立保存文章、发布状态和封面；设置缺省为开启
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-10-13 10:00:00

CREATE TABLE IF NOT EXISTS blog_post (
    id INTEGER PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    slug TEXT NOT NULL,
    excerpt TEXT NOT NULL DEFAULT '',
    body_markdown TEXT NOT NULL DEFAULT '',
    cover_path TEXT NOT NULL DEFAULT '',
    seo_description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','active')),
    published_at TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    deleted_at TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_blog_post_slug ON blog_post(slug) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS ix_blog_post_public ON blog_post(status,published_at DESC,id DESC) WHERE deleted_at IS NULL;

UPDATE admin_role SET permissions=json_insert(permissions,'$[#]','blog.view','$[#]','blog.manage'),
    updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE system_key IN ('administrator','operator')
    AND NOT EXISTS (SELECT 1 FROM json_each(admin_role.permissions) WHERE value='blog.view');
