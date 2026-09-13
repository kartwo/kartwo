-- Public Demo Mode Schema
-- Purpose: isolated demo identities and session-owned temporary products
-- Author: daxing  Email: 3442535897@qq.com  Time: 2026-09-13 11:00:00

ALTER TABLE admin_user ADD COLUMN role TEXT NOT NULL DEFAULT 'owner'
    CHECK (role IN ('owner', 'demo'));

CREATE TABLE demo_product (
    product_id    INTEGER PRIMARY KEY REFERENCES product(id) ON DELETE CASCADE,
    session_token TEXT NOT NULL,
    slot          INTEGER NOT NULL CHECK (slot BETWEEN 1 AND 3),
    expires_at    TEXT NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE (session_token, slot)
);

CREATE INDEX ix_demo_product_expiry ON demo_product(expires_at);
CREATE INDEX ix_demo_product_session ON demo_product(session_token);
