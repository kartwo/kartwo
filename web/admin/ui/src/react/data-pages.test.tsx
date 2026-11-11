// 概览与数据迁移测试 / Dashboard and Data Migration Tests
// 功能：验证聚合契约、下载权限及 CSV 格式绑定、行错误、原任务恢复与防重复确认
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:27:44

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
import { parseDashboard, dashboardMoney } from './dashboard-api'
import { importAPI, parseImport, type ImportPreview } from './import-api'
import type { Session } from './contracts'

const dashboard = { currency: 'USD', orders: { today: { count: 2, sales_cents: 1029 }, week: { count: 4, sales_cents: 2088 }, pending_fulfillment: 1 }, products: { count: 3, zero_stock: 1, low_stock: 2 }, setup: { has_products: true, payment_configured: false, domain_configured: false, ready: false } }
const preview: ImportPreview = { public_id: 'import-fixture-id', format: 'generic', status: 'previewed', total_rows: 1, product_count: 1, variant_count: 1, errors: [] }
function response(value: unknown, status = 200) { return new Response(JSON.stringify(value), { status }) }
function mount(path: keyof typeof businessPages, session: Session = account()) {
  const controller = new SessionController(fakeAPI(session))
  render(<MemoryRouter initialEntries={[path]}><AdminTheme><ConfigProvider theme={{ token: { motion: false } }}><SessionProvider controller={controller}><AdminRoot renderPage={route => {
    const Page = businessPages[route.path as keyof typeof businessPages]
    return <Suspense fallback={<PageLoading />}><Page /></Suspense>
  }} /></SessionProvider></ConfigProvider></AdminTheme></MemoryRouter>)
  return controller
}
async function upload(text = 'fixture csv', name = 'products.csv') {
  const file = new File([text], name, { type: 'text/csv' })
  await userEvent.setup().upload(screen.getByLabelText('CSV 文件'), file)
  return file
}
async function previewFile() {
  await screen.findByLabelText('CSV 文件'); await upload()
  await userEvent.setup().click(screen.getByRole('button', { name: '预览导入' }))
}
it('概览拒绝不安全计数、币种和矛盾就绪状态，金额仍按整数分显示', () => {
  expect(parseDashboard(dashboard).orders.today.sales_cents).toBe(1029)
  expect(dashboardMoney(1029, 'USD')).toMatch(/10\.29/)
  expect(() => parseDashboard({ ...dashboard, setup: { ...dashboard.setup, ready: true } })).toThrow()
  expect(() => parseDashboard({ ...dashboard, products: { ...dashboard.products, count: 1.5 } })).toThrow()
  expect(() => parseDashboard({ ...dashboard, currency: '<script>' })).toThrow()
})
it('概览保留订单与库存指标，商品总数准确说明包含草稿，入口按权限显示', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => response(dashboard)))
  mount('/dashboard', account('staff', ['dashboard.view']))
  await screen.findByText('全部未删除商品（含草稿）')
  expect(screen.getByText(/销售额.*10\.29/)).toBeTruthy()
  expect(screen.getByText('今日订单')).toBeTruthy()
  expect(screen.queryByRole('link', { name: '前往配置收款' })).toBeNull()
  expect(screen.queryByRole('link', { name: /前往发布首个商品/ })).toBeNull()
  expect(screen.getAllByText('请联系有权限的同事完成。')).toHaveLength(2)
})
it('概览加载失败不补零值，重试后可继续开店配置', async () => {
  const fetch = vi.fn().mockResolvedValueOnce(response({ error: '概览暂不可用' }, 500)).mockResolvedValueOnce(response(dashboard))
  vi.stubGlobal('fetch', fetch); mount('/dashboard')
  await screen.findByText('概览暂不可用'); expect(screen.queryByText('今日订单')).toBeNull()
  await userEvent.setup().click(screen.getByRole('button', { name: /重\s*试/ }))
  expect(await screen.findByRole('link', { name: '前往配置收款' })).toBeTruthy()
})
it('导入允许后端空错误 null，拒绝缺失错误、矛盾状态及不安全行数', () => {
  expect(parseImport({ ...preview, errors: null }).errors).toEqual([])
  expect(() => parseImport({ ...preview, errors: undefined })).toThrow()
  expect(() => parseImport({ ...preview, status: 'succeeded', errors: [{ row: 1, message: 'bad' }] })).toThrow()
  expect(() => parseImport({ ...preview, total_rows: -1 })).toThrow()
  expect(() => parseImport({ ...preview, public_id: '../private' })).toThrow()
})
it('导入上传 FormData 绑定来源，执行与核对必须返回原任务', async () => {
  const fetch = vi.fn(async (_url: string, _options: RequestInit) => response({ ...preview, format: 'shopify' })); vi.stubGlobal('fetch', fetch)
  const file = new File(['fixture'], 'fixture.csv')
  await importAPI.preview(file, 'shopify')
  const body = fetch.mock.calls[0]?.[1]?.body as FormData
  expect(body.get('file')).toBe(file); expect(body.get('format')).toBe('shopify')
  expect(fetch).toHaveBeenCalledWith('/admin/api/imports/csv/preview', expect.objectContaining({ method: 'POST', body: expect.any(FormData) }))
  await expect(importAPI.execute(preview)).rejects.toThrow('当前导入任务')
  await expect(importAPI.check(preview)).rejects.toThrow('当前导入任务')
  await expect(importAPI.preview(file, 'generic')).rejects.toThrow('所选来源不一致')
})
it('预览不执行写入，确认成功后不再提供确认按钮', async () => {
  const calls: string[] = []
  const fetch = vi.fn(async (url: string, options: RequestInit) => { calls.push(`${options.method} ${url}`); return response(url.endsWith('/execute') ? { ...preview, status: 'succeeded' } : preview) })
  vi.stubGlobal('fetch', fetch); mount('/imports/csv'); await previewFile()
  const confirm = await screen.findByRole('button', { name: '确认导入' })
  expect(calls).toEqual(['POST /admin/api/imports/csv/preview'])
  await userEvent.setup().click(confirm)
  await screen.findByText('商品已导入')
  expect(screen.queryByRole('button', { name: '确认导入' })).toBeNull()
  expect(calls).toEqual(['POST /admin/api/imports/csv/preview', 'POST /admin/api/imports/import-fixture-id/execute'])
})
it('行错误按文本展示并阻止确认，切换来源使旧预览失效', async () => {
  const fetch = vi.fn(async () => response({ ...preview, status: 'rejected', errors: [{ row: 2, message: '<img src=x> invalid SKU' }] })); vi.stubGlobal('fetch', fetch)
  mount('/imports/csv'); await previewFile()
  await screen.findByText('<img src=x> invalid SKU')
  expect(document.querySelector('img[src="x"]')).toBeNull()
  expect(screen.queryByRole('button', { name: '确认导入' })).toBeNull()
  await userEvent.setup().click(screen.getByRole('combobox', { name: '导入来源' }))
  await userEvent.setup().click(await screen.findByText('Shopify 商品 CSV', { exact: true }))
  expect(screen.queryByText('预览结果')).toBeNull()
  expect(fetch).toHaveBeenCalledTimes(1)
})
it('文件变化清除可执行预览，旧任务不能误执行', async () => {
  const fetch = vi.fn(async () => response(preview)); vi.stubGlobal('fetch', fetch)
  mount('/imports/csv'); await previewFile(); await screen.findByRole('button', { name: '确认导入' })
  await upload('changed fixture', 'new-products.csv')
  expect(screen.queryByRole('button', { name: '确认导入' })).toBeNull()
  expect(fetch).toHaveBeenCalledTimes(1)
})
it('确认处理中锁定文件来源与重复执行，响应丢失可只读核对原任务', async () => {
  const pending = deferred<Response>(), calls: string[] = []
  const fetch = vi.fn(async (url: string, options: RequestInit) => {
    calls.push(`${options.method} ${url}`)
    return url.endsWith('/execute') ? pending.promise : response(options.method === 'GET' ? { ...preview, status: 'succeeded' } : preview)
  }); vi.stubGlobal('fetch', fetch); mount('/imports/csv'); await previewFile()
  const confirm = await screen.findByRole('button', { name: '确认导入' })
  await userEvent.setup().click(confirm); await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2))
  expect((screen.getByLabelText('CSV 文件') as HTMLInputElement).disabled).toBe(true)
  await userEvent.setup().click(confirm); expect(fetch).toHaveBeenCalledTimes(2)
  await act(async () => pending.resolve(response({ error: '响应未确认' }, 502)))
  await screen.findByText('响应未确认')
  await userEvent.setup().click(screen.getByRole('button', { name: '核对任务状态' }))
  await screen.findByText('该任务已成功导入，无需再次确认')
  expect(calls).toEqual(['POST /admin/api/imports/csv/preview', 'POST /admin/api/imports/import-fixture-id/execute', 'GET /admin/api/imports/import-fixture-id'])
  expect(screen.queryByRole('button', { name: '确认导入' })).toBeNull()
})
it('已成功的文件预览不提供再次执行，超大文件在发送前被拒绝', async () => {
  const fetch = vi.fn(async () => response({ ...preview, status: 'succeeded' })); vi.stubGlobal('fetch', fetch)
  mount('/imports/csv'); await previewFile()
  await screen.findByText('该文件已成功导入；重复确认不会再创建商品。')
  expect(screen.queryByRole('button', { name: '确认导入' })).toBeNull()
  const large = new File(['x'], 'large.csv'); Object.defineProperty(large, 'size', { value: 5 * 1024 * 1024 + 1 })
  await expect(importAPI.preview(large, 'generic')).rejects.toThrow('5MB')
  expect(fetch).toHaveBeenCalledTimes(1)
})
it('预览失败后换文件清除旧错误，401 清除导入数据', async () => {
  let expired = false
  const fetch = vi.fn(async () => response({ error: expired ? '会话过期' : '文件解析失败' }, expired ? 401 : 400))
  vi.stubGlobal('fetch', fetch); const controller = mount('/imports/csv'); await previewFile()
  await screen.findByText('文件解析失败')
  await upload('changed', 'new.csv'); expect(screen.queryByText('文件解析失败')).toBeNull()
  expired = true; await userEvent.setup().click(screen.getByRole('button', { name: '预览导入' }))
  await waitFor(() => expect(controller.getSnapshot().phase).toBe('login'))
  expect(screen.queryByLabelText('CSV 文件')).toBeNull()
})
it('授权导入员工无需商品查看权限也能导入，但不显示无权限返回链接', async () => {
  const fetch = vi.fn(async () => response(preview)); vi.stubGlobal('fetch', fetch)
  mount('/imports/csv', account('staff', ['imports.manage'])); await previewFile()
  expect(await screen.findByRole('button', { name: '确认导入' })).toBeTruthy()
  expect(screen.queryByRole('link', { name: '返回商品' })).toBeNull()
})
it('演示导入可查看说明但不能选择文件或请求', async () => {
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch); mount('/imports/csv', account('demo'))
  expect((await screen.findByLabelText('CSV 文件') as HTMLInputElement).disabled).toBe(true)
  expect((screen.getByRole('button', { name: '预览导入' }) as HTMLButtonElement).disabled).toBe(true)
  expect(fetch).not.toHaveBeenCalled()
})
it('导出由原生下载链接接收 ZIP，不在前端缓冲整包', async () => {
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch); mount('/export')
  const link = await screen.findByRole('link', { name: '导出 ZIP' })
  expect(link.getAttribute('href')).toBe('/admin/api/export')
  expect(link.getAttribute('target')).toBe('_blank')
  expect(link.getAttribute('rel')).toBe('noopener noreferrer')
  expect(screen.getByText('SQLite 一致性快照')).toBeTruthy()
  expect(fetch).not.toHaveBeenCalled()
})
it('演示导出无下载链接，仅能查看说明', async () => {
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch); mount('/export', account('demo'))
  expect((await screen.findByRole('button', { name: '导出 ZIP' }) as HTMLButtonElement).disabled).toBe(true)
  expect(screen.queryByRole('link', { name: '导出 ZIP' })).toBeNull()
  expect(fetch).not.toHaveBeenCalled()
})
it.each(['/export', '/imports/csv', '/dashboard'] as const)('%s 无权员工不挂载或请求', async path => {
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch); mount(path, account('staff'))
  await screen.findByText('当前账号没有访问此页面的权限')
  expect(fetch).not.toHaveBeenCalled()
})
it('关于页保留 MIT 说明及官方许可链接，无接口依赖', async () => {
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch); mount('/about', account('staff'))
  expect((await screen.findByRole('link', { name: '查看 MIT 许可证' })).getAttribute('href')).toBe('https://github.com/kartwo/kartwo/blob/main/LICENSE')
  expect(fetch).not.toHaveBeenCalled()
})
