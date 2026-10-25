// 订单工作台界面测试 / Order Workbench UI Tests
// 功能：验证私密请求体、分页契约、服务端筛选、旧响应取消、失败重试及会话清除
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 13:42:16

import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfigProvider } from 'antd'
import { MemoryRouter } from 'react-router-dom'
import { expect, it, vi } from 'vitest'
import { AdminTheme } from './theme'
import { SessionProvider } from './SessionProvider'
import { SessionController } from './session'
import { PageRouter } from './PageRouter'
import { account, deferred, fakeAPI } from './test-fixtures'
import { emptyOrderFilters, initialOrderSearch, orderSearchPayload, parseWorkbench, searchOrders, type OrderSearch } from './order-workbench-api'
import { workbenchFixture, workbenchOrder } from './order-workbench-fixtures'
import type { Session } from './contracts'
function response(data: unknown, status = 200) { return new Response(JSON.stringify(data), { status }) }
function fixtureFetch(handle?: (query: OrderSearch, options: RequestInit) => Response | Promise<Response>) {
  const fetch = vi.fn(async (url: string, options: RequestInit) => {
    if (url !== '/admin/api/orders/search' || options.method !== 'POST') throw new Error('unexpected fixture request')
    const query = JSON.parse(options.body as string) as OrderSearch
    return handle ? handle(query, options) : response(workbenchFixture(query))
  }); vi.stubGlobal('fetch', fetch); return fetch
}
function mount(session: Session = account()) {
  const controller = new SessionController(fakeAPI(session))
  render(<MemoryRouter initialEntries={['/orders']}><AdminTheme><ConfigProvider theme={{ token: { motion: false } }}><SessionProvider controller={controller}><PageRouter /></SessionProvider></ConfigProvider></AdminTheme></MemoryRouter>)
  return controller
}
function text(name: string, value: string) { fireEvent.change(screen.getByRole('textbox', { name }), { target: { value } }) }

it('搜索使用同源 CSRF 请求体和白名单，不把客户邮箱放入 URI 或浏览器存储', async () => {
  const fetch = fixtureFetch(), store = vi.spyOn(Storage.prototype, 'setItem')
  document.cookie = 'kartwo_csrf=fixture-search-csrf; path=/'
  try {
    const input = { ...initialOrderSearch(), term: ' fixture@example.invalid ', customer_data: true, extra: 'not-allowed' }
    await searchOrders(input, true)
    expect(fetch.mock.calls[0]![0]).toBe('/admin/api/orders/search')
    expect(JSON.parse(fetch.mock.calls[0]![1].body as string)).toEqual({ ...initialOrderSearch(), term: 'fixture@example.invalid' })
    expect(fetch.mock.calls[0]![1].headers).toMatchObject({ 'X-CSRF-Token': 'fixture-search-csrf' }); expect(store).not.toHaveBeenCalled()
  } finally { document.cookie = 'kartwo_csrf=; max-age=0; path=/'; store.mockRestore() }
})
it('工作台先丢弃未授权邮箱再解析，授权用户则拒绝不合法私密字段', () => {
  const data = workbenchFixture(); (data.orders[0] as unknown as { email: unknown }).email = { private: true }
  expect(parseWorkbench(data, initialOrderSearch(), false).orders[0]!.email).toBe('已隐藏')
  expect(() => parseWorkbench(data, initialOrderSearch(), true)).toThrow()
})
it('拒绝数量、分页、重复对象、视图或发货条件不一致，不显示伪造清单', () => {
  const base = workbenchFixture(), input = initialOrderSearch()
  for (const data of [
    { ...base, counts: { ...base.counts, ready_to_fulfill: 2 } }, { ...base, total: 2 }, { ...base, page: 0 }, { ...base, page_size: 101 },
    { ...base, counts: { ...base.counts, ready_to_fulfill: 0 } },
    { ...base, view: 'shipped' }, { ...base, orders: [] }, { ...base, orders: [workbenchOrder({ refund_pending: true })] },
    { ...base, orders: [workbenchOrder(), workbenchOrder()], total: 2, counts: { ...base.counts, all: 2 } },
  ]) expect(() => parseWorkbench(data, input, true)).toThrow()
  const review = { ...input, view: 'needs_review' as const }
  expect(() => parseWorkbench(workbenchFixture(review, [workbenchOrder()], { ...base.counts, needs_review: 1 }), review, true)).toThrow()
})
it('允许服务器将越界页码归到末页，拒绝越过数量边界或未满的页面', () => {
  const input = { ...initialOrderSearch(), page: 999 }
  const data = { ...workbenchFixture(), page: 2, total: 21, counts: { ...workbenchFixture().counts, all: 21 } }
  expect(parseWorkbench(data, input, true).page).toBe(2)
  expect(() => parseWorkbench({ ...data, page: 1 }, input, true)).toThrow()
  expect(() => parseWorkbench({ ...data, orders: [] }, input, true)).toThrow()
})
it('无效日期、页码、体积、控制字符和未知过滤值在出站前拒绝', () => {
  const base = initialOrderSearch()
  for (const invalid of [{ term: 'a'.repeat(161) }, { term: 'a\u0000b' }, { page: -1 }, { page: 1.5 }, { page_size: 101 }, { from: '2026-02-30' }, { from: '2026-10-02', to: '2026-10-01' }, { view: 'invalid' }, { provider: 'invalid' }]) expect(() => orderSearchPayload({ ...base, ...invalid } as OrderSearch)).toThrow()
})
it('输入只修改草稿，提交后服务端查询；条件不写 URL，导出仍独立设置', async () => {
  const fetch = fixtureFetch(); mount(); await screen.findByRole('link', { name: 'order-fixture-a' })
  text('搜索订单', 'fixture@example.invalid'); expect(fetch).toHaveBeenCalledTimes(1); expect(screen.getByText(/筛选条件已修改/)).toBeTruthy()
  fireEvent.change(screen.getByLabelText('订单开始日期'), { target: { value: '2026-10-01' } }); fireEvent.change(screen.getByLabelText('订单结束日期'), { target: { value: '2026-10-25' } })
  await userEvent.click(screen.getByRole('button', { name: '查询订单' })); await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2))
  expect(JSON.parse(fetch.mock.calls[1]![1].body as string)).toMatchObject({ term: 'fixture@example.invalid', from: '2026-10-01', to: '2026-10-25', page: 1 })
  expect(screen.getByRole('link', { name: '导出订单 CSV' }).getAttribute('href')).toBe('/admin/api/orders/export'); expect(window.location.href).not.toContain('fixture@example.invalid')
})
it('工作视图触发新查询并回到第一页，清空恢复默认条件', async () => {
  const fetch = fixtureFetch(query => response(workbenchFixture(query, [workbenchOrder({ needs_review: query.view === 'needs_review', ready_to_fulfill: query.view !== 'needs_review' })], { all: 1, awaiting_payment: 0, ready_to_fulfill: query.view === 'needs_review' ? 0 : 1, needs_review: 1, refunds_pending: 0, shipped: 0 })))
  mount(); await screen.findByRole('link', { name: 'order-fixture-a' }); text('搜索订单', 'order-fixture')
  await userEvent.click(screen.getByRole('button', { name: '查询订单' })); await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2))
  await userEvent.click(screen.getByRole('button', { name: '查看资金待核对' })); await waitFor(() => expect(fetch).toHaveBeenCalledTimes(3)); await screen.findByText('资金待核对 · 1 条')
  expect(JSON.parse(fetch.mock.calls[2]![1].body as string)).toMatchObject({ term: 'order-fixture', view: 'needs_review', page: 1 })
  await userEvent.click(screen.getByRole('button', { name: '清空筛选' })); await waitFor(() => expect(fetch).toHaveBeenCalledTimes(4)); expect(JSON.parse(fetch.mock.calls[3]![1].body as string)).toEqual(initialOrderSearch())
  expect((screen.getByRole('textbox', { name: '搜索订单' }) as HTMLInputElement).value).toBe('')
})
it('更换状态与支付方式提交后才筛选，错误日期不出站且保留输入', async () => {
  const fetch = fixtureFetch(); mount(); await screen.findByRole('link', { name: 'order-fixture-a' })
  await userEvent.click(screen.getByRole('combobox', { name: '订单状态筛选' })); await userEvent.click(screen.getByText('已付款', { selector: '.ant-select-item-option-content' }))
  await userEvent.click(screen.getByRole('combobox', { name: '支付方式筛选' })); await userEvent.click(screen.getByText('Stripe', { selector: '.ant-select-item-option-content' })); expect(fetch).toHaveBeenCalledTimes(1)
  await userEvent.click(screen.getByRole('button', { name: '查询订单' })); await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2)); expect(JSON.parse(fetch.mock.calls[1]![1].body as string)).toMatchObject({ status: 'paid', provider: 'stripe' })
  fireEvent.change(screen.getByLabelText('订单开始日期'), { target: { value: '2026-10-25' } }); fireEvent.change(screen.getByLabelText('订单结束日期'), { target: { value: '2026-10-24' } })
  await userEvent.click(screen.getByRole('button', { name: '查询订单' })); await screen.findByText('结束日期不能早于开始日期'); expect(fetch).toHaveBeenCalledTimes(2)
})
it('分页发送页码而非再次全量读取，切换每页条数会回到第一页', async () => {
  const fetch = fixtureFetch(query => {
    const offset = (query.page - 1) * query.page_size, size = Math.min(query.page_size, 41 - offset)
    const rows = Array.from({ length: size }, (_, index) => workbenchOrder({ public_id: `paged-order-${offset + index + 1}` }))
    return response(workbenchFixture(query, rows, { all: 41, awaiting_payment: 0, ready_to_fulfill: 41, needs_review: 0, refunds_pending: 0, shipped: 0 }))
  }); mount(); await screen.findByRole('link', { name: 'paged-order-1' })
  await userEvent.click(screen.getByTitle('2')); await screen.findByRole('link', { name: 'paged-order-21' }); expect(JSON.parse(fetch.mock.calls[1]![1].body as string).page).toBe(2)
  expect(screen.queryByRole('link', { name: 'paged-order-1' })).toBeNull()
  await userEvent.click(screen.getByRole('combobox', { name: '页码' })); await userEvent.click(screen.getByText('50 条/页', { selector: '.ant-select-item-option-content' }))
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(3)); expect(JSON.parse(fetch.mock.calls[2]![1].body as string)).toMatchObject({ page: 1, page_size: 50 })
})
it('新视图取消旧查询，迟到的旧页面不能覆盖新结果', async () => {
  const pending = deferred<Response>()
  const fetch = fixtureFetch(query => query.view === 'all' ? pending.promise : response(workbenchFixture(query, [workbenchOrder({ public_id: 'review-new-order', needs_review: true, ready_to_fulfill: false })], { all: 1, awaiting_payment: 0, ready_to_fulfill: 0, needs_review: 1, refunds_pending: 0, shipped: 0 })))
  mount(); await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1)); await userEvent.click(screen.getByRole('button', { name: '查看资金待核对' })); await screen.findByRole('link', { name: 'review-new-order' })
  expect(fetch.mock.calls[0]![1].signal?.aborted).toBe(true)
  await act(async () => pending.resolve(response(workbenchFixture()))); expect(screen.queryByRole('link', { name: 'order-fixture-a' })).toBeNull(); expect(screen.getByRole('link', { name: 'review-new-order' })).toBeTruthy()
})
it('读取失败不显示空清单或旧数字，重试后恢复', async () => {
  let failed = true
  fixtureFetch(query => failed ? response({ error: 'Fixture search unavailable' }, 500) : response(workbenchFixture(query)))
  mount(); await screen.findByText('Fixture search unavailable'); expect(screen.queryByText('没有符合条件的订单')).toBeNull(); expect(screen.queryByRole('link', { name: 'order-fixture-a' })).toBeNull()
  failed = false; await userEvent.click(screen.getByRole('button', { name: '重试' })); await screen.findByRole('link', { name: 'order-fixture-a' })
})
it('员工只获得订单号搜索与脱敏结果，没有导出权限；401 清除查询及私密结果', async () => {
  const fetch = fixtureFetch(); const controller = mount(account('staff', ['orders.view']))
  await screen.findByRole('link', { name: 'order-fixture-a' }); expect(screen.getByPlaceholderText('订单号')).toBeTruthy(); expect(document.body.textContent).not.toContain('fixture@example.invalid'); expect(screen.queryByRole('link', { name: '导出订单 CSV' })).toBeNull()
  fetch.mockResolvedValue(response({ error: 'Fixture expired' }, 401)); text('搜索订单', 'private-query'); await userEvent.click(screen.getByRole('button', { name: '查询订单' })); await screen.findByRole('button', { name: '登录后台' })
  expect(screen.queryByRole('textbox', { name: '搜索订单' })).toBeNull(); expect(screen.queryByRole('link', { name: 'order-fixture-a' })).toBeNull(); expect(controller.getSnapshot().session).toBeNull()
  expect(emptyOrderFilters().term).toBe('')
})
