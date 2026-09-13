-- Content Page Queries
-- Purpose: admin content page create, list, read, update, soft delete
-- Author: daxing  Email: 3442535897@qq.com  Time: 2026-09-07 12:00:00

-- name: CreateContentPage :execlastid
INSERT INTO content_page (public_id, title, slug, body_markdown, seo_description, status) VALUES (?, ?, ?, ?, ?, ?);

-- name: ListContentPages :many
SELECT id, public_id, title, slug, body_markdown, seo_description, status, created_at, updated_at FROM content_page WHERE deleted_at IS NULL ORDER BY updated_at DESC;

-- name: GetContentPageByPublicID :one
SELECT id, public_id, title, slug, body_markdown, seo_description, status, created_at, updated_at FROM content_page WHERE public_id = ? AND deleted_at IS NULL;

-- name: UpdateContentPage :exec
UPDATE content_page SET title = ?, body_markdown = ?, seo_description = ?, status = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ? AND deleted_at IS NULL;

-- name: SoftDeleteContentPage :exec
UPDATE content_page SET deleted_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ? AND deleted_at IS NULL;
