// 用户角色接口 / Users and Roles API
// 功能：严格用户角色契约、权限依赖与一次性邀请地址校验，不缓存邀请秘密
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:09:08

import { request, APIError, errorMessage } from './transport'
import { record, string, choice, integer } from './api-values'
export const permissionGroups = [
  ['概览与诊断', [['dashboard.view', '查看概览'], ['diagnostics.view', '查看诊断'], ['audit.view', '查看日志']]],
  ['商品与内容', [['products.view', '查看商品'], ['products.create', '新建商品'], ['products.edit', '编辑商品'], ['products.publish', '上下架商品'], ['products.delete', '删除商品'], ['products.inventory', '修改库存'], ['products.price', '修改价格'], ['products.media', '管理商品图片'], ['categories.manage', '管理分类'], ['content.manage', '管理内容页'], ['blog.view', '查看博客文章'], ['blog.manage', '管理博客文章']]],
  ['订单', [['orders.view', '查看订单'], ['orders.customer_data', '查看客户信息'], ['orders.fulfill', '发货']]],
  ['客服留言', [['contact.view', '查看客服留言与设置'], ['contact.manage', '处理客服留言']]],
  ['其他', [['imports.manage', '导入商品'], ['settings.shop', '店铺设置'], ['settings.shipping', '配送设置'], ['settings.domain', '域名设置'], ['settings.policy', '政策设置']]],
] as const
const permissions = new Set<string>(permissionGroups.flatMap(([, items]) => items.map(([key]) => key)))
export interface ManagedUser { public_id: string; username: string; display_name: string; role_id: string; role_name: string; status: 'active' | 'invited' | 'disabled'; last_login_at: string; created_at: string }
export interface ManagedRole { public_id: string; name: string; permissions: string[]; system_key: string; user_count: number }
export interface UsersSnapshot { users: ManagedUser[]; roles: ManagedRole[] }
export interface UserFields { display_name: string; role_id: string; disabled: boolean }
export interface InviteFields { username: string; display_name: string; role_id: string }
export interface RoleFields { name: string; permissions: string[] }
function publicID(value: string): string {
  if (!/^[a-zA-Z0-9-]{8,}$/.test(value)) throw new Error('用户或角色标识无效')
  return encodeURIComponent(value)
}
function rows(value: unknown): unknown[] { if (!Array.isArray(value)) throw new Error('用户或角色列表格式不正确'); return value }
export function parseUsers(value: unknown): ManagedUser[] {
  const seen = new Set<string>()
  return rows(value).map(value => {
    const data = record(value), id = string(data, 'public_id'), role = string(data, 'role_id')
    publicID(id); if (role) publicID(role)
    if (seen.has(id)) throw new Error('用户标识重复'); seen.add(id)
    return { public_id: id, username: string(data, 'username'), display_name: string(data, 'display_name'), role_id: role, role_name: string(data, 'role_name'), status: choice(data, 'status', ['active', 'invited', 'disabled']), last_login_at: string(data, 'last_login_at'), created_at: string(data, 'created_at') }
  })
}
export function parseRoles(value: unknown): ManagedRole[] {
  const seen = new Set<string>()
  return rows(value).map(value => {
    const data = record(value), id = string(data, 'public_id'), raw = data.permissions
    publicID(id)
    if (seen.has(id)) throw new Error('角色标识重复'); seen.add(id)
    if (!Array.isArray(raw) || !raw.every(item => typeof item === 'string' && permissions.has(item))) throw new Error('角色包含无法识别或不可分配的权限，请核对服务端版本')
    return { public_id: id, name: string(data, 'name'), permissions: [...new Set(raw as string[])], system_key: string(data, 'system_key'), user_count: integer(data, 'user_count', 0, Number.MAX_SAFE_INTEGER) }
  })
}
export function permissionError(values: string[]): string {
  const selected = new Set(values)
  if (!selected.size) return '至少选择一项权限'
  if (values.some(value => !permissions.has(value))) return '含有不可分配的权限'
  if (values.some(value => value.startsWith('products.') && value !== 'products.view' || value === 'categories.manage') && !selected.has('products.view')) return '商品操作权限需要同时勾选「查看商品」'
  if (selected.has('orders.customer_data') && !selected.has('orders.view')) return '客户信息权限需要同时勾选「查看订单」'
  if (selected.has('orders.fulfill') && (!selected.has('orders.view') || !selected.has('orders.customer_data'))) return '发货权限需要同时勾选「查看订单」和「查看客户信息」'
  if (selected.has('contact.manage') && !selected.has('contact.view')) return '处理客服留言需要同时勾选「查看客服留言」'
  if (selected.has('blog.manage') && !selected.has('blog.view')) return '管理博客文章需要同时勾选「查看博客文章」'
  return ''
}
function rolePayload(input: RoleFields) {
  const error = permissionError(input.permissions)
  if (error) throw new Error(error)
  return { name: input.name.trim(), permissions: [...new Set(input.permissions)] }
}
export function inviteURL(value: unknown): string {
  const path = string(record(value), 'invite_path')
  if (!/^\/admin\/#\/accept-invite\?token=[a-zA-Z0-9_-]{32,256}$/.test(path)) throw new Error('邀请链接格式无效，请重新生成')
  return new URL(path, location.origin).href
}
export const usersAPI = {
  load: async (signal?: AbortSignal): Promise<UsersSnapshot> => {
    const [userData, roleData] = await Promise.all([request('GET', '/users', undefined, signal), request('GET', '/roles', undefined, signal)])
    const users = parseUsers(userData), roles = parseRoles(roleData)
    if (users.some(user => user.role_id && !roles.some(role => role.public_id === user.role_id))) throw new Error('用户引用的角色不存在，请刷新重试')
    return { users, roles }
  },
  createUser: async (input: InviteFields) => inviteURL(await request('POST', '/users', { username: input.username.trim().toLowerCase(), display_name: input.display_name.trim(), role_id: input.role_id })),
  saveUser: (id: string, input: UserFields) => request('PATCH', '/users/' + publicID(id), { display_name: input.display_name.trim(), role_id: input.role_id, disabled: input.disabled }),
  resetPassword: async (id: string) => inviteURL(await request('POST', '/users/' + publicID(id) + '/reset-password')),
  revokeSessions: async (id: string) => integer(record(await request('POST', '/users/' + publicID(id) + '/revoke-sessions')), 'revoked', 0, Number.MAX_SAFE_INTEGER),
  createRole: (input: RoleFields) => request('POST', '/roles', rolePayload(input)),
  saveRole: (id: string, input: RoleFields) => request('PATCH', '/roles/' + publicID(id), rolePayload(input)),
  deleteRole: (id: string) => request('DELETE', '/roles/' + publicID(id)),
}
// A committed mutation must not be presented as failed merely because the following list read failed.
export async function reloadAfter<T>(mutation: () => Promise<T>) {
  const value = await mutation()
  try { return { value, snapshot: await usersAPI.load(), warning: '' } }
  catch (error) {
    if (error instanceof APIError && error.status === 401) throw error
    return { value, snapshot: null, warning: `操作已完成，列表刷新失败：${errorMessage(error)}。请刷新列表核对。` }
  }
}
