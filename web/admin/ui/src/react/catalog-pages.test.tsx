// 分类内容迁移测试 / Category and Content Migration Tests
// 功能：验证分类管理权限、排序与链接、内容固定链接、正文保真和保存后刷新失败
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:34:25

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
import { businessPages } from './BusinessPages'
import { PageLoading } from './SettingsPages'
import { account, fakeAPI, deferred } from './test-fixtures'
import { catalogAPI, parseCategories, parseContent, type Category, type ContentPage } from './catalog-api'
import type { Session } from './contracts'

const category: Category = { public_id: 'category-fixture-id', name: 'Running Tops', slug: 'running-tops', position: 2, product_count: 0 }
const pageA: ContentPage = { public_id: 'content-fixture-a', title: 'Page A', slug: 'page-a', body_markdown: '# A\n\nBody A', seo_description: 'Fixture SEO', status: 'active', updated_at: '2026-10-25' }
const pageB: ContentPage = { ...pageA, public_id: 'content-fixture-b', title: 'Page B', slug: 'page-b', body_markdown: 'Body B', status: 'draft' }
function response(value: unknown, status = 200) { return new Response(JSON.stringify(value), { status }) }
function mount(path: '/categories' | '/merchandising', session: Session = account()) {
  const controller = new SessionController(fakeAPI(session))
  render(<MemoryRouter initialEntries={[path]}><AdminTheme><ConfigProvider theme={{ token: { motion: false } }}><SessionProvider controller={controller}><AdminRoot renderPage={route => {
    const Page = businessPages[route.path as keyof typeof businessPages]
    return <Suspense fallback={<PageLoading />}><Page /></Suspense>
  }} /></SessionProvider></ConfigProvider></AdminTheme></MemoryRouter>)
  return controller
}
function card(title: string) { return screen.getByText(title, { exact: true, selector: '.ant-card-head-title' }).closest('.ant-card') as HTMLElement }
function change(panel: HTMLElement, label: string, value: string) { fireEvent.change(within(panel).getByLabelText(label), { target: { value } }) }
function submit(panel: HTMLElement, label: string) { fireEvent.submit(within(panel).getByLabelText(label).closest('form')!) }

it('分类和内容契约拒绝重复标识、未知状态与不安全排序', () => {
  expect(() => parseCategories({ categories: [category, category] })).toThrow()
  expect(() => parseCategories({ categories: [{ ...category, position: -1 }] })).toThrow()
  expect(() => parseContent({ ...pageA, status: 'unknown' })).toThrow()
  expect(parseContent({ ...pageA, internal_id: 1 })).not.toHaveProperty('internal_id')
})
it('内容写入只发送允许字段，保留 Markdown 空白，不透传 public_id 等元数据', async () => {
  const fetch = vi.fn(async () => response({ ok: true })); vi.stubGlobal('fetch', fetch)
  await catalogAPI.savePage(pageA.public_id, { ...pageA, body_markdown: '  # Original\n\nTrailing  ' })
  expect(fetch).toHaveBeenCalledWith('/admin/api/content-pages/content-fixture-a', expect.objectContaining({ method: 'PATCH', body: JSON.stringify({ title: 'Page A', slug: 'page-a', body_markdown: '  # Original\n\nTrailing  ', seo_description: 'Fixture SEO', status: 'active' }) }))
  await expect(catalogAPI.deletePage('../private')).rejects.toThrow()
  await expect(catalogAPI.createCategory({ name: 'A', slug: 'a', position: 1.5 })).rejects.toThrow()
  expect(fetch).toHaveBeenCalledTimes(1)
})
it('只有商品查看权限的员工可看分类，不能创建修改删除', async () => {
  const fetch = vi.fn(async () => response({ categories: [category] })); vi.stubGlobal('fetch', fetch)
  mount('/categories', account('staff', ['products.view']))
  await screen.findByText('0 件商品')
  expect((within(card('Running Tops')).getByLabelText('分类名称') as HTMLInputElement).disabled).toBe(true)
  expect(screen.queryByRole('button', { name: '创建分类' })).toBeNull()
  expect(screen.queryByRole('button', { name: '删除分类' })).toBeNull()
  submit(card('Running Tops'), '分类名称'); await act(async () => {})
  expect(fetch).toHaveBeenCalledTimes(1)
})
it('分类自动链接跟随完整名称，手动修改后保留；创建精确提交整数排序', async () => {
  let created = false
  const fetch = vi.fn(async (_url: string, options: RequestInit) => {
    if (options.method === 'POST') { created = true; return response({ public_id: category.public_id }, 201) }
    return response({ categories: created ? [{ ...category, name: 'Running Shorts', slug: 'custom-slug', position: 4 }] : [] })
  }); vi.stubGlobal('fetch', fetch); mount('/categories', account('staff', ['products.view', 'categories.manage']))
  await screen.findByText('尚无分类，请先新建一个')
  const panel = card('新建分类'), user = userEvent.setup()
  await user.type(within(panel).getByLabelText('分类名称'), 'Running Tops')
  expect((within(panel).getByLabelText('链接 slug') as HTMLInputElement).value).toBe('running-tops')
  change(panel, '链接 slug', 'custom-slug'); change(panel, '分类名称', 'Running Shorts'); change(panel, '排序', '4')
  expect((within(panel).getByLabelText('链接 slug') as HTMLInputElement).value).toBe('custom-slug')
  submit(panel, '分类名称')
  await screen.findByText('分类已创建')
  expect(fetch.mock.calls.find(([, options]) => options.method === 'POST')?.[1].body).toBe('{"name":"Running Shorts","slug":"custom-slug","position":4}')
  expect((within(card('新建分类')).getByLabelText('分类名称') as HTMLInputElement).value).toBe('')
})
it('分类排序拒绝小数，修改时保持公开对象范围', async () => {
  const fetch = vi.fn(async (_url: string, options: RequestInit) => response(options.method === 'GET' ? { categories: [category] } : { ok: true }))
  vi.stubGlobal('fetch', fetch); mount('/categories'); await screen.findByText('0 件商品')
  const panel = card('Running Tops')
  change(panel, '排序', '1.5'); submit(panel, '分类名称')
  await screen.findByText('排序须为非负安全整数'); expect(fetch).toHaveBeenCalledTimes(1)
  change(panel, '排序', '3'); change(panel, '分类名称', 'New Name'); submit(panel, '分类名称')
  await screen.findByText('分类已保存')
  expect(fetch).toHaveBeenCalledWith('/admin/api/categories/category-fixture-id', expect.objectContaining({ method: 'PATCH', body: '{"name":"New Name","slug":"running-tops","position":3}' }))
})
it('有关联商品的分类禁止删除，空分类删除须确认且可取消', async () => {
  let deleted = false
  const used = { ...category, public_id: 'category-used-fixture', name: 'Used Category', product_count: 2 }
  const fetch = vi.fn(async (_url: string, options: RequestInit) => {
    if (options.method === 'DELETE') { deleted = true; return response({ ok: true }) }
    return response({ categories: deleted ? [used] : [category, used] })
  }); vi.stubGlobal('fetch', fetch); mount('/categories'); await screen.findByText('2 件商品')
  expect((within(card('Used Category')).getByRole('button', { name: '删除分类' }) as HTMLButtonElement).disabled).toBe(true)
  await userEvent.setup().click(within(card('Running Tops')).getByRole('button', { name: '删除分类' }))
  let dialog = await screen.findByRole('dialog', { name: '删除分类' })
  await userEvent.setup().click(within(dialog).getByRole('button', { name: /取\s*消/ }))
  expect(fetch).toHaveBeenCalledTimes(1)
  await userEvent.setup().click(within(card('Running Tops')).getByRole('button', { name: '删除分类' }))
  dialog = await screen.findByRole('dialog', { name: '删除分类' })
  await userEvent.setup().click(within(dialog).getByRole('button', { name: /删\s*除/ }))
  await screen.findByText('分类已删除')
  expect(fetch).toHaveBeenCalledWith('/admin/api/categories/category-fixture-id', expect.objectContaining({ method: 'DELETE' }))
})
it('分类创建成功但刷新失败仍追加已创建项并清空创建表单', async () => {
  let created = false
  const fetch = vi.fn(async (_url: string, options: RequestInit) => {
    if (options.method === 'POST') { created = true; return response({ public_id: category.public_id }, 201) }
    return created ? response({ error: '列表暂不可用' }, 500) : response({ categories: [] })
  }); vi.stubGlobal('fetch', fetch); mount('/categories'); await screen.findByText('尚无分类，请先新建一个')
  const panel = card('新建分类'); change(panel, '分类名称', 'Running Tops'); submit(panel, '分类名称')
  await screen.findByText(/保存操作已完成，列表刷新失败/)
  expect(screen.getByRole('button', { name: '保存分类' })).toBeTruthy()
  expect((within(card('新建分类')).getByLabelText('分类名称') as HTMLInputElement).value).toBe('')
})
it('内容详情快速切换时只保留当前对象，正文 HTML 只是文本值', async () => {
  const late = deferred<Response>()
  const fetch = vi.fn(async (url: string) => url.endsWith('/content-pages') ? response({ pages: [pageA, pageB] }) : url.endsWith('/content-fixture-a') ? late.promise : response({ ...pageB, body_markdown: '<img src=x>\nBody B' }))
  vi.stubGlobal('fetch', fetch); mount('/merchandising')
  await screen.findByRole('heading', { name: '内容', level: 2 })
  await userEvent.setup().click(await screen.findByRole('button', { name: 'Page A' }))
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2))
  await userEvent.setup().click(screen.getByRole('button', { name: 'Page B' }))
  await waitFor(() => expect((screen.getByLabelText('标题') as HTMLInputElement).value).toBe('Page B'))
  await act(async () => late.resolve(response(pageA)))
  expect((screen.getByLabelText('标题') as HTMLInputElement).value).toBe('Page B')
  expect((screen.getByLabelText('内容（安全 Markdown）') as HTMLTextAreaElement).value).toBe('<img src=x>\nBody B')
  expect(document.querySelector('img[src="x"]')).toBeNull()
})
it('内容新建后链接固定且刷新失败也不会重复创建；再次保存使用原 ID', async () => {
  let created = false
  const fetch = vi.fn(async (_url: string, options: RequestInit) => {
    if (options.method === 'POST') { created = true; return response({ public_id: pageB.public_id }, 201) }
    if (options.method === 'PATCH') return response({ ok: true })
    return created ? response({ error: '列表读取失败' }, 500) : response({ pages: [] })
  }); vi.stubGlobal('fetch', fetch); mount('/merchandising'); await screen.findByText('尚无内容页')
  const panel = card('新建内容页')
  change(panel, '标题', 'New Page'); change(panel, '链接 slug', 'new-page'); change(panel, '内容（安全 Markdown）', '  ## Kept\n\nText  ')
  submit(panel, '标题')
  await screen.findByText(/保存操作已完成，列表刷新失败/)
  expect((screen.getByLabelText('链接 slug') as HTMLInputElement).disabled).toBe(true)
  expect((screen.getByLabelText('内容（安全 Markdown）') as HTMLTextAreaElement).value).toBe('  ## Kept\n\nText  ')
  change(card('编辑内容页'), '标题', 'Edited Page'); submit(card('编辑内容页'), '标题')
  await waitFor(() => expect(fetch.mock.calls.some(([, options]) => options.method === 'PATCH')).toBe(true))
  expect(fetch.mock.calls.filter(([, options]) => options.method === 'POST')).toHaveLength(1)
  expect(fetch).toHaveBeenCalledWith('/admin/api/content-pages/content-fixture-b', expect.objectContaining({ method: 'PATCH', body: '{"title":"Edited Page","slug":"new-page","body_markdown":"  ## Kept\\n\\nText  ","seo_description":"","status":"draft"}' }))
})
it('内容编辑保留已发布状态且只发允许字段，演示可读但不能保存', async () => {
  const fetch = vi.fn(async (url: string, options: RequestInit) => response(options.method === 'PATCH' ? { ok: true } : url.endsWith('/content-pages') ? { pages: [pageA] } : pageA))
  vi.stubGlobal('fetch', fetch); mount('/merchandising', account('staff', ['content.manage']))
  await userEvent.setup().click(await screen.findByRole('button', { name: 'Page A' }))
  await waitFor(() => expect((screen.getByLabelText('标题') as HTMLInputElement).value).toBe('Page A'))
  change(card('编辑内容页'), '标题', 'Updated A'); submit(card('编辑内容页'), '标题')
  await screen.findByText('内容页已保存')
  const write = fetch.mock.calls.find(([, options]) => options.method === 'PATCH')
  expect(JSON.parse(write?.[1].body as string)).toEqual({ title: 'Updated A', slug: 'page-a', body_markdown: '# A\n\nBody A', seo_description: 'Fixture SEO', status: 'active' })
})
it('内容删除当前页面后回到空白新建，取消删除不发写入', async () => {
  let deleted = false
  const fetch = vi.fn(async (url: string, options: RequestInit) => {
    if (options.method === 'DELETE') { deleted = true; return response({ ok: true }) }
    return response(url.endsWith('/content-pages') ? { pages: deleted ? [] : [pageA] } : pageA)
  }); vi.stubGlobal('fetch', fetch); mount('/merchandising')
  await userEvent.setup().click(await screen.findByRole('button', { name: 'Page A' }))
  await waitFor(() => expect((screen.getByLabelText('标题') as HTMLInputElement).value).toBe('Page A'))
  await userEvent.setup().click(screen.getByRole('button', { name: '删除内容页：Page A' }))
  let dialog = await screen.findByRole('dialog', { name: '删除内容页' })
  await userEvent.setup().click(within(dialog).getByRole('button', { name: /取\s*消/ }))
  expect(fetch).toHaveBeenCalledTimes(2)
  await userEvent.setup().click(screen.getByRole('button', { name: '删除内容页：Page A' }))
  dialog = await screen.findByRole('dialog', { name: '删除内容页' })
  await userEvent.setup().click(within(dialog).getByRole('button', { name: /删\s*除/ }))
  await screen.findByText('内容页已删除')
  expect((screen.getByLabelText('标题') as HTMLInputElement).value).toBe('')
  expect((screen.getByLabelText('链接 slug') as HTMLInputElement).disabled).toBe(false)
})
it('演示内容可查看原文，不能创建保存或删除', async () => {
  const fetch = vi.fn(async (url: string) => response(url.endsWith('/content-pages') ? { pages: [pageA] } : pageA)); vi.stubGlobal('fetch', fetch)
  mount('/merchandising', account('demo'))
  await userEvent.setup().click(await screen.findByRole('button', { name: 'Page A' }))
  await waitFor(() => expect((screen.getByLabelText('标题') as HTMLInputElement).value).toBe('Page A'))
  expect((screen.getByLabelText('内容（安全 Markdown）') as HTMLTextAreaElement).disabled).toBe(true)
  expect(screen.queryByRole('button', { name: '保存内容页' })).toBeNull()
  expect(screen.queryByRole('button', { name: '删除内容页：Page A' })).toBeNull()
  submit(card('编辑内容页'), '标题'); await act(async () => {})
  expect(fetch).toHaveBeenCalledTimes(2)
})
it('内容保存 401 清除页面，会话未授权错误不继续读取列表', async () => {
  const fetch = vi.fn(async (_url: string, options: RequestInit) => options.method === 'GET' ? response({ pages: [] }) : response({ error: '会话已过期' }, 401))
  vi.stubGlobal('fetch', fetch); const controller = mount('/merchandising'); await screen.findByText('尚无内容页')
  const panel = card('新建内容页'); change(panel, '标题', 'New Page'); change(panel, '链接 slug', 'new-page'); submit(panel, '标题')
  await waitFor(() => expect(controller.getSnapshot().phase).toBe('login'))
  expect(screen.queryByLabelText('内容（安全 Markdown）')).toBeNull()
  expect(fetch).toHaveBeenCalledTimes(2)
})
it.each(['/categories', '/merchandising'] as const)('%s 读取失败无默认可写内容，重试后恢复', async path => {
  const fetch = vi.fn().mockResolvedValueOnce(response({ error: '读取失败' }, 500)).mockResolvedValueOnce(response(path === '/categories' ? { categories: [] } : { pages: [] }))
  vi.stubGlobal('fetch', fetch); mount(path)
  await screen.findByText('读取失败')
  expect(screen.queryByLabelText('分类名称')).toBeNull(); expect(screen.queryByLabelText('标题')).toBeNull()
  await userEvent.setup().click(screen.getByRole('button', { name: /重\s*试/ }))
  expect(await screen.findByLabelText(path === '/categories' ? '分类名称' : '标题')).toBeTruthy()
})
