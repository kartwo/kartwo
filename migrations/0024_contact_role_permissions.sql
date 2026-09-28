-- 客服预设角色权限 / Contact Preset Role Permissions
-- 功能：使预设管理员与运营角色可查看并处理客服留言
-- 作者：仗键天涯(daxing)
-- 邮箱：3442535897@qq.com
-- 时间：2026-09-28 17:35:00

UPDATE admin_role
SET permissions = json_insert(permissions, '$[#]', 'contact.view', '$[#]', 'contact.manage'),
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE system_key IN ('administrator','operator');
