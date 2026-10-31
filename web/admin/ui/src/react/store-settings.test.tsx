// 店面配置交互验证 / Storefront Settings Interaction Tests
// 功能：核对品牌上传、开关与隐私留存请求，覆盖读取失败、权限和过期会话
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:34:28

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
import { parseShop, parseBlog, parseContact, type ShopSettings } from './store-settings-api'
import type { Session } from './contracts'

function mount(path: keyof typeof settingsPages, session: Session = account()) {
  const controller = new SessionController(fakeAPI(session))
  render(<MemoryRouter initialEntries={[path]}><AdminTheme><SessionProvider controller={controller}><AdminRoot renderPage={route => {
    const Page = settingsPages[route.path as keyof typeof settingsPages]
    return <Suspense fallback={<PageLoading />}><Page /></Suspense>
  }} /></SessionProvider></AdminTheme></MemoryRouter>)
  return controller
}
const shop: ShopSettings = { name: 'Fixture Store', logo_url: '', source: 'db', readonly: false }
const contact = { enabled: false, retention_months: 12, ip_retention_days: 90 }
function response(data: unknown, status = 200) { return new Response(JSON.stringify(data), { status }) }
function submit(label: string) { fireEvent.submit(screen.getByLabelText(label).closest('form')!) }
function change(label: string, value: string) { fireEvent.change(screen.getByLabelText(label), { target: { value } }) }

it('严格配置响应拒绝未知值、越界留存和外站 Logo，不将空响应当作默认开启', () => {
  expect(parseShop({ ...shop, source: 'demo', readonly: false }).readonly).toBe(true)
  for (const logo_url of ['https://example.com/logo.webp', '//example.com/a.webp', '/media/brand/../private.webp', 'javascript:alert(1)', '/media/brand/a.svg']) {
    expect(() => parseShop({ ...shop, logo_url })).toThrow()
  }
  expect(parseShop({ ...shop, logo_url: '/media/brand/hash.webp' }).logo_url).toBe('/media/brand/hash.webp')
  for (const value of [null, {}, { enabled: 'false' }]) expect(() => parseBlog(value)).toThrow()
  for (const value of [0, 25, 1.5, '12', null]) expect(() => parseContact({ ...contact, retention_months: value })).toThrow()
  for (const value of [6, 91, 7.1]) expect(() => parseContact({ ...contact, ip_retention_days: value })).toThrow()
})

it('授权员工可上传与移除 Logo，上传保留未保存名称，保存仅提交名称', async () => {
  let data = { ...shop }
  const writes: { url: string; options: RequestInit }[] = []
  vi.stubGlobal('fetch', vi.fn(async (url: string, options: RequestInit) => {
    if (options.method !== 'GET') writes.push({ url, options })
    if (options.method === 'POST') data = { ...data, logo_url: '/media/brand/fixture.webp' }
    if (options.method === 'DELETE') data = { ...data, logo_url: '' }
    if (options.method === 'PUT') data = { ...data, name: JSON.parse(options.body as string).name }
    return response(data)
  }))
  mount('/shop', account('staff', ['settings.shop']))
  await screen.findByRole('heading', { name: '店铺', level: 2 })
  await screen.findByLabelText('店铺名称')
  change('店铺名称', ' New Store ')
  const file = new File(['fixture-image-bytes'], 'logo.png', { type: 'image/png' })
  await userEvent.setup().upload(screen.getByLabelText('上传 Logo'), file)
  expect(await screen.findByAltText('Fixture Store Logo')).toBeTruthy()
  expect((screen.getByLabelText('店铺名称') as HTMLInputElement).value).toBe(' New Store ')
  expect(writes[0]?.url).toBe('/admin/api/settings/shop/logo')
  expect(writes[0]?.options.body).toBeInstanceOf(FormData)
  expect((writes[0]?.options.body as FormData).get('file')).toBe(file)
  expect(writes[0]?.options.headers).not.toHaveProperty('Content-Type')
  await userEvent.setup().click(screen.getByRole('button', { name: '删除 Logo' }))
  await screen.findByText('Logo 已删除，店面页眉恢复显示店铺名称。')
  expect(screen.queryByRole('img', { name: 'Fixture Store Logo' })).toBeNull()
  expect((screen.getByLabelText('店铺名称') as HTMLInputElement).value).toBe(' New Store ')
  expect(writes[1]?.options.method).toBe('DELETE')
  submit('店铺名称')
  await screen.findByText('店铺名称已保存，店面会立即使用新名称。')
  expect(JSON.parse(writes[2]?.options.body as string)).toEqual({ name: 'New Store' })
  expect((screen.getByLabelText('店铺名称') as HTMLInputElement).value).toBe('New Store')
})

it('Logo 请求返回较新的已保存名称时，上传和移除仍保留当前名称草稿', async () => {
  const upload = deferred<Response>(), removal = deferred<Response>()
  const writes: string[] = []
  vi.stubGlobal('fetch', vi.fn(async (_url: string, options: RequestInit) => {
    if (options.method === 'GET') return response(shop)
    writes.push(options.method!)
    if (options.method === 'POST') return upload.promise
    return removal.promise
  }))
  mount('/shop', account('staff', ['settings.shop']))
  await screen.findByDisplayValue('Fixture Store')
  const user = userEvent.setup()
  const input = screen.getByLabelText('店铺名称') as HTMLInputElement
  await user.clear(input); await user.type(input, 'Unsaved merchant name')
  await user.upload(screen.getByLabelText('上传 Logo'), new File(['x'], 'logo.png', { type: 'image/png' }))
  await act(async () => upload.resolve(response({ ...shop, name: 'Updated saved name', logo_url: '/media/brand/new.webp' })))
  expect(await screen.findByAltText('Updated saved name Logo')).toBeTruthy()
  expect(input.value).toBe('Unsaved merchant name')
  await user.click(screen.getByRole('button', { name: '删除 Logo' }))
  await act(async () => removal.resolve(response({ ...shop, name: 'Latest saved name' })))
  await screen.findByText('Logo 已删除，店面页眉恢复显示店铺名称。')
  expect(input.value).toBe('Unsaved merchant name')
  expect(writes).toEqual(['POST', 'DELETE'])
})

it('上传未结束时阻止重复上传和名称保存；错误后保留原 Logo 和输入并可重试', async () => {
  const upload = deferred<Response>()
  const fetch = vi.fn(async (_url: string, options: RequestInit) => options.method === 'GET' ? response({ ...shop, logo_url: '/media/brand/old.webp' }) : upload.promise)
  vi.stubGlobal('fetch', fetch)
  mount('/shop')
  await screen.findByLabelText('更换 Logo')
  change('店铺名称', 'Still editing')
  const input = screen.getByLabelText('更换 Logo') as HTMLInputElement
  const file = new File(['x'], 'logo.png', { type: 'image/png' })
  fireEvent.change(input, { target: { files: [file] } })
  await waitFor(() => expect(input.disabled).toBe(true))
  fireEvent.change(input, { target: { files: [file] } }); submit('店铺名称')
  await act(async () => {})
  expect(fetch).toHaveBeenCalledTimes(2)
  await act(async () => upload.resolve(response({ error: '图片超过上传限制' }, 413)))
  expect(await screen.findByText('图片超过上传限制')).toBeTruthy()
  expect(screen.getByAltText('Fixture Store Logo').getAttribute('src')).toBe('/media/brand/old.webp')
  expect((screen.getByLabelText('店铺名称') as HTMLInputElement).value).toBe('Still editing')
  expect(input.disabled).toBe(false)
})

it('演示店铺即使收到可写状态也不可保存或上传', async () => {
  const fetch = vi.fn(async () => response(shop)); vi.stubGlobal('fetch', fetch)
  mount('/shop', account('demo'))
  expect((await screen.findByLabelText('店铺名称') as HTMLInputElement).disabled).toBe(true)
  expect((screen.getByLabelText('上传 Logo') as HTMLInputElement).disabled).toBe(true)
  submit('店铺名称')
  fireEvent.change(screen.getByLabelText('上传 Logo'), { target: { files: [new File(['x'], 'a.png')] } })
  await act(async () => {})
  expect(fetch).toHaveBeenCalledTimes(1)
})

it('读取博客失败时无默认开关；重试后可关闭，保存失败保留修改以便重试', async () => {
  const fetch = vi.fn()
    .mockResolvedValueOnce(response({ error: '读取失败' }, 500))
    .mockResolvedValueOnce(response({ enabled: true }))
    .mockResolvedValueOnce(response({ error: '保存失败' }, 500))
    .mockResolvedValueOnce(response({ enabled: false }))
  vi.stubGlobal('fetch', fetch)
  mount('/blog-settings')
  await screen.findByText('读取失败')
  expect(screen.queryByRole('switch')).toBeNull()
  await userEvent.setup().click(screen.getByRole('button', { name: /重\s*试/ }))
  const toggle = await screen.findByRole('switch', { name: '开启店面博客' })
  expect(toggle.getAttribute('aria-checked')).toBe('true')
  await userEvent.setup().click(toggle)
  submit('开启店面博客')
  await screen.findByText('保存失败')
  expect(toggle.getAttribute('aria-checked')).toBe('false')
  submit('开启店面博客')
  await screen.findByText('博客设置已保存')
  expect(fetch).toHaveBeenLastCalledWith('/admin/api/settings/blog', expect.objectContaining({ method: 'PUT', body: '{"enabled":false}' }))
})

it('客服留存只能提交整数；开启和关闭保留明确布尔值及完整周期', async () => {
  const fetch = vi.fn(async (_url: string, options: RequestInit) => response(options.method === 'GET' ? contact : JSON.parse(options.body as string)))
  vi.stubGlobal('fetch', fetch)
  mount('/contact-settings')
  await screen.findByLabelText('结案后保留完整留言（月）')
  change('结案后保留完整留言（月）', '1.5')
  submit('开启在线留言')
  await screen.findByText('请输入 1–24 的整数')
  expect(fetch).toHaveBeenCalledTimes(1)
  change('结案后保留完整留言（月）', '6')
  change('IP 地址保留（天）', '30')
  await userEvent.setup().click(screen.getByRole('switch', { name: '开启在线留言' }))
  submit('开启在线留言')
  await screen.findByText('客服留言设置已保存')
  expect(fetch).toHaveBeenLastCalledWith('/admin/api/settings/contact', expect.objectContaining({ method: 'PUT', body: JSON.stringify({ enabled: true, retention_months: 6, ip_retention_days: 30 }) }))
})

it.each([
  ['/blog-settings', '开启店面博客', 'blog.view', { enabled: false }],
  ['/contact-settings', '开启在线留言', 'contact.view', contact],
] as const)('%s 授权员工可读但不能提交', async (path, label, permission, data) => {
  const fetch = vi.fn(async () => response(data)); vi.stubGlobal('fetch', fetch)
  mount(path, account('staff', [permission]))
  const toggle = await screen.findByRole('switch', { name: label }) as HTMLButtonElement
  expect(toggle.disabled).toBe(true)
  expect(screen.queryByRole('button', { name: '保存设置' })).toBeNull()
  submit(label); await act(async () => {})
  expect(fetch).toHaveBeenCalledTimes(1)
})

it('客服保存 401 时清理会话及当前表单', async () => {
  vi.stubGlobal('fetch', vi.fn(async (_url: string, options: RequestInit) => options.method === 'GET' ? response(contact) : response({ error: '登录已过期' }, 401)))
  const controller = mount('/contact-settings')
  await screen.findByLabelText('开启在线留言')
  submit('开启在线留言')
  await waitFor(() => expect(controller.getSnapshot().phase).toBe('login'))
  expect(screen.queryByLabelText('开启在线留言')).toBeNull()
})
