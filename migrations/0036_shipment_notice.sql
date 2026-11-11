-- 运单版本与发货通知 / Shipment Revisions and Notices
-- 功能：保留整单运单快照和逐次投递证据，不回填或补发历史邮件
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-10-25 15:33:44
CREATE TABLE IF NOT EXISTS shipment_revision (
    id INTEGER PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    shipment_id INTEGER NOT NULL REFERENCES shipment(id) ON DELETE CASCADE,
    version INTEGER NOT NULL CHECK(version>0),
    carrier TEXT NOT NULL,
    tracking_number TEXT NOT NULL,
    tracking_url TEXT NOT NULL DEFAULT '',
    legacy INTEGER NOT NULL DEFAULT 0 CHECK(legacy IN (0,1)),
    created_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE(shipment_id,version)
);
CREATE TABLE IF NOT EXISTS shipment_notice (
    id INTEGER PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    revision_id INTEGER NOT NULL REFERENCES shipment_revision(id) ON DELETE CASCADE,
    sequence INTEGER NOT NULL CHECK(sequence>0),
    request_key TEXT NOT NULL,
    origin TEXT NOT NULL CHECK(origin IN ('shipped','tracking_updated','retry','resend_after_check')),
    outbox_id INTEGER NOT NULL UNIQUE REFERENCES email_outbox(id) ON DELETE CASCADE,
    previous_id INTEGER REFERENCES shipment_notice(id) ON DELETE SET NULL,
    resolution TEXT NOT NULL DEFAULT '' CHECK(resolution IN ('','received','retried','not_received','superseded')),
    resolution_key TEXT NOT NULL DEFAULT '',
    sending_at INTEGER NOT NULL DEFAULT 0,
    resolved_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT(strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE(revision_id,sequence),
    UNIQUE(revision_id,request_key)
);
