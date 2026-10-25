// 发货通知页面测试 / Shipment Notice Page Tests
// 功能：验证通知语义、权限、版本、未知请求恢复、确认重发、刷新失败与共享操作锁
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 16:25:07

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
import type { Session } from './contracts'
import { saleFixture } from './after-sale-fixtures'
import { orderA } from './order-fixtures'
import { legacyShipmentFixture, shipmentFixture, shippedOrder, noticeA, noticeBID, revisionA } from './shipment-fixtures'
import { noticeActionPayload, parseShipmentHistory, prepareShipmentIntent, shipmentAPI } from './shipment-api'
import { shipmentPayload, type Order } from './order-api'
function response(value: unknown, status = 200) { return new Response(JSON.stringify(value), { status }) }
async function mount(session: Session = account(), id = shippedOrder.public_id) {
  const controller = new SessionController(fakeAPI(session))
  render(<MemoryRouter initialEntries={[`/orders/${id}`]}><AdminTheme><ConfigProvider theme={{ token: { motion: false } }}><SessionProvider controller={controller}><AdminRoot renderPage={route => {
    const Page = businessPages[route.path as keyof typeof businessPages]; return <Suspense fallback={<PageLoading />}><Page /></Suspense>
  }} /></SessionProvider></ConfigProvider></AdminTheme></MemoryRouter>)
  // Wait for actual parent stages before asserting the child history request.
  // Do not combine auth, lazy imports and two reads into one selector deadline.
  await screen.findByRole('heading', { name: '订单详情', level: 2 })
  await screen.findByText(`订单 ${id}`)
  return controller
}
function fixture(order: () => Order = () => shippedOrder, history: () => unknown = shipmentFixture, handle?: (url: string, options: RequestInit) => Response | Promise<Response> | undefined) {
  const fetch = vi.fn(async (url: string, options: RequestInit) => {
    if (options.method === 'GET' && url.endsWith('/after-sales')) return response(saleFixture([], order()))
    const handled = await handle?.(url, options); if (handled) return handled
    if (options.method === 'GET') return response(url.endsWith('/shipment') ? history() : order())
    throw new Error(`Unexpected fixture write ${url}`)
  }); vi.stubGlobal('fetch', fetch); return fetch
}
const writes = (fetch: ReturnType<typeof fixture>) => fetch.mock.calls.filter(([, options]) => options.method !== 'GET')
async function dialogButton(name: string | RegExp) { await userEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name })) }

it('通知契约拒绝错对象、未知状态、重复 ID、混乱版本与缺失当前通知，非授权资料在解析前丢弃', () => {
  const base = shipmentFixture()
  for (const data of [
    { ...base, order_id: 'other-order' }, { ...base, notices: [{ ...noticeA, status: 'delivered' }] }, { ...base, notices: [noticeA, noticeA], notice_count: 2 },
    { ...base, current_revision: noticeBID }, { ...base, notices: [], notice_count: 0 }, { ...base, notices: [{ ...noticeA, revision_id: noticeBID }] },
    { ...base, revisions: [{ ...revisionA, version: 2 }] }, { ...base, notice_count: 51 },
  ]) expect(() => parseShipmentHistory(data, shippedOrder.public_id, true)).toThrow()
  const hidden = parseShipmentHistory({ ...base, revisions: [{ ...revisionA, tracking_number: { private: true }, tracking_url: { private: true } }], recipient: 'private@example.invalid', body: 'PRIVATE-BODY' }, shippedOrder.public_id, false)
  expect(hidden.revisions[0]!.tracking_number).toBe('已隐藏'); expect(hidden.revisions[0]!.tracking_url).toBe(''); expect(JSON.stringify(hidden)).not.toContain('PRIVATE-BODY'); expect(hidden).not.toHaveProperty('recipient')
  expect(parseShipmentHistory(legacyShipmentFixture(), shippedOrder.public_id, true).current_version).toBe(0)
})
it('API 白名单、版本和未知确认在请求前检查，拒绝不匹配的确认结果', async () => {
  const fetch = vi.fn(async (_url: string, _options: RequestInit) => response({ ok: true, notice_id: noticeA.public_id, action: 'retry' })); vi.stubGlobal('fetch', fetch)
  expect(() => noticeActionPayload({ notice_id: noticeA.public_id, action: 'resend_after_check', request_key: 'fixture-request-key-01', confirm_unknown: false })).toThrow()
  expect(() => prepareShipmentIntent({ kind: 'update', fields: { expected_revision: 'invalid', carrier: 'C', tracking_number: 'N', tracking_url: '' } })).toThrow()
  expect(fetch).not.toHaveBeenCalled()
  await expect(shipmentAPI.manage(shippedOrder.public_id, { notice_id: noticeA.public_id, action: 'retry', request_key: 'fixture-request-key-01', confirm_unknown: false })).rejects.toThrow('通知操作结果尚未确认')
  expect(JSON.parse(fetch.mock.calls[0]![1]!.body as string)).toEqual({ action: 'retry', request_key: 'fixture-request-key-01', confirm_unknown: false })
  for (const carrier of ['Carrier\nBcc: x', 'C'.repeat(129)]) expect(() => shipmentPayload({ carrier, tracking_number: 'N', tracking_url: '' })).toThrow()
})
it('员工无客户资料权限不能更正/重发，历史即使误返回明文也脱敏', async () => {
  fixture(); await mount(account('staff', ['orders.view', 'orders.fulfill']))
  await screen.findByText('发送失败')
  expect(screen.queryByRole('button', { name: '更正运单' })).toBeNull(); expect(screen.queryByRole('button', { name: '重试通知' })).toBeNull()
  await userEvent.click(screen.getByRole('button', { name: /运单历史/ }))
  expect(document.body.textContent).not.toContain(revisionA.tracking_number); expect(document.querySelector(`a[href="${revisionA.tracking_url}"]`)).toBeNull()
})
it('邮件服务器接收与商家确认收到分别展示，成功邮件没有重复重发入口', async () => {
  fixture(() => shippedOrder, () => ({ ...shipmentFixture(), notices: [{ ...noticeA, status: 'sent', attempts: 1, reason: '', sent_at: noticeA.created_at }] })); await mount()
  await screen.findByText('邮件服务器已接收')
  expect(screen.queryByRole('button', { name: '重试通知' })).toBeNull(); expect(screen.queryByRole('button', { name: '核对后重发' })).toBeNull()
  expect(document.body.textContent).toContain('不等于顾客已收到')
})
it('历史运单保持无通知空态，更正明确带旧版本并保存新追踪，不重复发货', async () => {
  let saved = false; let next: Order = shippedOrder
  const history = () => saved ? { ...shipmentFixture(), current_revision: '019a0000-0000-7000-8000-000000000002', current_version: 2, revision_count: 2,
    revisions: [{ ...revisionA, public_id: '019a0000-0000-7000-8000-000000000002', version: 2, tracking_number: 'TRACK-2' }, { ...revisionA, legacy: true }],
    notices: [{ ...noticeA, revision_id: '019a0000-0000-7000-8000-000000000002', version: 2, status: 'pending', attempts: 0, origin: 'tracking_updated', reason: '' }] } : legacyShipmentFixture()
  const fetch = fixture(() => next, history, (_url, options) => {
    if (options.method === 'PATCH') { const input = JSON.parse(options.body as string); saved = true; next = { ...shippedOrder, tracking_number: input.tracking_number }; return response({ ok: true }) }
  }); await mount(); await screen.findByText(/历史发货记录没有通知历史/)
  await userEvent.click(screen.getByRole('button', { name: '更正运单' })); fireEvent.change(screen.getByRole('textbox', { name: '更正运单号' }), { target: { value: 'TRACK-2' } })
  expect(within(screen.getByRole('dialog')).getByText(/原发货时间、付款、退款和库存不变/)).toBeTruthy()
  await dialogButton('保存更正并通知'); await waitFor(() => expect(writes(fetch)).toHaveLength(1))
  expect(writes(fetch)[0]![1].method).toBe('PATCH'); expect(JSON.parse(writes(fetch)[0]![1].body as string)).toEqual({ expected_revision: '', carrier: shippedOrder.tracking_carrier, tracking_number: 'TRACK-2', tracking_url: shippedOrder.tracking_url })
  expect(writes(fetch).some(([url]) => url.endsWith('/fulfill'))).toBe(false)
  await screen.findByText(/当前运单第 2 版/); expect(screen.queryByText(/运单已变化，请先使用/)).toBeNull()
})
it('重试要二次确认，取消不写；成功刷新失败不误报未创建，也不自动再次写入', async () => {
  let saved = false
  const fetch = fixture(() => shippedOrder, shipmentFixture, (url, options) => {
    if (options.method === 'POST') { saved = true; return response({ ok: true, notice_id: noticeBID, action: 'retry' }) }
    if (saved && url.endsWith('/order-fixture-a')) return response({ error: 'Order read unavailable' }, 500)
  }); await mount(); await userEvent.click(await screen.findByRole('button', { name: '重试通知' }))
  expect(writes(fetch)).toHaveLength(0); await dialogButton(/取\s*消/)
  await userEvent.click(screen.getByRole('button', { name: '重试通知' })); await dialogButton('确认重试')
  await screen.findByText('已收到通知操作确认，详情尚待刷新核对。')
  expect(screen.queryByText('原通知操作尚未确认')).toBeNull(); expect(writes(fetch)).toHaveLength(1)
  expect((screen.getByRole('button', { name: '提交退款' }) as HTMLButtonElement).disabled).toBe(true)
})
it('未知投递重发必须勾选核对，收到确认仅发原动作，不伪造服务器接收结果', async () => {
  let done = false
  const history = () => ({ ...shipmentFixture(), notices: [{ ...noticeA, status: 'delivery_unknown', reason: 'delivery_unknown', resolution: done ? 'received' : '' }] })
  const fetch = fixture(() => shippedOrder, history, (_url, options) => { if (options.method === 'POST') { done = true; return response({ ok: true, notice_id: noticeA.public_id, action: 'received' }) } }); await mount()
  await userEvent.click(await screen.findByRole('button', { name: '核对后重发' }))
  expect((within(screen.getByRole('dialog')).getByRole('button', { name: '确认重发' }) as HTMLButtonElement).disabled).toBe(true); expect(writes(fetch)).toHaveLength(0)
  await dialogButton(/取\s*消/)
  await userEvent.click(screen.getByRole('button', { name: '确认已收到' }))
  await userEvent.click(screen.getByRole('checkbox', { name: '我已核对，确认顾客已收到此邮件' })); await dialogButton('确认已收到')
  await screen.findByText('商家已核对收到'); expect(JSON.parse(writes(fetch)[0]![1].body as string)).toMatchObject({ action: 'received', confirm_unknown: true })
  expect(screen.queryByText('邮件服务器已接收')).toBeNull(); expect(screen.queryByRole('button', { name: '核对后重发' })).toBeNull()
})
it('网络中断保留原通知动作/键，刷新后仍可恢复；连点只提交原 payload 一次', async () => {
  let posts = 0; const pending = deferred<Response>()
  const fetch = fixture(() => shippedOrder, shipmentFixture, (_url, options) => { if (options.method === 'POST') { posts++; return posts === 1 ? Promise.reject(new TypeError('Network unavailable')) : pending.promise } }); await mount()
  await userEvent.click(await screen.findByRole('button', { name: '重试通知' })); await dialogButton('确认重试')
  await screen.findByText('原通知操作尚未确认'); expect((screen.getByRole('button', { name: '提交退款' }) as HTMLButtonElement).disabled).toBe(true)
  await userEvent.click(screen.getByRole('button', { name: '刷新订单' })); await screen.findByText('发送失败'); expect(screen.getByText('原通知操作尚未确认')).toBeTruthy()
  const resume = screen.getByRole('button', { name: '继续原通知操作' }); fireEvent.click(resume); fireEvent.click(resume)
  await waitFor(() => expect(writes(fetch)).toHaveLength(2)); expect(writes(fetch)[0]![1].body).toBe(writes(fetch)[1]![1].body)
  await act(async () => pending.resolve(response({ ok: true, notice_id: noticeBID, action: 'retry' })))
  await waitFor(() => expect(screen.queryByText('原通知操作尚未确认')).toBeNull()); expect(writes(fetch)).toHaveLength(2)
})
it('通知读取失败不当作历史空态，过期运单快照停止更正；401 清除私密页面', async () => {
  let mode: 'failed' | 'changed' | 'unauthorized' = 'failed'
  const fetch = fixture(() => shippedOrder, shipmentFixture, (url, options) => {
    if (url.endsWith('/shipment') && options.method === 'GET') return mode === 'failed' ? response({ error: 'History unavailable' }, 500) : mode === 'unauthorized' ? response({ error: 'Expired' }, 401) : response({ ...shipmentFixture(), revisions: [{ ...revisionA, tracking_number: 'CHANGED-ELSEWHERE' }] })
  }); await mount(); await screen.findByText('History unavailable'); expect(screen.queryByRole('button', { name: '更正运单' })).toBeNull(); expect(screen.queryByText(/历史发货记录没有通知历史/)).toBeNull()
  mode = 'changed'; await userEvent.click(screen.getByRole('button', { name: '刷新通知' })); await screen.findByText(/运单已变化，请先使用/)
  expect((screen.getByRole('button', { name: '更正运单' }) as HTMLButtonElement).disabled).toBe(true)
  mode = 'unauthorized'; await userEvent.click(screen.getByRole('button', { name: '刷新通知' })); await screen.findByLabelText('用户名'); expect(document.body.textContent).not.toContain(shippedOrder.email); expect(writes(fetch)).toHaveLength(0)
})
it('首次发货响应丢失保留原运单，订单读取也失败时仍能原请求恢复', async () => {
  let posts = 0, brokenRead = false
  const fetch = fixture(() => posts > 1 ? shippedOrder : orderA, legacyShipmentFixture, (url, options) => {
    if (options.method === 'POST') { posts++; return posts === 1 ? response({ error: 'Result unknown' }, 500) : response({ ok: true, status: 'fulfilled' }) }
    if (brokenRead && url.endsWith('/order-fixture-a')) return response({ error: 'Read unavailable' }, 500)
  }); await mount(); fireEvent.change(await screen.findByRole('textbox', { name: '承运商' }), { target: { value: 'Fixture Carrier' } }); fireEvent.change(screen.getByRole('textbox', { name: '运单号' }), { target: { value: revisionA.tracking_number } })
  await userEvent.click(screen.getByRole('button', { name: '确认发货' })); await screen.findByText('原发货操作尚未确认')
  brokenRead = true; await userEvent.click(screen.getByRole('button', { name: '刷新订单' })); await screen.findByText('Read unavailable')
  brokenRead = false; await userEvent.click(screen.getByRole('button', { name: '继续原发货操作' })); await waitFor(() => expect(screen.queryByText('原发货操作尚未确认')).toBeNull())
  expect(writes(fetch)).toHaveLength(2); expect(writes(fetch)[0]![1].body).toBe(writes(fetch)[1]![1].body)
})

it('授权发货员工可明确核对后重发，传正确动作与确认，旧未知证据继续显示', async () => {
  let queued = false
  const history = () => {
    const old = { ...noticeA, status: 'delivery_unknown', reason: 'delivery_unknown', resolution: queued ? 'not_received' : '' }
    return { ...shipmentFixture(), notice_count: queued ? 2 : 1, notices: queued ? [{ ...noticeA, public_id: noticeBID, sequence: 2, status: 'pending', attempts: 0, origin: 'resend_after_check', reason: '' }, old] : [old] }
  }
  const fetch = fixture(() => shippedOrder, history, (_url, options) => { if (options.method === 'POST') { queued = true; return response({ ok: true, notice_id: noticeBID, action: 'resend_after_check' }) } })
  await mount(account('staff', ['orders.view', 'orders.customer_data', 'orders.fulfill']))
  await screen.findByText(`订单 ${shippedOrder.public_id}`)
  await userEvent.click(await screen.findByRole('button', { name: '核对后重发' })); await userEvent.click(screen.getByRole('checkbox', { name: '我已核对，确认未收到，接受重复邮件风险后重发' })); await dialogButton('确认重发')
  await screen.findByText('等待发送'); expect(screen.queryByRole('button', { name: '提交退款' })).toBeNull(); expect(writes(fetch)).toHaveLength(1)
  expect(JSON.parse(writes(fetch)[0]![1].body as string)).toMatchObject({ action: 'resend_after_check', confirm_unknown: true })
  await userEvent.click(screen.getByRole('button', { name: /更多投递记录/ })); await screen.findByText('已核对未收到并重发'); expect(screen.getByText('投递结果待核对')).toBeTruthy()
})

it('运单更正网络中断锁定原版本与字段，恢复不换内容；明确冲突只提示刷新', async () => {
  let patches = 0
  const fetch = fixture(() => shippedOrder, shipmentFixture, (_url, options) => {
    if (options.method === 'PATCH') { patches++; return patches === 1 ? response({ error: 'Update unconfirmed' }, 502) : response({ error: 'Revision changed' }, 409) }
  }); await mount(); await userEvent.click(await screen.findByRole('button', { name: '更正运单' })); fireEvent.change(screen.getByRole('textbox', { name: '更正运单号' }), { target: { value: 'TRACK-NEW' } }); await dialogButton('保存更正并通知')
  await screen.findByText('原运单更正尚未确认'); expect(screen.queryByRole('dialog')).toBeNull()
  await userEvent.click(screen.getByRole('button', { name: '刷新订单' })); await screen.findByText('发送失败')
  await userEvent.click(screen.getByRole('button', { name: '继续原运单更正' })); await waitFor(() => expect(screen.queryByText('原运单更正尚未确认')).toBeNull())
  expect(writes(fetch)).toHaveLength(2); expect(writes(fetch)[0]![1].body).toBe(writes(fetch)[1]![1].body)
  expect(JSON.parse(writes(fetch)[0]![1].body as string)).toMatchObject({ expected_revision: revisionA.public_id, tracking_number: 'TRACK-NEW' })
  expect(screen.getByText('Revision changed')).toBeTruthy()
})

it('无效更正链接不提交或锁住表单，修正后仍使用打开时的原版本', async () => {
  const fetch = fixture(() => shippedOrder, shipmentFixture, (_url, options) => options.method === 'PATCH' ? response({ error: 'Fixture version conflict' }, 409) : undefined)
  await mount(); await userEvent.click(await screen.findByRole('button', { name: '更正运单' }))
  const url = screen.getByRole('textbox', { name: '更正追踪链接（可选）' })
  fireEvent.change(url, { target: { value: 'http://carrier.example.invalid/tracking' } }); await dialogButton('保存更正并通知')
  await waitFor(() => expect(within(screen.getByRole('dialog')).getByText(/追踪链接须为 HTTPS/)).toBeTruthy())
  expect(writes(fetch)).toHaveLength(0); expect(screen.queryByText('原运单更正尚未确认')).toBeNull()
  await waitFor(() => expect((within(screen.getByRole('dialog')).getByRole('button', { name: '保存更正并通知' }) as HTMLButtonElement).disabled).toBe(false))
  fireEvent.change(url, { target: { value: 'https://carrier.example.invalid/tracking-new' } }); await dialogButton('保存更正并通知')
  await within(screen.getByRole('dialog')).findByText('Fixture version conflict')
  expect(writes(fetch)).toHaveLength(1); expect(JSON.parse(writes(fetch)[0]![1].body as string)).toMatchObject({ expected_revision: revisionA.public_id, tracking_url: 'https://carrier.example.invalid/tracking-new' })
})

it('轮询只读更新状态，隐藏页面不请求；读取失败清除旧状态且不冒充无通知', async () => {
  const timers: (() => void)[] = []
  vi.spyOn(window, 'setInterval').mockImplementation((handler, delay) => { if (delay === 20000 && typeof handler === 'function') timers.push(handler as () => void); const handle = setTimeout(() => {}, 0); clearTimeout(handle); return handle })
  const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
  let mode: 'pending' | 'sent' | 'failed' = 'pending'
  const fetch = fixture(() => shippedOrder, () => ({ ...shipmentFixture(), notices: [{ ...noticeA, status: mode, attempts: 0, reason: '', sent_at: mode === 'sent' ? noticeA.created_at : '' }] }), (url, options) => url.endsWith('/shipment') && options.method === 'GET' && mode === 'failed' ? response({ error: 'Poll unavailable' }, 500) : undefined)
  await mount(); await screen.findByText('等待发送'); await waitFor(() => expect(timers.length).toBeGreaterThan(0))
  const before = fetch.mock.calls.length; visibility.mockReturnValue('hidden'); await act(async () => timers.at(-1)!()); expect(fetch).toHaveBeenCalledTimes(before)
  visibility.mockReturnValue('visible'); mode = 'sent'; await act(async () => timers.at(-1)!()); await screen.findByText('邮件服务器已接收')
  mode = 'failed'; await userEvent.click(screen.getByRole('button', { name: '刷新通知' })); await screen.findByText('Poll unavailable')
  expect(screen.queryByText('邮件服务器已接收')).toBeNull(); expect(screen.queryByText(/历史发货记录没有通知历史/)).toBeNull(); expect(writes(fetch)).toHaveLength(0)
})

it('公开演示不请求真实通知，读取到伪造已发货订单也不加载历史', async () => {
  const fetch = fixture(); await mount(account('demo'))
  await screen.findByText('公开演示不读取真实发货通知。')
  expect(fetch.mock.calls.some(([url]) => url.endsWith('/shipment'))).toBe(false)
})
