// 售后审核页面测试 / After Sale Page Interaction Tests
// 功能：真实订单页审核/退回/办结、权限与共享锁、失败原键恢复、说明保留及迟到响应隔离
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 22:55:10

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
import { orderA, refundRequest } from './order-fixtures'
import { nextSale, saleFixture, saleInitial, saleKey, saleOtherID, saleRefundID, saleTime } from './after-sale-fixtures'
import type { Session } from './contracts'
import type { Order } from './order-api'
import type { AfterSaleCase, AfterSaleIntent } from './after-sale-api'

function response(value: unknown, status = 200) { return new Response(JSON.stringify(value), { status }) }
async function mount(session: Session = account()) {
  const controller = new SessionController(fakeAPI(session))
  render(<MemoryRouter initialEntries={[`/orders/${orderA.public_id}`]}><AdminTheme><ConfigProvider theme={{ token: { motion: false } }}><SessionProvider controller={controller}><AdminRoot renderPage={route => {
    const Page = businessPages[route.path as keyof typeof businessPages]; return <Suspense fallback={<PageLoading />}><Page /></Suspense>
  }} /></SessionProvider></ConfigProvider></AdminTheme></MemoryRouter>)
  await screen.findByRole('heading', { name: '订单详情', level: 2 })
  return controller
}
function fixture(cases: AfterSaleCase[] = [structuredClone(saleInitial)], order: Order = structuredClone(orderA)) {
  let nonce = 0
  const state = { cases, order, key: saleKey, readsFail: false, mutate: null as ((p: AfterSaleIntent) => Promise<Response> | Response) | null, read: null as (() => Promise<Response> | Response) | null }
  const fetch = vi.fn(async (url: string, options: RequestInit) => {
    if (options.method === 'GET') {
      if (state.readsFail) return response({ error: 'FIXTURE-READ-FAILED' }, 500)
      if (url.endsWith('/after-sales')) return state.read ? state.read() : response(saleFixture(state.cases, state.order, state.key))
      return response(state.order)
    }
    if (!url.includes('/after-sales/') || !url.endsWith('/actions')) throw new Error(`Unexpected mutation ${url}`)
    const p = JSON.parse(options.body as string) as AfterSaleIntent
    if (state.mutate) return state.mutate(p)
    const c = state.cases[0]!
    const next = p.action === 'reject' ? 'rejected' : p.action === 'receive_return' ? 'returned' : p.action === 'complete' ? 'completed' : c.kind === 'return_refund' && c.state === 'requested' ? 'awaiting_return' : 'approved'
    const updated = nextSale(c, p.action, next, p.message, p.action === 'complete' ? { refund_request_id: p.refund_request_id } : next === 'awaiting_return' ? { return_instructions: p.message } : {})
    state.cases = [updated]
    state.key = `00000000-0000-7000-8000-${(0x200 + ++nonce).toString(16).padStart(12, '0')}`
    return response({ ok: true, order_id: orderA.public_id, case: updated })
  }); vi.stubGlobal('fetch', fetch)
  return { state, fetch }
}
const writes = (fetch: ReturnType<typeof fixture>['fetch']) => fetch.mock.calls.filter(([, options]) => options.method !== 'GET')
function panel() { return screen.getByText('售后申请', { exact: true, selector: '.ant-card-head-title' }).closest('.ant-card') as HTMLElement }
async function ready() { await screen.findByText(`订单 ${orderA.public_id}`); await screen.findByText(saleInitial.reason) }
async function open(name = '批准退款申请') { await userEvent.click(within(panel()).getByRole('button', { name })); return screen.findByRole('dialog') }
function fillMessage(dialog: HTMLElement, value: string, label = '处理说明（顾客可见）') { fireEvent.change(within(dialog).getByLabelText(label), { target: { value } }) }
async function submit(dialog: HTMLElement, name = '批准退款申请') { await userEvent.click(within(dialog).getByRole('button', { name })) }
async function close(dialog: HTMLElement) { await userEvent.click(within(dialog).getByRole('button', { name: '关闭并保留说明' })) }

it.each(['staff', 'demo'] as const)('%s 无客户资料时不请求或保留售后全文，伪造处理权限也无入口', async role => {
  const { fetch } = fixture(); await mount(account(role, ['orders.view', 'orders.aftersales']))
  await screen.findByText('客户资料和支付参考号已隐藏。')
  expect(fetch.mock.calls.some(([url]) => url.endsWith('/after-sales'))).toBe(false)
  expect(document.body.textContent).not.toContain(saleInitial.reason)
  expect(screen.queryByRole('button', { name: '批准退款申请' })).toBeNull()
})
it('只有订单与资料权限可看全文和历史，但不能审核或退款', async () => {
  fixture(); await mount(account('staff', ['orders.view', 'orders.customer_data'])); await ready()
  expect(screen.getByText('当前账号可查看，处理申请还需要“处理售后”权限。')).toBeTruthy()
  expect(screen.queryByRole('button', { name: '批准退款申请' })).toBeNull()
  expect(screen.queryByRole('button', { name: '提交退款' })).toBeNull()
  await userEvent.click(screen.getByRole('button', { name: '处理历史（1 条）' }))
  expect(screen.getByText('提交申请 · 顾客')).toBeTruthy()
})
it('有处理中售后暂停首次发货；有处理权限员工可批准申请但不调用资金或库存接口', async () => {
  const { fetch, state } = fixture(); await mount(account('staff', ['orders.view', 'orders.customer_data', 'orders.fulfill', 'orders.aftersales'])); await ready()
  expect(screen.queryByRole('button', { name: '确认发货' })).toBeNull()
  const dialog = await open(); fillMessage(dialog, ' Approved for the requested amount. ')
  await submit(dialog)
  await screen.findByText('退款已获批准，待实际处理', { selector: '.ant-tag' })
  expect(state.cases[0]?.state).toBe('approved')
  expect(writes(fetch)).toHaveLength(1)
  const [url, options] = writes(fetch)[0]!
  expect(url).toMatch(/\/after-sales\/.+\/actions$/)
  expect(JSON.parse(options.body as string)).toEqual({ request_key: saleKey, expected_version: 1, action: 'approve', message: 'Approved for the requested amount.', refund_request_id: '' })
  expect(screen.queryByRole('button', { name: '提交退款' })).toBeNull()
})
it('顾客理由作为文本展示；拒绝必须填写说明，未提交字段保持原样', async () => {
  const c = { ...structuredClone(saleInitial), reason: '<img src=x onerror=alert(1)> PRIVATE-REASON' }
  const { fetch } = fixture([c]); await mount(); await screen.findByText(c.reason)
  expect(panel().querySelector('img')).toBeNull()
  const dialog = await open('拒绝申请'); await submit(dialog, '拒绝申请')
  expect(writes(fetch)).toHaveLength(0)
  await within(dialog).findByText('请填写给顾客的说明')
  fillMessage(dialog, 'Please contact support to review the item details.')
  await submit(dialog, '拒绝申请')
  await screen.findByText('已拒绝', { selector: '.ant-tag' }); expect(writes(fetch)).toHaveLength(1)
})
it('退货批准明确填写指引；确认收货需人工勾选，检查批准与实际退款分别处理', async () => {
  const { fetch } = fixture([{ ...structuredClone(saleInitial), kind: 'return_refund' }]); await mount(); await ready()
  let dialog = await open('同意退货并给出指引')
  fillMessage(dialog, 'Contact support for a return label and the correct return address.', '退回指引（顾客可见）')
  await submit(dialog, '同意退货并给出指引')
  await screen.findByText('已同意退货，待收到包裹', { selector: '.ant-tag' })
  dialog = await open('确认实际收到退货'); fillMessage(dialog, 'Parcel received; inspection pending.')
  expect((within(dialog).getByRole('button', { name: '确认实际收到退货' }) as HTMLButtonElement).disabled).toBe(true)
  await userEvent.click(within(dialog).getByRole('checkbox', { name: '我已核对，确认实际收到退回包裹，后续仍需检查商品' }))
  await submit(dialog, '确认实际收到退货')
  await screen.findByText('已收到退货，待检查审核', { selector: '.ant-tag' })
  dialog = await open()
  expect((within(dialog).getByLabelText('处理说明（顾客可见）') as HTMLTextAreaElement).value).toBe('')
  fillMessage(dialog, 'Inspection passed. Approved for the requested amount.')
  await submit(dialog); await screen.findByText('退款已获批准，待实际处理', { selector: '.ant-tag' })
  expect(writes(fetch)).toHaveLength(3)
  expect(writes(fetch).map(([, o]) => JSON.parse(o.body as string).action)).toEqual(['approve', 'receive_return', 'approve'])
})
it('打开审核时暂停页内刷新；关闭再打开保留尚未提交的说明，不自动审核', async () => {
  const { fetch } = fixture(); await mount(); await ready()
  let dialog = await open(); fillMessage(dialog, 'Keep this unsent review draft.')
  expect((screen.getByRole('button', { name: '刷新订单' }) as HTMLButtonElement).disabled).toBe(true)
  expect((screen.getByRole('button', { name: '刷新售后与订单' }) as HTMLButtonElement).disabled).toBe(true)
  await close(dialog); dialog = await open()
  expect((within(dialog).getByLabelText('处理说明（顾客可见）') as HTMLTextAreaElement).value).toBe('Keep this unsent review draft.')
  expect(writes(fetch)).toHaveLength(0)
})
it('未知审核结果保留原说明和UUIDv7，页内刷新换了读取键也不覆盖原请求', async () => {
  const { state, fetch } = fixture(); let attempt = 0
  state.mutate = p => {
    attempt++
    if (attempt === 1) { state.cases = [nextSale(saleInitial, 'approve', 'approved', p.message)]; state.key = saleOtherID; throw new TypeError('Disconnected after commit') }
    return response({ ok: true, order_id: orderA.public_id, case: state.cases[0] })
  }
  await mount(); await ready()
  const dialog = await open(); fillMessage(dialog, 'Keep the exact original review.'); await submit(dialog)
  await screen.findByRole('button', { name: '继续原售后操作' })
  expect((screen.getByRole('button', { name: '提交退款' }) as HTMLButtonElement).disabled).toBe(true)
  await close(dialog)
  await userEvent.click(screen.getByRole('button', { name: '刷新订单' }))
  await screen.findByRole('button', { name: '继续原售后操作' })
  await waitFor(() => expect((screen.getByRole('button', { name: '继续原售后操作' }) as HTMLButtonElement).disabled).toBe(false))
  expect(writes(fetch)).toHaveLength(1)
  await userEvent.click(screen.getByRole('button', { name: '继续原售后操作' }))
  await screen.findByText('退款已获批准，待实际处理', { selector: '.ant-tag' })
  expect(writes(fetch)).toHaveLength(2)
  expect(writes(fetch)[0]?.[1].body).toBe(writes(fetch)[1]?.[1].body)
  expect(JSON.parse(writes(fetch)[1]![1].body as string).request_key).toBe(saleKey)
})
it('错误成功响应按未知处理，新退款/发货/售后均暂停且不会伪造已确认结果', async () => {
  const { state, fetch } = fixture()
  state.mutate = p => response({ ok: true, order_id: orderA.public_id, case: { ...nextSale(saleInitial, 'approve', 'approved', p.message), public_id: saleOtherID } })
  await mount(); await ready(); const dialog = await open(); fillMessage(dialog, 'Review this request.'); await submit(dialog)
  await screen.findByRole('button', { name: '继续原售后操作' })
  expect(screen.queryByText('已收到售后操作确认')).toBeNull()
  expect((screen.getByRole('button', { name: '提交退款' }) as HTMLButtonElement).disabled).toBe(true)
  expect(writes(fetch)).toHaveLength(1)
})
it('审核已确认而重读失败保留确认并设屏障，刷新恢复不重复审核', async () => {
  const { state, fetch } = fixture()
  state.mutate = p => { state.cases = [nextSale(saleInitial, 'approve', 'approved', p.message)]; state.readsFail = true; return response({ ok: true, order_id: orderA.public_id, case: state.cases[0] }) }
  await mount(); await ready(); const dialog = await open(); fillMessage(dialog, 'Confirmed once.'); await submit(dialog)
  await screen.findByText('已收到售后操作确认', { selector: '.ant-card-head-title' })
  expect(screen.queryByRole('button', { name: '继续原售后操作' })).toBeNull()
  expect(within(panel()).queryByText(saleInitial.reason)).toBeNull()
  expect((screen.getByRole('button', { name: '提交退款' }) as HTMLButtonElement).disabled).toBe(true)
  state.readsFail = false
  await userEvent.click(screen.getByRole('button', { name: '刷新订单' }))
  await screen.findByText('退款已获批准，待实际处理', { selector: '.ant-tag' })
  expect(writes(fetch)).toHaveLength(1)
})
it('售后读取失败不假设空历史或开放发货，401则清除全部私密界面', async () => {
  const { state, fetch } = fixture(); state.read = () => response({ error: 'AFTERSALE-READ-FAILED' }, 500)
  const controller = await mount(); await screen.findByText('AFTERSALE-READ-FAILED')
  expect(screen.queryByRole('button', { name: '确认发货' })).toBeNull()
  expect(screen.queryByRole('button', { name: '提交退款' })).toBeNull()
  expect(document.body.textContent).not.toContain(saleInitial.reason)
  state.read = () => response({ error: 'expired' }, 401)
  await userEvent.click(screen.getByRole('button', { name: '重试' }))
  await waitFor(() => expect(controller.getSnapshot().session).toBeNull())
  expect(document.body.textContent).not.toContain(orderA.email); expect(writes(fetch)).toHaveLength(0)
})
it('版本冲突保留说明和打开时记录，显式重读并采用新版本后才继续收货', async () => {
  const awaiting = nextSale({ ...structuredClone(saleInitial), kind: 'return_refund' }, 'approve', 'awaiting_return', 'Use return label', { return_instructions: 'Use return label' })
  const { state, fetch } = fixture([awaiting]); let attempt = 0
  state.mutate = p => {
    attempt++
    if (attempt === 1) {
      state.cases = [{ ...awaiting, version: 3, return_carrier: 'Return carrier', return_tracking_number: 'NEW-CUSTOMER-TRACK', events: [...awaiting.events, { version: 3, actor: 'customer', action: 'return_tracking', state: 'awaiting_return', message: '', carrier: 'Return carrier', tracking_number: 'NEW-CUSTOMER-TRACK', created_at: saleTime }] }]
      state.key = saleOtherID; return response({ error: 'VERSION-CHANGED' }, 409)
    }
    state.cases = [nextSale(state.cases[0]!, 'receive_return', 'returned', p.message)]
    return response({ ok: true, order_id: orderA.public_id, case: state.cases[0] })
  }
  await mount(); await ready(); const dialog = await open('确认实际收到退货'); fillMessage(dialog, 'Keep my receipt inspection note.')
  await userEvent.click(within(dialog).getByRole('checkbox')); await submit(dialog, '确认实际收到退货')
  await within(dialog).findByText('VERSION-CHANGED')
  expect(screen.queryByRole('button', { name: '继续原售后操作' })).toBeNull()
  await userEvent.click(within(dialog).getByRole('button', { name: '刷新并保留说明' }))
  await within(dialog).findByRole('button', { name: '采用最新记录重新核对' })
  expect((within(dialog).getByLabelText('处理说明（顾客可见）') as HTMLTextAreaElement).value).toBe('Keep my receipt inspection note.')
  await userEvent.click(within(dialog).getByRole('button', { name: '采用最新记录重新核对' }))
  await within(dialog).findByText('Return carrier · NEW-CUSTOMER-TRACK')
  expect((within(dialog).getByRole('checkbox') as HTMLInputElement).checked).toBe(false)
  await userEvent.click(within(dialog).getByRole('checkbox')); await submit(dialog, '确认实际收到退货')
  await screen.findByText('已收到退货，待检查审核', { selector: '.ant-tag' })
  const payloads = writes(fetch).map(([, o]) => JSON.parse(o.body as string))
  expect(payloads.map(p => p.expected_version)).toEqual([2, 3]); expect(payloads.map(p => p.request_key)).toEqual([saleKey, saleOtherID])
})
it('只关联已验证匹配退款办结；处理权限员工可以记办结而不获得实际退款入口', async () => {
  const approved = nextSale(saleInitial, 'approve', 'approved', 'Approved')
  const order: Order = { ...orderA, refunded_cents: 123, payment_status: 'partially_refunded', receipts: [{ ...orderA.receipts[0]!, refunded_cents: 123 }], refund_requests: [{ ...refundRequest, public_id: saleRefundID, state: 'succeeded', created_at: saleTime }] }
  const { fetch } = fixture([approved], order)
  await mount(account('staff', ['orders.view', 'orders.customer_data', 'orders.aftersales'])); await ready()
  const dialog = await open('关联成功退款并办结')
  await userEvent.click(within(dialog).getByRole('combobox', { name: '匹配的成功退款' }))
  await userEvent.click(screen.getByText(new RegExp(saleRefundID), { selector: '.ant-select-item-option-content' }))
  await submit(dialog, '关联成功退款并办结')
  await screen.findByText('已关联成功退款办结', { selector: '.ant-tag' })
  expect(writes(fetch)).toHaveLength(1)
  expect(JSON.parse(writes(fetch)[0]![1].body as string)).toEqual({ request_key: saleKey, expected_version: 2, action: 'complete', message: '', refund_request_id: saleRefundID })
  expect(screen.queryByRole('button', { name: '提交退款' })).toBeNull()
})
it('无成功退款不能办结，已办结后资金异常继续警示并保留历史', async () => {
  const approved = nextSale(saleInitial, 'approve', 'approved', 'Approved')
  const { state } = fixture([approved]); await mount(); await ready()
  expect((screen.getByRole('button', { name: '关联成功退款并办结' }) as HTMLButtonElement).disabled).toBe(true)
  state.cases = [nextSale(approved, 'complete', 'completed', '', { refund_request_id: saleRefundID, refund_needs_review: true })]
  await userEvent.click(screen.getByRole('button', { name: '刷新订单' }))
  await screen.findByText('已办结退款出现新的待核对情况。')
  expect(screen.getByText('已关联成功退款办结', { selector: '.ant-tag' })).toBeTruthy()
  expect(within(panel()).queryByRole('button', { name: '关联成功退款并办结' })).toBeNull()
})
it('同一写入等待期间共享操作锁阻止再次审核，未提交的长说明保留供修正', async () => {
  const { state, fetch } = fixture(); const pending = deferred<Response>()
  state.mutate = () => pending.promise
  await mount(); await ready(); const dialog = await open()
  fillMessage(dialog, '字'.repeat(1334)); await submit(dialog)
  await within(dialog).findByText('说明过长，请缩短后再提交'); expect(writes(fetch)).toHaveLength(0)
  fillMessage(dialog, 'One serialized review.')
  await submit(dialog); await waitFor(() => expect(writes(fetch)).toHaveLength(1))
  fireEvent.submit(within(dialog).getByLabelText('处理说明（顾客可见）').closest('form')!)
  expect(writes(fetch)).toHaveLength(1)
  await act(async () => { state.cases = [nextSale(saleInitial, 'approve', 'approved', 'One serialized review.')]; pending.resolve(response({ ok: true, order_id: orderA.public_id, case: state.cases[0] })) })
  await screen.findByText('退款已获批准，待实际处理', { selector: '.ant-tag' })
})
