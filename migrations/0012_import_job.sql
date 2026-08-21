-- 导入任务 / Import Job Schema
-- 功能：持久化 CSV 干跑预览、行级错误与执行状态；来源哈希为同文件重试的幂等锚点
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-08-21 12:23:42

CREATE TABLE IF NOT EXISTS import_job (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    public_id     TEXT NOT NULL UNIQUE,
    source_sha256 TEXT NOT NULL UNIQUE,
    source_csv    TEXT NOT NULL,
    status        TEXT NOT NULL, -- previewed | rejected | succeeded
    total_rows    INTEGER NOT NULL,
    product_count INTEGER NOT NULL,
    variant_count INTEGER NOT NULL,
    errors_json   TEXT NOT NULL DEFAULT '[]',
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    completed_at  TEXT
);

CREATE INDEX IF NOT EXISTS ix_import_job_created_at ON import_job(created_at);
