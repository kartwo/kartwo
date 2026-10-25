// 用户角色交互测试 / Users and Roles Interaction Tests
// 功能：验证预设改权、权限依赖、一次性邀请、会话撤销及店主权限边界
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:09:08

import { Suspense } from 'react'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfigProvider } from 'antd'
import { MemoryRouter } from 'react-router-dom'
import { expect, it, vi } from 'vitest'
import { AdminRoot } from './AdminRoot'
import { AdminTheme } from './theme'
import { SessionProvider } from './SessionProvider'
import { SessionController } from './session'
import { settingsPages, PageLoading } from './SettingsPages'
import { account, fakeAPI, deferred } from './test-fixtures'
import { parseUsers, parseRoles, permissionError, inviteURL, usersAPI, type ManagedRole } from './users-api'
import type { Session } from './contracts'

const viewer: ManagedRole = { public_id: 'role-viewer-fixture', name: '只读', permissions: ['dashboard.view', 'products.view', 'orders.view'], system_key: 'viewer', user_count: 1 }
const custom: ManagedRole = { public_id: 'role-custom-fixture', name: 'Custom Fixture', permissions: ['blog.view'], system_key: '', user_count: 0 }
const owner = { public_id: 'owner-fixture-id', username: 'fixture-owner', display_name: 'Owner', role_id: '', role_name: '店主', status: 'active', last_login_at: '', created_at: '2026-10-25' }
const staff = { public_id: 'staff-fixture-id', username: 'fixture-staff', display_name: 'Staff', role_id: viewer.public_id, role_name: viewer.name, status: 'active', last_login_at: '', created_at: '2026-10-25' }
const invitePath = '/admin/#/accept-invite?token=' + 'fixture'.repeat(6)
function response(value: unknown, status = 200) { return new Response(JSON.stringify(value), { status }) }
function fixtureFetch() {
  return vi.fn(async (url: string, options: RequestInit) => response(options.method === 'GET' ? url.endsWith('/users') ? [owner, staff] : [viewer, custom] : { ok: true }))
}
function mount(session: Session = account()) {
  const controller = new SessionController(fakeAPI(session))
  render(<MemoryRouter initialEntries={['/users']}><AdminTheme><ConfigProvider theme={{ token: { motion: false } }}><SessionProvider controller={controller}><AdminRoot renderPage={() => {
    const Page = settingsPages['/users']; return <Suspense fallback={<PageLoading />}><Page /></Suspense>
  }} /></SessionProvider></ConfigProvider></AdminTheme></MemoryRouter>)
  return controller
}
function card(title: string) { return screen.getByText(title, { exact: true, selector: '.ant-card-head-title' }).closest('.ant-card') as HTMLElement }
function submitInside(panel: HTMLElement, label: string) { fireEvent.submit(within(panel).getByLabelText(label).closest('form')!) }

it('用户角色边界拒绝重复 ID、未知状态、非数组及不可分配权限', () => {
  expect(() => parseUsers([owner, owner])).toThrow()
  expect(() => parseUsers([{ ...staff, status: 'unknown' }])).toThrow()
  expect(() => parseRoles({ roles: [viewer] })).toThrow()
  expect(() => parseRoles([{ ...viewer, permissions: ['orders.refund'] }])).toThrow()
  expect(() => parseRoles([{ ...viewer, permissions: ['new.unknown'] }])).toThrow()
  expect(() => parseRoles([{ ...viewer, user_count: 1.5 }])).toThrow()
})
it('角色依赖按实际服务端规则校验，绝不自动添加店主权限', () => {
  for (const value of [[], ['products.edit'], ['categories.manage'], ['orders.customer_data'], ['orders.view', 'orders.fulfill'], ['contact.manage'], ['blog.manage'], ['users.manage']]) expect(permissionError(value)).not.toBe('')
  expect(permissionError(['products.view', 'products.inventory', 'orders.view', 'orders.customer_data', 'orders.fulfill', 'blog.view', 'blog.manage'])).toBe('')
})
it('邀请地址只允许本站固定入口及合法令牌，不接受外站和额外参数', () => {
  expect(inviteURL({ invite_path: invitePath })).toBe(location.origin + invitePath)
  for (const invite_path of ['https://example.com/' + invitePath, '//example.com/a', 'javascript:alert(1)', invitePath + '&redirect=https://example.com', '/admin/#/accept-invite?token=short']) expect(() => inviteURL({ invite_path })).toThrow()
})
it('员工保存精确提交，敏感操作使用公开 ID，拒绝路径穿越', async () => {
  const fetch = fixtureFetch(); vi.stubGlobal('fetch', fetch)
  await usersAPI.saveUser(staff.public_id, { display_name: ' New Staff ', role_id: custom.public_id, disabled: true })
  expect(fetch).toHaveBeenCalledWith('/admin/api/users/staff-fixture-id', expect.objectContaining({ method: 'PATCH', body: JSON.stringify({ display_name: 'New Staff', role_id: custom.public_id, disabled: true }) }))
  expect(() => usersAPI.saveUser('../owner', { display_name: '', role_id: '', disabled: true })).toThrow()
})
it.each(['staff', 'demo'] as const)('%s 即使持有管理权限也不能挂载用户页', async role => {
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch); mount(account(role, ['users.manage', 'roles.manage', 'sessions.manage']))
  await screen.findByText('当前账号没有访问此页面的权限')
  expect(fetch).not.toHaveBeenCalled()
})
it('预设角色权限可编辑、名称和删除受限，店主账号无员工操作', async () => {
  const fetch = fixtureFetch(); vi.stubGlobal('fetch', fetch); mount()
  await screen.findByText('fixture-owner')
  expect(within(card('fixture-owner')).queryByRole('button', { name: '保存员工' })).toBeNull()
  await userEvent.setup().click(screen.getByRole('tab', { name: '角色与权限' }))
  const panel = card('只读')
  expect((within(panel).getByLabelText('角色名称') as HTMLInputElement).disabled).toBe(true)
  expect(within(panel).queryByRole('button', { name: '删除角色' })).toBeNull()
  await userEvent.setup().click(within(panel).getByRole('checkbox', { name: '查看博客文章' }))
  await userEvent.setup().click(within(panel).getByRole('button', { name: '保存权限' }))
  await screen.findByText('角色已更新；相关员工需重新登录')
  const write = fetch.mock.calls.find(([, options]) => options.method === 'PATCH')
  expect(write?.[0]).toBe('/admin/api/roles/role-viewer-fixture')
  const payload = JSON.parse(write?.[1].body as string) as { name: string; permissions: string[] }
  expect(payload.name).toBe('只读')
  expect([...payload.permissions].sort()).toEqual(['dashboard.view', 'products.view', 'orders.view', 'blog.view'].sort())
})
it('自定义角色发货依赖不完整时阻止提交，错误保留选择', async () => {
  const fetch = fixtureFetch(); vi.stubGlobal('fetch', fetch); mount(); await screen.findByText('fixture-owner')
  await userEvent.setup().click(screen.getByRole('tab', { name: '角色与权限' }))
  const panel = card('新建自定义角色')
  fireEvent.change(within(panel).getByLabelText('角色名称'), { target: { value: 'Fulfillment' } })
  await userEvent.setup().click(within(panel).getByRole('checkbox', { name: '发货' }))
  submitInside(panel, '角色名称')
  await screen.findByText('发货权限需要同时勾选「查看订单」和「查看客户信息」')
  expect(fetch.mock.calls.every(([, options]) => options.method === 'GET')).toBe(true)
  expect((within(panel).getByRole('checkbox', { name: '发货' }) as HTMLInputElement).checked).toBe(true)
})
it('自定义角色创建只提交名称和选择的权限，成功清空创建表单', async () => {
  const fetch = fixtureFetch(); vi.stubGlobal('fetch', fetch); mount(); await screen.findByText('fixture-owner')
  await userEvent.setup().click(screen.getByRole('tab', { name: '角色与权限' }))
  const panel = card('新建自定义角色')
  fireEvent.change(within(panel).getByLabelText('角色名称'), { target: { value: ' Blog Team ' } })
  await userEvent.setup().click(within(panel).getByRole('checkbox', { name: '查看博客文章' }))
  await userEvent.setup().click(within(panel).getByRole('checkbox', { name: '管理博客文章' }))
  submitInside(panel, '角色名称')
  await screen.findByText('角色已创建')
  expect(fetch.mock.calls.find(([, options]) => options.method === 'POST')?.[1].body).toBe('{"name":"Blog Team","permissions":["blog.view","blog.manage"]}')
  expect((within(panel).getByLabelText('角色名称') as HTMLInputElement).value).toBe('')
  expect((within(panel).getByRole('checkbox', { name: '查看博客文章' }) as HTMLInputElement).checked).toBe(false)
})
it('邀请创建成功后列表读取失败仍保留一次性链接及成功状态，不提交密码', async () => {
  let created = false
  const fetch = vi.fn(async (url: string, options: RequestInit) => {
    if (options.method === 'POST') { created = true; return response({ public_id: 'new-user-fixture', invite_path: invitePath }, 201) }
    if (created && url.endsWith('/users')) return response({ error: '刷新失败' }, 500)
    return response(url.endsWith('/users') ? [owner, staff] : [viewer, custom])
  }); vi.stubGlobal('fetch', fetch); mount(); await screen.findByText('fixture-owner')
  const panel = card('邀请员工')
  fireEvent.change(within(panel).getByLabelText('登录用户名'), { target: { value: ' NewStaff ' } })
  submitInside(panel, '登录用户名')
  const link = await screen.findByLabelText('邀请链接') as HTMLInputElement
  expect(link.value).toBe(location.origin + invitePath)
  await screen.findByText(/操作已完成，列表刷新失败/)
  expect(fetch.mock.calls.find(([, options]) => options.method === 'POST')?.[1].body).toBe(JSON.stringify({ username: 'newstaff', display_name: '', role_id: viewer.public_id }))
  expect((within(panel).getByLabelText('登录用户名') as HTMLInputElement).value).toBe('')
  await userEvent.setup().click(screen.getByRole('button', { name: '隐藏链接' }))
  expect(screen.queryByLabelText('邀请链接')).toBeNull()
})
it('员工停用保存期间阻止重复提交，精确提示旧会话撤销', async () => {
  const pending = deferred<Response>()
  const fetch = fixtureFetch(); fetch.mockImplementation(async (url: string, options: RequestInit) => options.method === 'PATCH' ? pending.promise : response(url.endsWith('/users') ? [owner, staff] : [viewer, custom]))
  vi.stubGlobal('fetch', fetch); mount(); await screen.findByText('fixture-staff')
  const panel = card('fixture-staff')
  await userEvent.setup().click(within(panel).getByRole('switch', { name: '停用员工' }))
  submitInside(panel, '显示名称')
  await waitFor(() => expect(fetch.mock.calls.filter(([, options]) => options.method === 'PATCH')).toHaveLength(1))
  submitInside(panel, '显示名称'); await act(async () => {})
  expect(fetch.mock.calls.filter(([, options]) => options.method === 'PATCH')).toHaveLength(1)
  await act(async () => pending.resolve(response({ ok: true })))
  await screen.findByText('账号已更新，旧会话已撤销')
  expect(JSON.parse(fetch.mock.calls.find(([, options]) => options.method === 'PATCH')?.[1].body as string)).toEqual({ display_name: 'Staff', role_id: viewer.public_id, disabled: true })
})
it('重置密码和退出设备都须确认，取消不发请求', async () => {
  const fetch = vi.fn(async (url: string, options: RequestInit) => response(options.method === 'GET' ? url.endsWith('/users') ? [owner, staff] : [viewer, custom] : url.endsWith('/reset-password') ? { invite_path: invitePath } : { revoked: 2 }))
  vi.stubGlobal('fetch', fetch); mount(); await screen.findByText('fixture-staff')
  const panel = card('fixture-staff')
  await userEvent.setup().click(within(panel).getByRole('button', { name: '重置密码' }))
  let dialog = await screen.findByRole('dialog', { name: '重置员工密码' })
  await userEvent.setup().click(within(dialog).getByRole('button', { name: /取\s*消/ }))
  expect(fetch).toHaveBeenCalledTimes(2)
  await userEvent.setup().click(within(panel).getByRole('button', { name: '重置密码' }))
  dialog = await screen.findByRole('dialog', { name: '重置员工密码' })
  await userEvent.setup().click(within(dialog).getByRole('button', { name: '生成链接' }))
  await screen.findByLabelText('邀请链接')
  await userEvent.setup().click(within(card('fixture-staff')).getByRole('button', { name: '退出设备' }))
  dialog = await screen.findByRole('dialog', { name: '撤销员工会话' })
  await userEvent.setup().click(within(dialog).getByRole('button', { name: /撤\s*销/ }))
  await screen.findByText('已撤销 2 个会话')
  expect(fetch.mock.calls.filter(([, options]) => options.method === 'POST').map(([url]) => url)).toEqual(['/admin/api/users/staff-fixture-id/reset-password', '/admin/api/users/staff-fixture-id/revoke-sessions'])
})
it('自定义角色删除先确认，预设角色不会出现删除入口', async () => {
  const fetch = fixtureFetch(); vi.stubGlobal('fetch', fetch); mount(); await screen.findByText('fixture-owner')
  await userEvent.setup().click(screen.getByRole('tab', { name: '角色与权限' }))
  await userEvent.setup().click(within(card('Custom Fixture')).getByRole('button', { name: '删除角色' }))
  const dialog = await screen.findByRole('dialog', { name: '删除角色' })
  await userEvent.setup().click(within(dialog).getByRole('button', { name: /删\s*除/ }))
  await screen.findByText('角色已删除')
  expect(fetch.mock.calls.some(([url, options]) => url === '/admin/api/roles/role-custom-fixture' && options.method === 'DELETE')).toBe(true)
})
it('删除角色冲突时在确认框内显示服务端错误，保留取消和重试操作', async () => {
  const fetch = vi.fn(async (url: string, options: RequestInit) => options.method === 'GET' ? response(url.endsWith('/users') ? [owner, staff] : [viewer, custom]) : response({ error: '角色仍有用户' }, 409))
  vi.stubGlobal('fetch', fetch); mount(); await screen.findByText('fixture-owner')
  await userEvent.setup().click(screen.getByRole('tab', { name: '角色与权限' }))
  await userEvent.setup().click(within(card('Custom Fixture')).getByRole('button', { name: '删除角色' }))
  const dialog = await screen.findByRole('dialog', { name: '删除角色' })
  await userEvent.setup().click(within(dialog).getByRole('button', { name: /删\s*除/ }))
  expect(await within(dialog).findByText('角色仍有用户')).toBeTruthy()
  expect(within(dialog).getByRole('button', { name: /取\s*消/ })).toBeTruthy()
})
it('读取失败无可写用户表单，重试后恢复；401 清除会话', async () => {
  let failed = true
  const fetch = vi.fn(async (url: string, options: RequestInit) => {
    if (options.method !== 'GET') return response({ error: '会话过期' }, 401)
    if (failed) return response({ error: '读取失败' }, 500)
    return response(url.endsWith('/users') ? [owner, staff] : [viewer, custom])
  }); vi.stubGlobal('fetch', fetch); const controller = mount()
  await screen.findByText('读取失败'); expect(screen.queryByLabelText('登录用户名')).toBeNull()
  failed = false; await userEvent.setup().click(screen.getByRole('button', { name: /重\s*试/ }))
  await screen.findByText('fixture-staff'); submitInside(card('fixture-staff'), '显示名称')
  await waitFor(() => expect(controller.getSnapshot().phase).toBe('login'))
  expect(screen.queryByText('fixture-staff')).toBeNull()
  expect(screen.queryByLabelText('邀请链接')).toBeNull()
})
