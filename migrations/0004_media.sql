-- 媒体资产 / Media Assets Schema
-- 功能：商品图片原图与派生图记录；DB 只存相对路径，不存 BLOB
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-06-18 09:48:36
-- 说明：纯 SQL 幂等迁移；路径相对 ./data/media；内容哈希命名；软删。

CREATE TABLE IF NOT EXISTS media_asset (
    id            INTEGER PRIMARY KEY,
    public_id     TEXT NOT NULL UNIQUE,
    product_id    INTEGER NOT NULL REFERENCES product(id) ON DELETE CASCADE,
    content_hash  TEXT NOT NULL,
    original_path TEXT NOT NULL,                 -- originals/<hash>.<ext>
    mime          TEXT NOT NULL,
    width         INTEGER NOT NULL,
    height        INTEGER NOT NULL,
    size_bytes    INTEGER NOT NULL,
    position      INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    deleted_at    TEXT
);
CREATE INDEX IF NOT EXISTS ix_media_product ON media_asset(product_id);

CREATE TABLE IF NOT EXISTS media_derivative (
    id         INTEGER PRIMARY KEY,
    asset_id   INTEGER NOT NULL REFERENCES media_asset(id) ON DELETE CASCADE,
    label      TEXT NOT NULL,                     -- thumb | medium | large
    path       TEXT NOT NULL,                     -- derived/<hash>_<label>.webp
    format     TEXT NOT NULL,                     -- webp
    width      INTEGER NOT NULL,
    height     INTEGER NOT NULL,
    size_bytes INTEGER NOT NULL,
    UNIQUE (asset_id, label)
);
