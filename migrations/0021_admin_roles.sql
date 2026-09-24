-- 后台多用户与角色 / Admin Users and Roles
-- 功能：在旧 owner/demo 字段旁增加独立账号类型、员工角色和账号状态
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-09-24 10:00:00

CREATE TABLE admin_role (
    id INTEGER PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL UNIQUE,
    permissions TEXT NOT NULL,
    system_key TEXT UNIQUE,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

ALTER TABLE admin_user ADD COLUMN account_type TEXT NOT NULL DEFAULT 'owner'
    CHECK (account_type IN ('owner', 'staff', 'demo'));
ALTER TABLE admin_user ADD COLUMN role_id INTEGER REFERENCES admin_role(id);
ALTER TABLE admin_user ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
ALTER TABLE admin_user ADD COLUMN disabled_at TEXT;
ALTER TABLE admin_user ADD COLUMN last_login_at TEXT;
UPDATE admin_user SET account_type = 'demo' WHERE role = 'demo';
CREATE INDEX ix_admin_user_role ON admin_user(role_id);

CREATE TABLE admin_invite (
    id INTEGER PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    admin_id INTEGER NOT NULL REFERENCES admin_user(id),
    expires_at TEXT NOT NULL,
    consumed_at TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX ix_admin_invite_user ON admin_invite(admin_id);

ALTER TABLE audit_event ADD COLUMN admin_username_snapshot TEXT NOT NULL DEFAULT '';
UPDATE audit_event SET admin_username_snapshot = (SELECT username FROM admin_user WHERE id = audit_event.admin_id);
CREATE TRIGGER trg_audit_username_snapshot AFTER INSERT ON audit_event BEGIN
    UPDATE audit_event SET admin_username_snapshot = (SELECT username FROM admin_user WHERE id = NEW.admin_id) WHERE id = NEW.id;
END;

INSERT INTO admin_role (public_id, name, permissions, system_key) VALUES
('00000000-0000-7000-8000-000000000001', '管理员', '["dashboard.view","diagnostics.view","audit.view","products.view","products.create","products.edit","products.publish","products.delete","products.inventory","products.price","products.media","categories.manage","content.manage","orders.view","orders.customer_data","orders.fulfill","imports.manage","settings.shop","settings.shipping","settings.domain","settings.policy"]', 'administrator'),
('00000000-0000-7000-8000-000000000002', '运营', '["dashboard.view","products.view","products.create","products.edit","products.publish","products.inventory","products.price","products.media","categories.manage","content.manage","orders.view","orders.customer_data","orders.fulfill","settings.shipping"]', 'operator'),
('00000000-0000-7000-8000-000000000003', '内容编辑', '["dashboard.view","products.view","products.create","products.edit","products.media","categories.manage","content.manage"]', 'content_editor'),
('00000000-0000-7000-8000-000000000004', '只读', '["dashboard.view","products.view","orders.view"]', 'viewer');
