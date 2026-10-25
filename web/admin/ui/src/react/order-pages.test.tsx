// 订单页面迁移测试 / Order Pages Migration Tests
// 功能：验证资料权限、原生导出、资金履约分离、退款确认/原键恢复及刷新失败屏障
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:46:12

import { Suspense } from 'react'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfigProvider } from 'antd'
import { MemoryRouter, useLocation, useNavigate } from 'react-router-dom'
import { expect, it, vi } from 'vitest'
import { AdminRoot } from './AdminRoot'
import { AdminTheme } from './theme'
import { SessionProvider } from './SessionProvider'
import { SessionController } from './session'
import { businessPages } from './BusinessPages'
import { PageLoading } from './SettingsPages'
import { account, fakeAPI, deferred } from './test-fixtures'
import { orderA, orderB, refundRequest, refundResult, orderSummary } from './order-fixtures'
import type { Order } from './order-api'
import type { Session } from './contracts'
import { workbenchFixture } from './order-workbench-fixtures'
import { legacyShipmentFixture } from './shipment-fixtures'
function response(value: unknown, status = 200) { return new Response(JSON.stringify(value), { status }) }
function fixtureFetch(handle?: (url: string, options: RequestInit) => Response | undefined | Promise<Response | undefined>) {
  const fetch = vi.fn(async (url: string, options: RequestInit) => {
    const handled = await handle?.(url, options); if (handled) return handled
    if (url.endsWith('/orders/search') && options.method === 'POST') return response(workbenchFixture())
    if (options.method === 'GET') {
      if (url.endsWith('/shipment')) return response(legacyShipmentFixture(url.includes('order-fixture-b') ? orderB.public_id : orderA.public_id))
      if (url.endsWith('/orders')) return response({ orders: [orderSummary(orderA)] })
      if (url.endsWith('/order-fixture-a')) return response(orderA)
      if (url.endsWith('/order-fixture-b')) return response(orderB)
    }
    throw new Error(`Unexpected fixture request ${options.method} ${url}`)
  }); vi.stubGlobal('fetch', fetch); return fetch
}
function Controls() { const location = useLocation(), navigate = useNavigate(); return <><output aria-label="测试当前地址">{location.pathname}</output><button onClick={() => void navigate('/orders/order-fixture-b')}>测试切到订单 B</button></> }
function mount(path = `/orders/${orderA.public_id}`, session: Session = account()) {
  const controller = new SessionController(fakeAPI(session))
  render(<MemoryRouter initialEntries={[path]}><AdminTheme><ConfigProvider theme={{ token: { motion: false } }}><SessionProvider controller={controller}><Controls /><AdminRoot renderPage={route => {
    const Page = businessPages[route.path as keyof typeof businessPages]
    return <Suspense fallback={<PageLoading />}><Page /></Suspense>
  }} /></SessionProvider></ConfigProvider></AdminTheme></MemoryRouter>); return controller
}
const writes = (fetch: ReturnType<typeof fixtureFetch>) => fetch.mock.calls.filter(([url, options]) => options.method !== 'GET' && !url.endsWith('/orders/search'))
function change(name: string, value: string) { fireEvent.change(screen.getByRole('textbox', { name }), { target: { value } }) }
async function prepareRefund(amount = '1.23') {
  await userEvent.click(await screen.findByRole('combobox', { name: '选择付款' }))
  await userEvent.click(screen.getByText(/fixture-payment-a · 可退/, { selector: '.ant-select-item-option-content' }))
  change('退款金额', amount); await userEvent.click(screen.getByRole('button', { name: '提交退款' }))
}
async function confirmRefund() { await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: '提交退款' })) }
function settledOrder(state: 'pending' | 'succeeded' = 'pending'): Order {
  return { ...orderA, refunded_cents: state === 'succeeded' ? 123 : 0, payment_status: state === 'succeeded' ? 'partially_refunded' : 'paid', receipts: [{ ...orderA.receipts[0]!, refunded_cents: state === 'succeeded' ? 123 : 0 }], refund_requests: [{ ...refundRequest, state, gateway_ref: 'fixture-gateway-ref' }] }
}
it('订单列表隐藏未授权邮箱，即使接口误返回明文；员工不获得 CSV 链接', async () => {
  fixtureFetch(); mount('/orders', account('staff', ['orders.view']))
  await screen.findByRole('link', { name: orderA.public_id }); expect(document.body.textContent).not.toContain(orderA.email); expect(screen.getByText('已隐藏')).toBeTruthy()
  expect(screen.queryByRole('link', { name: '导出订单 CSV' })).toBeNull()
  await userEvent.click(screen.getByRole('link', { name: orderA.public_id })); await screen.findByText('客户资料和支付参考号已隐藏。')
  for (const secret of [orderA.email, orderA.ship_name, orderA.ship_phone, orderA.ship_address, orderA.receipts[0]!.reference]) expect(document.body.textContent).not.toContain(secret)
  expect(screen.queryByRole('button', { name: '确认发货' })).toBeNull(); expect(screen.queryByRole('button', { name: '提交退款' })).toBeNull()
})
it('演示订单只读脱敏，伪造权限的员工仍无退款、平台核对、撤销和导出入口', async () => {
  fixtureFetch(); mount(`/orders/${orderA.public_id}`, account('staff', ['orders.view', 'orders.refund', 'orders.export', 'orders.fulfill']))
  await screen.findByText('客户资料和支付参考号已隐藏。')
  for (const name of ['提交退款', '确认发货', '撤销顾客访问']) expect(screen.queryByRole('button', { name })).toBeNull()
  expect(screen.queryByRole('button', { name: /核对平台退款记录/ })).toBeNull()
})
it('演示可查看订单状态与金额，顾客资料与所有写入口隐藏', async () => {
  const fetch = fixtureFetch(); mount(`/orders/${orderA.public_id}`, account('demo'))
  await screen.findByText('客户资料和支付参考号已隐藏。'); expect(document.body.textContent).toContain('12.00 USD'); expect(document.body.textContent).not.toContain(orderA.email)
  expect(screen.queryByRole('button', { name: '提交退款' })).toBeNull(); expect(screen.queryByRole('button', { name: '撤销顾客访问' })).toBeNull(); expect(writes(fetch)).toHaveLength(0)
})
it('店主 CSV 原生下载保留状态/UTC 日期范围，错误范围没有下载链接', async () => {
  const fetch = fixtureFetch(); mount('/orders'); const link = await screen.findByRole('link', { name: '导出订单 CSV' })
  expect(link.getAttribute('href')).toBe('/admin/api/orders/export'); expect(link.getAttribute('target')).toBe('_blank')
  fireEvent.change(screen.getByLabelText('导出开始日期'), { target: { value: '2026-10-01' } }); fireEvent.change(screen.getByLabelText('导出结束日期'), { target: { value: '2026-10-01' } })
  await userEvent.click(screen.getByRole('combobox', { name: '导出订单状态' })); await userEvent.click(screen.getByText('已付款', { selector: '.ant-select-item-option-content' }))
  expect(link.getAttribute('href')).toBe('/admin/api/orders/export?status=paid&from=2026-10-01&to=2026-10-01')
  fireEvent.change(screen.getByLabelText('导出结束日期'), { target: { value: '2026-09-30' } }); expect(screen.getByText('结束日期不能早于开始日期')).toBeTruthy(); expect(screen.queryByRole('link', { name: '导出订单 CSV' })).toBeNull(); expect(fetch).toHaveBeenCalledTimes(1)
})
it('发货需要客户资料与发货权限；仅有客户资料的员工只能查看收货信息', async () => {
  fixtureFetch(); mount(`/orders/${orderA.public_id}`, account('staff', ['orders.view', 'orders.customer_data']))
  await screen.findByText(orderA.email); expect(document.body.textContent).toContain(orderA.ship_address); expect(screen.queryByRole('button', { name: '确认发货' })).toBeNull(); expect(screen.queryByRole('button', { name: '提交退款' })).toBeNull()
})
it('授权员工验证承运商/运单/HTTPS，成功后按新详情展示运单，重复提交受锁保护', async () => {
  const pending = deferred<Response>(); let fulfilled = false
  const fetch = fixtureFetch(url => {
    if (url.endsWith('/fulfill')) return pending.promise
    if (fulfilled && url.endsWith('/order-fixture-a')) return response({ ...orderA, status: 'fulfilled', fulfillment_status: 'fulfilled', tracking_carrier: 'Fixture Carrier', tracking_number: 'TEST-123', tracking_url: 'https://tracking.example.invalid/123' })
  }); mount(`/orders/${orderA.public_id}`, account('staff', ['orders.view', 'orders.customer_data', 'orders.fulfill']))
  const button = await screen.findByRole('button', { name: '确认发货' }); await userEvent.click(button); await screen.findByText('请填写承运商'); expect(writes(fetch)).toHaveLength(0)
  change('承运商', ' Fixture Carrier '); change('运单号', ' TEST-123 '); change('追踪链接（可选）', 'javascript:bad()'); fireEvent.submit(button.closest('form')!); await screen.findByText(/追踪链接须为 HTTPS/); expect(writes(fetch)).toHaveLength(0)
  change('追踪链接（可选）', 'https://tracking.example.invalid/123'); fireEvent.submit(button.closest('form')!); fireEvent.submit(button.closest('form')!)
  await waitFor(() => expect(writes(fetch)).toHaveLength(1)); fulfilled = true; await act(async () => pending.resolve(response({ ok: true, status: 'fulfilled' })))
  expect((await screen.findByRole('link', { name: '查看物流追踪' })).getAttribute('href')).toBe('https://tracking.example.invalid/123'); expect(screen.queryByRole('button', { name: '确认发货' })).toBeNull()
  expect(JSON.parse(writes(fetch)[0]![1].body as string)).toEqual({ carrier: 'Fixture Carrier', tracking_number: 'TEST-123', tracking_url: 'https://tracking.example.invalid/123' })
})
it('未决退款或资金核对屏障即使订单仍标 paid 也不允许发货或新退款', async () => {
  fixtureFetch(url => url.endsWith('/order-fixture-a') ? response({ ...orderA, refund_requests: [refundRequest], receipts: [{ ...orderA.receipts[0]!, refund_reconciliation_required: true }] }) : undefined)
  mount(); await screen.findByText(/平台退款尚待确认/); expect(screen.queryByRole('button', { name: '确认发货' })).toBeNull(); expect(screen.queryByRole('button', { name: '提交退款' })).toBeNull(); expect(screen.getByRole('button', { name: '核对平台退款记录：1' })).toBeTruthy()
})
it('已发货后退款仍展示原运单，历史失败退款不冒充当前已退资金', async () => {
  fixtureFetch(url => url.endsWith('/order-fixture-a') ? response({ ...orderA, status: 'refunded', payment_status: 'refunded', fulfillment_status: 'fulfilled', refunded_cents: 1200, receipts: [{ ...orderA.receipts[0]!, refunded_cents: 1200 }], tracking_carrier: 'Fixture Carrier', tracking_number: 'TEST-123', tracking_url: 'https://tracking.example.invalid/123', refunds: [{ provider: 'stripe', provider_refund_id: 'fixture-history-refund', amount_cents: 123, created_at: '2026-10-25', observed_state: 'failed' }] }) : undefined)
  mount(); await screen.findByText('发货与追踪记录', { exact: true }); expect(document.body.textContent).toContain('TEST-123'); expect(document.body.textContent).toContain('未成功'); expect(document.body.textContent).toContain('已退 12.00 USD'); expect(screen.queryByRole('button', { name: '确认发货' })).toBeNull()
})
it('退款必须二次确认付款与精确金额，取消不请求，0/超余额/第三位小数拒绝', async () => {
  const fetch = fixtureFetch(); mount(); await prepareRefund('1.23')
  const dialog = screen.getByRole('dialog'); expect(within(dialog).getByText('1.23 USD')).toBeTruthy(); expect(within(dialog).getByText('fixture-payment-a')).toBeTruthy(); expect(writes(fetch)).toHaveLength(0)
  await userEvent.click(within(dialog).getByRole('button', { name: /取\s*消/ }))
  for (const value of ['0', '12.01', '1.005']) { change('退款金额', value); await userEvent.click(screen.getByRole('button', { name: '提交退款' })); await screen.findByText(/请选择付款，并填写不超过可退余额/); expect(screen.queryByRole('dialog')).toBeNull() }
  expect(writes(fetch)).toHaveLength(0)
})
it('断网退款保留原付款/金额/键，刷新后仍锁定；继续原请求完全复用 payload 且不并发重提', async () => {
  let postCalls = 0
  const fetch = fixtureFetch((url, options) => {
    if (url.endsWith('/refunds') && options.method === 'POST') { postCalls++; return postCalls === 1 ? response({ error: 'Gateway unconfirmed' }, 502) : response(refundResult, 202) }
    if (url.endsWith('/order-fixture-a') && postCalls >= 2) return response(settledOrder())
  }); mount(); await prepareRefund(); await confirmRefund(); await screen.findByText('原退款请求尚未确认')
  expect(screen.getByText('Gateway unconfirmed')).toBeTruthy(); expect((screen.getByRole('textbox', { name: '退款金额' }) as HTMLInputElement).disabled).toBe(true)
  change('退款金额', '9.99'); await userEvent.click(screen.getByRole('button', { name: '刷新订单' })); await screen.findByRole('combobox', { name: '选择付款' }); expect(screen.getByText('原退款请求尚未确认')).toBeTruthy()
  const continueButton = screen.getByRole('button', { name: '继续原退款请求' }); fireEvent.click(continueButton); fireEvent.click(continueButton)
  await waitFor(() => expect(writes(fetch)).toHaveLength(2)); await waitFor(() => expect(screen.queryByText('原退款请求尚未确认')).toBeNull())
  expect(writes(fetch)[0]![1].body).toBe(writes(fetch)[1]![1].body); const body = JSON.parse(writes(fetch)[1]![1].body as string); expect(body.amount_cents).toBe(123); expect(body.expected_refunded_cents).toBe(0); expect(body.request_key).toMatch(/^[a-f0-9-]{36}$/)
  expect(screen.queryByRole('button', { name: '确认发货' })).toBeNull()
})
it('退款已确认但详情刷新失败保留请求 ID，冻结新退款发货，可核对原请求恢复', async () => {
  let stage = 0
  const fetch = fixtureFetch((url, options) => {
    if (url.endsWith('/refunds') && options.method === 'POST') { stage = 1; return response(refundResult, 202) }
    if (url.endsWith('/refresh')) { stage = 2; return response({ ...refundResult, state: 'succeeded' }) }
    if (url.endsWith('/order-fixture-a') && stage === 1) return response({ error: 'Read unavailable' }, 500)
    if (url.endsWith('/order-fixture-a') && stage === 2) return response(settledOrder('succeeded'))
  }); mount(); await prepareRefund(); await confirmRefund(); await screen.findByText('已收到退款请求结果')
  expect(screen.queryByRole('button', { name: '继续原退款请求' })).toBeNull(); expect(document.body.textContent).toContain(refundResult.public_id)
  expect((screen.getByRole('button', { name: '提交退款' }) as HTMLButtonElement).disabled).toBe(true); expect((screen.getByRole('button', { name: '确认发货' }) as HTMLButtonElement).disabled).toBe(true)
  await userEvent.click(screen.getByRole('button', { name: '核对此退款请求' })); await waitFor(() => expect(screen.queryByText('已收到退款请求结果')).toBeNull())
  expect(screen.getByRole('button', { name: '提交退款' })).toBeTruthy(); expect(writes(fetch).filter(([url]) => url.endsWith('/refunds'))).toHaveLength(1); expect(writes(fetch).filter(([url]) => url.endsWith('/refresh'))).toHaveLength(1)
})
it('手动输入平台退款编号核对已有请求，不创建新退款；平台清单核对只有读取语义提示', async () => {
  const fetch = fixtureFetch((url, options) => {
    if (url.endsWith('/order-fixture-a')) return response({ ...orderA, refund_requests: [refundRequest] })
    if (url.endsWith('/refresh')) return response(refundResult)
    if (url.endsWith('/refund-check') && options.method === 'POST') return response({ public_id: 'refund-check-fixture', refunded_cents: 0, pending: true })
  }); mount(); await screen.findByLabelText(`支付平台退款编号：${refundRequest.public_id}`); fireEvent.change(screen.getByLabelText(`支付平台退款编号：${refundRequest.public_id}`), { target: { value: ' fixture-gateway-ref ' } })
  await userEvent.click(screen.getByRole('button', { name: `继续处理退款：${refundRequest.public_id}` })); await waitFor(() => expect(writes(fetch)).toHaveLength(1))
  expect(JSON.parse(writes(fetch)[0]![1].body as string)).toEqual({ gateway_ref: 'fixture-gateway-ref' })
  await waitFor(() => expect((screen.getByRole('button', { name: '核对平台退款记录：1' }) as HTMLButtonElement).disabled).toBe(false)); await userEvent.click(screen.getByRole('button', { name: '核对平台退款记录：1' }))
  await waitFor(() => expect(writes(fetch)).toHaveLength(2)); expect(writes(fetch)[1]![0]).toMatch(/\/refund-check$/); expect(writes(fetch).some(([url]) => url.endsWith('/refunds'))).toBe(false)
})
it('店主撤销访问需确认，取消不请求，成功不改变订单付款与发货信息', async () => {
  const fetch = fixtureFetch(url => url.endsWith('/revoke-access') ? response({ ok: true }) : undefined); mount()
  await userEvent.click(await screen.findByRole('button', { name: '撤销顾客访问' })); expect(within(screen.getByRole('dialog')).getByText(/所有设备上的查看凭证/)).toBeTruthy()
  await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: /取\s*消/ })); expect(writes(fetch)).toHaveLength(0)
  await userEvent.click(screen.getByRole('button', { name: '撤销顾客访问' })); await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: '撤销访问' }))
  await waitFor(() => expect(writes(fetch)).toHaveLength(1)); expect(JSON.parse(writes(fetch)[0]![1].body as string)).toEqual({}); expect(document.body.textContent).toContain('12.00 USD')
})
it('对象响应不匹配时拒绝展示可写详情，重试恢复；快速切换不显示旧订单', async () => {
  const pending = deferred<Response>(); let calls = 0
  fixtureFetch(url => {
    if (url.endsWith('/order-fixture-a')) { calls++; return calls === 1 ? response(orderB) : pending.promise }
  }); mount(); await screen.findByText('订单详情与所选订单不一致'); expect(screen.queryByRole('button', { name: '提交退款' })).toBeNull()
  await userEvent.click(screen.getByRole('button', { name: /重\s*试/ })); await userEvent.click(screen.getByRole('button', { name: '测试切到订单 B' })); await screen.findByText(orderB.email)
  await act(async () => pending.resolve(response(orderA))); expect(document.body.textContent).not.toContain(orderA.email); expect(document.body.textContent).toContain(orderB.email)
})
it('退款 401 清除会话和冻结请求，不能继续显示原支付或地址资料', async () => {
  const fetch = fixtureFetch(url => url.endsWith('/refunds') ? response({ error: 'Session expired' }, 401) : undefined); const controller = mount()
  await prepareRefund(); await confirmRefund(); await waitFor(() => expect(controller.getSnapshot().phase).toBe('login'))
  expect(document.body.textContent).not.toContain(orderA.email); expect(document.body.textContent).not.toContain(orderA.receipts[0]!.reference); expect(screen.queryByRole('button', { name: '继续原退款请求' })).toBeNull(); expect(writes(fetch)).toHaveLength(1)
})
