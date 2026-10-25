// 安全配置交互测试 / Security Settings Interaction Tests
// 功能：验证改密退出、秘密清空及备份逐字段只读与保存测试顺序
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:55:07

import { Suspense } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { expect, it, vi } from 'vitest'
import { AdminRoot } from './AdminRoot'
import { AdminTheme } from './theme'
import { SessionProvider } from './SessionProvider'
import { SessionController } from './session'
import { settingsPages, PageLoading } from './SettingsPages'
import { account, deferred, fakeAPI } from './test-fixtures'
import { backupKeys, parseBackup, parseTranslation, backupPayload, securitySettingsAPI, type BackupFields } from './security-settings-api'
import type { Session } from './contracts'

function mount(path: keyof typeof settingsPages, session: Session = account()) {
  const controller = new SessionController(fakeAPI(session))
  render(<MemoryRouter initialEntries={[path]}><AdminTheme><SessionProvider controller={controller}><AdminRoot renderPage={route => {
    const Page = settingsPages[route.path as keyof typeof settingsPages]
    return <Suspense fallback={<PageLoading />}><Page /></Suspense>
  }} /></SessionProvider></AdminTheme></MemoryRouter>)
  return controller
}
function response(data: unknown, status = 200) { return new Response(JSON.stringify(data), { status }) }
function submit(label: string) { fireEvent.submit(screen.getByLabelText(label).closest('form')!) }
function change(label: string, value: string) { fireEvent.change(screen.getByLabelText(label), { target: { value } }) }
const translation = { provider: 'deepl', plan: 'developer', has_api_key: true, price_note: 'Fixture price note', developer_quota: 'Fixture quota', growth_quota: 'Fixture growth quota' }
function backupFixture() {
  const data: Record<string, unknown> = { demo_readonly: false, interval: '24h', retention: 7, webdav_enabled: 'true', webdav_url: 'https://dav.example.com', webdav_path: '/backups', webdav_username: 'fixture-user', webdav_password_set: true, restart_required: true }
  for (const key of backupKeys) { data[`${key}_readonly`] = false; if (key !== 'webdav_password') data[`${key}_source`] = 'db' }
  return data
}
const backupFields: BackupFields = { interval: '24h', retention: '7', webdav_enabled: true, webdav_url: 'https://dav.example.com', webdav_path: '/backups', webdav_username: 'fixture-user', webdav_password: '' }

it('备份只提交可写字段，密码空白保留且留空省略；响应不保留秘密', () => {
  const raw = backupFixture(); raw.interval_source = 'env'; raw.webdav_url_readonly = true
  const parsed = parseBackup({ ...raw, webdav_password: 'must-not-retain' })
  expect(parsed).not.toHaveProperty('webdav_password')
  expect(backupPayload(backupFields, parsed)).toEqual({ retention: '7', webdav_enabled: true, webdav_path: '/backups', webdav_username: 'fixture-user' })
  expect(backupPayload({ ...backupFields, webdav_password: ' fixture-password ' }, parsed).webdav_password).toBe(' fixture-password ')
  expect(backupPayload(backupFields, parseBackup({ ...raw, demo_readonly: true }))).toEqual({})
  expect(() => parseBackup({ ...raw, webdav_enabled: 'yes' })).toThrow()
  expect(() => parseBackup({ ...raw, retention: 1.5 })).toThrow()
  expect(() => parseBackup({ ...raw, interval_source: 'unknown' })).toThrow()
  expect(() => parseTranslation({ ...translation, provider: 'unknown' })).toThrow()
  expect(parseTranslation({ ...translation, api_key: 'must-not-retain' })).not.toHaveProperty('api_key')
})

it('店主仅修改用户名时保留密码空值，密码不裁剪且不提交确认字段', async () => {
  const fetch = vi.fn(async () => response({ ok: true, signed_out: true })); vi.stubGlobal('fetch', fetch)
  await securitySettingsAPI.saveOwner({ username: ' new-owner ', current_password: ' fixture-current ', new_password: '' })
  expect(fetch).toHaveBeenCalledWith('/admin/api/account', expect.objectContaining({ method: 'PUT', body: JSON.stringify({ username: 'new-owner', current_password: ' fixture-current ', new_password: '' }) }))
  await securitySettingsAPI.saveStaff({ username: 'ignored', current_password: ' fixture-current ', new_password: ' fixture-new ' })
  expect(fetch).toHaveBeenLastCalledWith('/admin/api/account/password', expect.objectContaining({ body: JSON.stringify({ current_password: ' fixture-current ', new_password: ' fixture-new ' }) }))
})

it('店主改名成功立即清除会话与密码表单', async () => {
  const fetch = vi.fn(async (_url: string, options: RequestInit) => response(options.method === 'GET' ? { username: 'fixture', readonly: false } : { ok: true, signed_out: true }))
  vi.stubGlobal('fetch', fetch)
  const controller = mount('/account')
  await screen.findByLabelText('管理员用户名')
  change('管理员用户名', 'New Owner')
  change('当前密码', 'fixture-current')
  submit('管理员用户名')
  await waitFor(() => expect(controller.getSnapshot().phase).toBe('login'))
  expect(screen.queryByLabelText('当前密码')).toBeNull()
  expect(fetch).toHaveBeenCalledTimes(2)
  expect(JSON.parse(fetch.mock.calls[1]?.[1].body as string)).toEqual({ username: 'New Owner', current_password: 'fixture-current', new_password: '' })
})

it('员工只改自己的密码，确认不一致拒绝，成功不调用店主接口', async () => {
  const fetch = vi.fn(async (_url: string, options: RequestInit) => response(options.method === 'GET' ? { username: 'fixture-staff', readonly: true } : { ok: true }))
  vi.stubGlobal('fetch', fetch)
  const controller = mount('/account', account('staff'))
  const username = await screen.findByLabelText('管理员用户名') as HTMLInputElement
  expect(username.disabled).toBe(true)
  change('当前密码', 'fixture-current'); change('新密码', 'fixture-new-password'); change('确认新密码', 'mismatch')
  submit('当前密码')
  await screen.findByText('两次输入的新密码不一致')
  expect(fetch).toHaveBeenCalledTimes(1)
  change('确认新密码', 'fixture-new-password')
  submit('当前密码')
  await waitFor(() => expect(controller.getSnapshot().phase).toBe('login'))
  expect(fetch).toHaveBeenLastCalledWith('/admin/api/account/password', expect.objectContaining({ method: 'PUT', body: JSON.stringify({ current_password: 'fixture-current', new_password: 'fixture-new-password' }) }))
})

it('当前密码错误保留会话并显示错误', async () => {
  const fetch = vi.fn(async (_url: string, options: RequestInit) => options.method === 'GET' ? response({ username: 'fixture', readonly: false }) : response({ error: '当前密码错误' }, 403))
  vi.stubGlobal('fetch', fetch)
  const controller = mount('/account')
  await screen.findByLabelText('当前密码'); change('当前密码', 'wrong-password')
  submit('管理员用户名')
  await screen.findByText('当前密码错误')
  expect(controller.getSnapshot().phase).toBe('ready')
})
it('演示无法修改账号，即使服务器误给可写标识', async () => {
  const fetch = vi.fn(async () => response({ username: 'fixture-hidden', readonly: false })); vi.stubGlobal('fetch', fetch)
  mount('/account', account('demo'))
  expect((await screen.findByLabelText('管理员用户名') as HTMLInputElement).disabled).toBe(true)
  expect(screen.queryByLabelText('当前密码')).toBeNull()
  submit('管理员用户名'); await act(async () => {})
  expect(fetch).toHaveBeenCalledTimes(1)
})

it('翻译密钥空值保留、保存后清空；只配置不自动调用翻译', async () => {
  const fetch = vi.fn(async () => response(translation)); vi.stubGlobal('fetch', fetch)
  mount('/translation')
  await screen.findByLabelText('DeepL API Key')
  change('DeepL API Key', ' fixture-translation-key ')
  submit('DeepL API Key')
  await screen.findByText('DeepL 配置已保存')
  expect((screen.getByLabelText('DeepL API Key') as HTMLInputElement).value).toBe('')
  expect(fetch).toHaveBeenLastCalledWith('/admin/api/settings/translation', expect.objectContaining({ method: 'PUT', body: JSON.stringify({ plan: 'developer', api_key: 'fixture-translation-key' }) }))
  submit('DeepL API Key')
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(3))
  expect(fetch).toHaveBeenLastCalledWith('/admin/api/settings/translation', expect.objectContaining({ body: '{"plan":"developer","api_key":""}' }))
})

it('备份环境只读字段不阻止其他字段保存，密码不回填，重启要求可见', async () => {
  const data = backupFixture(); data.interval_readonly = true; data.interval_source = 'env'; data.webdav_url_source = 'env'
  const fetch = vi.fn(async () => response(data)); vi.stubGlobal('fetch', fetch)
  mount('/backup')
  expect((await screen.findByLabelText('备份周期') as HTMLInputElement).disabled).toBe(true)
  expect((screen.getByLabelText('WebDAV 地址（HTTPS）') as HTMLInputElement).disabled).toBe(true)
  change('保留自动备份份数', '30'); change('WebDAV 密码 / 授权码', ' fixture-webdav-password ')
  submit('保留自动备份份数')
  await screen.findByText('自动备份设置已保存；本地策略重启后生效')
  expect(fetch).toHaveBeenLastCalledWith('/admin/api/settings/backup', expect.objectContaining({ method: 'PUT', body: JSON.stringify({ retention: '30', webdav_enabled: true, webdav_path: '/backups', webdav_username: 'fixture-user', webdav_password: ' fixture-webdav-password ' }) }))
  expect((screen.getByLabelText('WebDAV 密码 / 授权码') as HTMLInputElement).value).toBe('')
})

it('备份先保存再测试，保存期间阻止重复；测试失败仍清空已保存密码', async () => {
  const pending = deferred<Response>(), calls: string[] = []
  const fetch = vi.fn(async (url: string, options: RequestInit) => {
    calls.push(`${options.method} ${url}`)
    if (options.method === 'GET') return response(backupFixture())
    if (options.method === 'PUT') return pending.promise
    return response({ error: '目标不可达' }, 502)
  }); vi.stubGlobal('fetch', fetch)
  mount('/backup')
  await screen.findByLabelText('WebDAV 密码 / 授权码')
  change('WebDAV 密码 / 授权码', 'fixture-new-password')
  const button = screen.getByRole('button', { name: '保存并测试连接' })
  await userEvent.setup().click(button)
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2))
  await userEvent.setup().click(button); submit('备份周期')
  await act(async () => {})
  expect(fetch).toHaveBeenCalledTimes(2)
  await act(async () => pending.resolve(response(backupFixture())))
  await screen.findByText('配置已保存，连接测试失败：目标不可达')
  expect(calls).toEqual(['GET /admin/api/settings/backup', 'PUT /admin/api/settings/backup', 'POST /admin/api/settings/backup/test'])
  expect((screen.getByLabelText('WebDAV 密码 / 授权码') as HTMLInputElement).value).toBe('')
})

it('全环境只读备份允许店主仅测试，绝不先写配置', async () => {
  const data = backupFixture(); for (const key of backupKeys) data[`${key}_readonly`] = true
  const fetch = vi.fn(async (_url: string, options: RequestInit) => response(options.method === 'GET' ? data : { ok: true })); vi.stubGlobal('fetch', fetch)
  mount('/backup')
  await screen.findByLabelText('备份周期')
  expect(screen.queryByRole('button', { name: '保存备份设置' })).toBeNull()
  await userEvent.setup().click(screen.getByRole('button', { name: '测试已配置连接' }))
  await screen.findByText('WebDAV 测试成功：可连通并可写入')
  expect(fetch).toHaveBeenCalledTimes(2)
  expect(fetch).toHaveBeenLastCalledWith('/admin/api/settings/backup/test', expect.objectContaining({ method: 'POST' }))
})

it.each(['/translation', '/backup'] as const)('%s 演示只读且不能提交', async path => {
  const fetch = vi.fn(async () => response(path === '/backup' ? backupFixture() : translation)); vi.stubGlobal('fetch', fetch)
  mount(path, account('demo'))
  await screen.findByLabelText(path === '/backup' ? '备份周期' : 'DeepL API Key')
  submit(path === '/backup' ? '备份周期' : 'DeepL API Key')
  await act(async () => {})
  expect(fetch).toHaveBeenCalledTimes(1)
  if (path === '/backup') expect((screen.getByRole('button', { name: '测试已配置连接' }) as HTMLButtonElement).disabled).toBe(true)
})

it.each(['/translation', '/backup'] as const)('%s 员工即使持有伪造设置权限也不能挂载', async path => {
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch)
  mount(path, account('staff', ['translation.manage', 'settings.backup']))
  expect(await screen.findByText('当前账号没有访问此页面的权限')).toBeTruthy()
  expect(fetch).not.toHaveBeenCalled()
})

it('备份保存失败不继续远端测试，并保留本次输入以便重试', async () => {
  const fetch = vi.fn(async (_url: string, options: RequestInit) => options.method === 'GET' ? response(backupFixture()) : response({ error: '保存配置失败' }, 500))
  vi.stubGlobal('fetch', fetch); mount('/backup')
  await screen.findByLabelText('WebDAV 密码 / 授权码'); change('WebDAV 密码 / 授权码', 'fixture-unsaved-password')
  await userEvent.setup().click(screen.getByRole('button', { name: '保存并测试连接' }))
  await screen.findByText('保存配置失败')
  expect(fetch).toHaveBeenCalledTimes(2)
  expect((screen.getByLabelText('WebDAV 密码 / 授权码') as HTMLInputElement).value).toBe('fixture-unsaved-password')
})

it('备份读取失败只显示重试，不展示默认可写设置', async () => {
  const fetch = vi.fn().mockResolvedValueOnce(response({ error: '读取失败' }, 500)).mockResolvedValueOnce(response(backupFixture()))
  vi.stubGlobal('fetch', fetch); mount('/backup')
  await screen.findByText('读取失败')
  expect(screen.queryByLabelText('备份周期')).toBeNull()
  await userEvent.setup().click(screen.getByRole('button', { name: /重\s*试/ }))
  expect(await screen.findByLabelText('备份周期')).toBeTruthy()
})
