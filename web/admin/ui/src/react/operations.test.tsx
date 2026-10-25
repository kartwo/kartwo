// 客服与可观测测试 / Contact and Observability Tests
// 功能：验证私密留言切换、只读权限、状态写入及审计与诊断错误边界
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:17:21

import { Suspense } from 'react'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfigProvider } from 'antd'
import { MemoryRouter } from 'react-router-dom'
import { expect, it, vi } from 'vitest'
import { AdminRoot } from './AdminRoot'
import { AdminTheme } from './theme'
import { SessionProvider } from './SessionProvider'
import { SessionController } from './session'
import { businessPages } from './BusinessPages'
import { PageLoading } from './SettingsPages'
import { account, fakeAPI, deferred } from './test-fixtures'
import { contactAPI, parseMessage, parseContactList, type ContactMessage } from './contact-api'
import { parseAudit, parseDiagnostics, formatBytes, backupTime } from './observability-api'
import type { Session } from './contracts'

const alice: ContactMessage = { public_id: 'message-fixture-a', name: 'Alice Fixture', email: 'alice@example.com', topic: 'order', body: 'First private body', order_ref: 'order-fixture', ip_address: '203.0.113.1', country_code: 'US', status: 'new', created_at: '2026-10-25T01:00:00Z', updated_at: '', closed_at: '', anonymized_at: '', processing_by_name: '', processing_at: '', closed_by_name: '' }
const bob: ContactMessage = { ...alice, public_id: 'message-fixture-b', name: 'Bob Fixture', email: 'bob@example.com', body: 'Second private body' }
function diagnosticFixture() { return {
  database: { status: 'ok' }, media: { asset_count: 2, original_bytes: 1024, derivative_bytes: 2048, total_bytes: 3072 },
  disk: { available: true, total_bytes: 1000000000, free_bytes: 100000000, used_bytes: 900000000, health: 'critical', hint: 'Fixture disk protection', message: '' },
  backups: { automatic_count: 2, upgrade_count: 1, total_bytes: 10240, latest_at: null, message: '' },
} }
const event = { public_id: 'event-fixture-id', action: 'role.update', target_type: 'role', target_public_id: 'role-fixture-id', created_at: '2026-10-25', admin_username: 'owner-at-action-time' }
function response(value: unknown, status = 200) { return new Response(JSON.stringify(value), { status }) }
function mount(path: keyof typeof businessPages, session: Session = account()) {
  const controller = new SessionController(fakeAPI(session))
  render(<MemoryRouter initialEntries={[path]}><AdminTheme><ConfigProvider theme={{ token: { motion: false } }}><SessionProvider controller={controller}><AdminRoot renderPage={route => {
    const Page = businessPages[route.path as keyof typeof businessPages]
    return <Suspense fallback={<PageLoading />}><Page /></Suspense>
  }} /></SessionProvider></ConfigProvider></AdminTheme></MemoryRouter>)
  return controller
}
it('留言白名单校验，演示在解析内容前丢弃私密数据', () => {
  expect(parseMessage({ ...alice, debug_secret: 'do-not-keep' })).not.toHaveProperty('debug_secret')
  expect(() => parseMessage({ ...alice, status: 'unexpected' })).toThrow()
  expect(() => parseMessage({ ...alice, country_code: 'https://example.com' })).toThrow()
  expect(() => parseContactList({ messages: [alice, alice] }, false)).toThrow()
  expect(parseContactList({ messages: [alice], demo_hidden: false }, true)).toEqual({ messages: [], demo_hidden: true })
  expect(parseContactList({ messages: [{ arbitrary: 'private' }], demo_hidden: true }, false)).toEqual({ messages: [], demo_hidden: true })
})
it('留言详情和状态响应必须匹配对象，状态写入只有 status 字段', async () => {
  const fetch = vi.fn(async () => response(bob)); vi.stubGlobal('fetch', fetch)
  await expect(contactAPI.get(alice.public_id)).rejects.toThrow('当前选择不符')
  await expect(contactAPI.setStatus(alice.public_id, 'closed')).rejects.toThrow('当前选择不符')
  expect(fetch).toHaveBeenLastCalledWith('/admin/api/contact/messages/message-fixture-a', expect.objectContaining({ method: 'PATCH', body: '{"status":"closed"}' }))
  await expect(contactAPI.get('../private')).rejects.toThrow()
  expect(fetch).toHaveBeenCalledTimes(2)
})
it('快速切换留言只展示最后选择，旧响应即使晚返回也不能覆盖', async () => {
  const late = deferred<Response>()
  const fetch = vi.fn(async (url: string) => url.endsWith('/messages') ? response({ messages: [alice, bob] }) : url.endsWith('/message-fixture-a') ? late.promise : response(bob))
  vi.stubGlobal('fetch', fetch); mount('/contact')
  await screen.findByRole('heading', { name: '客服留言', level: 2 })
  await userEvent.setup().click(await screen.findByRole('button', { name: /Alice Fixture/ }))
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2))
  await userEvent.setup().click(screen.getByRole('button', { name: /Bob Fixture/ }))
  await screen.findByText('Second private body')
  await act(async () => late.resolve(response(alice)))
  expect(screen.getByText('Second private body')).toBeTruthy()
  expect(screen.queryByText('First private body')).toBeNull()
  expect(screen.queryByText('alice@example.com')).toBeNull()
})
it('只读客服员工可读资料，正文按文本显示，不提供写入按钮', async () => {
  const body = '<img src=x onerror="alert(1)">\nSecond line'
  const fetch = vi.fn(async (url: string) => response(url.endsWith('/messages') ? { messages: [alice] } : { ...alice, body })); vi.stubGlobal('fetch', fetch)
  mount('/contact', account('staff', ['contact.view']))
  await screen.findByRole('heading', { name: '客服留言', level: 2 })
  await userEvent.setup().click(await screen.findByRole('button', { name: /Alice Fixture/ }))
  await screen.findByText('alice@example.com')
  expect(screen.getByText(/<img src=x/).textContent).toBe(body)
  expect(document.querySelector('img[src="x"]')).toBeNull()
  expect(screen.queryByRole('button', { name: '标记处理中' })).toBeNull()
  expect(screen.queryByRole('button', { name: /结\s*案/ })).toBeNull()
  expect(screen.getByText('美国 (US)')).toBeTruthy()
})
it('处理状态保存锁定详情切换和重复提交，并更新处理人', async () => {
  const pending = deferred<Response>()
  const fetch = vi.fn(async (url: string, options: RequestInit) => options.method === 'PATCH' ? pending.promise : response(url.endsWith('/messages') ? { messages: [alice, bob] } : url.endsWith('/message-fixture-a') ? alice : bob))
  vi.stubGlobal('fetch', fetch); mount('/contact', account('staff', ['contact.view', 'contact.manage']))
  await userEvent.setup().click(await screen.findByRole('button', { name: /Alice Fixture/ }))
  await screen.findByText('First private body')
  const action = screen.getByRole('button', { name: '标记处理中' })
  await userEvent.setup().click(action)
  await waitFor(() => expect(fetch.mock.calls.filter(([, options]) => options.method === 'PATCH')).toHaveLength(1))
  await userEvent.setup().click(action)
  await userEvent.setup().click(screen.getByRole('button', { name: /Bob Fixture/ }))
  expect(fetch).toHaveBeenCalledTimes(3)
  await act(async () => pending.resolve(response({ ...alice, status: 'processing', processing_by_name: 'Operator Fixture', processing_at: '2026-10-25T02:00:00Z' })))
  await screen.findByText('处理状态已更新')
  expect(screen.getByText(/Operator Fixture · 2026/)).toBeTruthy()
  expect(fetch).toHaveBeenCalledTimes(3)
})
it('匿名化留言仍可查看留存记录但不能修改状态', async () => {
  const anonymous = { ...alice, name: '', email: '', body: '', ip_address: '', country_code: '', status: 'closed', anonymized_at: '2026-10-25' }
  vi.stubGlobal('fetch', vi.fn(async (url: string) => response(url.endsWith('/messages') ? { messages: [anonymous] } : anonymous)))
  mount('/contact')
  await userEvent.setup().click(await screen.findByRole('button', { name: /已匿名化/ }))
  await screen.findByText('此留言已到期匿名化，无法再修改处理状态。')
  expect(screen.queryByRole('button', { name: /重\s*开/ })).toBeNull()
  expect(screen.getByText('未知（未获得可信定位）')).toBeTruthy()
})
it('演示忽略错误返回的真实留言，不提供详情读取', async () => {
  const fetch = vi.fn(async () => response({ messages: [alice, bob], demo_hidden: false })); vi.stubGlobal('fetch', fetch)
  mount('/contact', account('demo'))
  await screen.findByText(/公开演示不会展示真实顾客留言/)
  expect(screen.queryByText(/Alice Fixture/)).toBeNull()
  expect(screen.queryByText(/alice@example/)).toBeNull()
  expect(fetch).toHaveBeenCalledTimes(1)
})
it('留言状态筛选精确传参，空结果正常展示', async () => {
  const fetch = vi.fn(async (url: string) => response(url.includes('?status=closed') ? { messages: [] } : { messages: [alice] })); vi.stubGlobal('fetch', fetch)
  mount('/contact'); await screen.findByRole('button', { name: /Alice Fixture/ })
  const select = screen.getByRole('combobox', { name: '留言状态' })
  await userEvent.setup().click(select)
  await userEvent.setup().click(await screen.findByText('已结案', { exact: true }))
  await waitFor(() => expect(fetch).toHaveBeenLastCalledWith('/admin/api/contact/messages?status=closed', expect.objectContaining({ method: 'GET' })))
  await screen.findByText('暂无留言')
})
it('留言刷新失败后清除已读详情并可重试，不把读取错误当空列表', async () => {
  let fail = false
  const fetch = vi.fn(async (url: string) => url.endsWith('/messages') ? fail ? response({ error: '收件箱读取失败' }, 500) : response({ messages: [alice] }) : response(alice))
  vi.stubGlobal('fetch', fetch); mount('/contact')
  await userEvent.setup().click(await screen.findByRole('button', { name: /Alice Fixture/ }))
  await screen.findByText('First private body')
  fail = true; await userEvent.setup().click(screen.getByRole('button', { name: '刷新留言' }))
  await screen.findByText('收件箱读取失败')
  expect(screen.queryByText('First private body')).toBeNull()
  expect(screen.queryByText('暂无留言')).toBeNull()
  fail = false; await userEvent.setup().click(screen.getByRole('button', { name: /重\s*试/ }))
  await screen.findByRole('button', { name: /Alice Fixture/ })
  expect(screen.queryByText('First private body')).toBeNull()
})
it('状态写入失败保留当前留言和原状态，便于重新读取核对', async () => {
  vi.stubGlobal('fetch', vi.fn(async (url: string, options: RequestInit) => options.method === 'PATCH' ? response({ error: '留言已是该状态，请刷新页面' }, 409) : response(url.endsWith('/messages') ? { messages: [alice] } : alice)))
  mount('/contact')
  await userEvent.setup().click(await screen.findByRole('button', { name: /Alice Fixture/ }))
  await screen.findByText('First private body')
  await userEvent.setup().click(screen.getByRole('button', { name: '标记处理中' }))
  await screen.findByText('留言已是该状态，请刷新页面')
  expect(screen.getByText('First private body')).toBeTruthy()
  expect(screen.queryByText('处理状态已更新')).toBeNull()
})
it('留言写入 401 清理会话与私密资料', async () => {
  const fetch = vi.fn(async (url: string, options: RequestInit) => options.method === 'PATCH' ? response({ error: '登录过期' }, 401) : response(url.endsWith('/messages') ? { messages: [alice] } : alice))
  vi.stubGlobal('fetch', fetch); const controller = mount('/contact')
  await userEvent.setup().click(await screen.findByRole('button', { name: /Alice Fixture/ }))
  await screen.findByText('First private body')
  await userEvent.setup().click(screen.getByRole('button', { name: '标记处理中' }))
  await waitFor(() => expect(controller.getSnapshot().phase).toBe('login'))
  expect(screen.queryByText('First private body')).toBeNull()
  expect(screen.queryByText('alice@example.com')).toBeNull()
})
it('审计响应只保留必要字段，演示遮蔽用户和对象，未知动作仍为文本', () => {
  expect(parseAudit({ events: [{ ...event, request_body: 'do-not-retain', session_token: 'do-not-retain' }] })[0]).toEqual(event)
  expect(parseAudit({ events: [event] }, true)[0]).toEqual({ ...event, public_id: 'demo-event-1', admin_username: '店主', target_public_id: '已隐藏' })
  expect(() => parseAudit({ events: [event, event] })).toThrow()
})
it('审计保留操作时用户名和中文动作，可刷新；未知动作安全转义', async () => {
  const fetch = vi.fn(async () => response({ events: [event, { ...event, public_id: 'event-fixture-2', action: '<img src=x>', target_public_id: '' }] })); vi.stubGlobal('fetch', fetch)
  mount('/audit', account('staff', ['audit.view']))
  await screen.findByText('更新角色权限')
  expect(screen.getAllByText('owner-at-action-time')).toHaveLength(2)
  expect(screen.getByText('<img src=x>')).toBeTruthy()
  expect(document.querySelector('img[src="x"]')).toBeNull()
  await userEvent.setup().click(screen.getByRole('button', { name: '刷新日志' }))
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2))
})
it('审计读取失败显示错误而不是空列表，重试后恢复', async () => {
  const fetch = vi.fn().mockResolvedValueOnce(response({ error: '日志暂不可用' }, 503)).mockResolvedValueOnce(response({ events: [] }))
  vi.stubGlobal('fetch', fetch); mount('/audit')
  await screen.findByText('日志暂不可用'); expect(screen.queryByText('尚无日志记录')).toBeNull()
  await userEvent.setup().click(screen.getByRole('button', { name: /重\s*试/ }))
  await screen.findByText('尚无日志记录')
})
it('诊断严格校验状态和容量，保留未知备份时间与零值语义', () => {
  const fixture = diagnosticFixture()
  expect(() => parseDiagnostics({ ...fixture, database: { status: 'failed' } })).toThrow()
  expect(() => parseDiagnostics({ ...fixture, disk: { ...fixture.disk, health: 'unknown' } })).toThrow()
  expect(() => parseDiagnostics({ ...fixture, media: { ...fixture.media, asset_count: -1 } })).toThrow()
  expect(parseDiagnostics(fixture).backups.latest_at).toBeNull()
  expect(formatBytes(0)).toBe('0 B'); expect(formatBytes(1024)).toBe('1.0 KB')
  expect(backupTime(null)).toBe('尚未生成'); expect(backupTime('invalid')).toBe('暂不可用')
})
it('诊断展示磁盘保护和备份状态，刷新失败不保留绿色正常项', async () => {
  const fetch = vi.fn().mockResolvedValueOnce(response(diagnosticFixture())).mockResolvedValueOnce(response({ error: '数据库暂不可用' }, 503))
  vi.stubGlobal('fetch', fetch); mount('/diagnostics')
  await screen.findByText('磁盘空间紧急')
  expect(screen.getByText('Fixture disk protection')).toBeTruthy()
  expect(screen.getByText('尚未生成')).toBeTruthy()
  await userEvent.setup().click(screen.getByRole('button', { name: '刷新诊断' }))
  await screen.findByText('数据库暂不可用')
  expect(screen.queryByText('正常')).toBeNull()
})
it('磁盘探测失败不隐藏其他诊断，备份读失败也明确说明', async () => {
  const fixture = diagnosticFixture()
  vi.stubGlobal('fetch', vi.fn(async () => response({ ...fixture, disk: { ...fixture.disk, available: false, health: '', message: '磁盘探测失败' }, backups: { ...fixture.backups, message: '部分备份不可读' } })))
  mount('/diagnostics', account('staff', ['diagnostics.view']))
  await screen.findByText('磁盘探测失败')
  expect(screen.getByText('部分备份不可读')).toBeTruthy()
  expect(screen.getByText('媒体文件')).toBeTruthy()
  expect(screen.getByText('正常')).toBeTruthy()
})
it.each(['/contact', '/audit', '/diagnostics'] as const)('%s 无相应权限时不挂载或请求资料', async path => {
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch); mount(path, account('staff'))
  await screen.findByText('当前账号没有访问此页面的权限')
  expect(fetch).not.toHaveBeenCalled()
})
