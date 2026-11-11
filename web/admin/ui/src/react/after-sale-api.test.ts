// 售后接口边界测试 / After Sale API Boundary Tests
// 功能：有界与错对象拒绝、资料先拒绝、原键白名单、退款证据候选和重放结果验证
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 22:45:01

import { expect, it, vi } from 'vitest'
import { afterSaleAPI, allowedAfterSaleActions, matchingRefunds, parseAfterSales, prepareAfterSale, type AfterSaleCase, type AfterSaleIntent } from './after-sale-api'
import { nextSale, parsedSaleFixture, saleFixture, saleInitial, saleKey, saleOtherID, saleRefundID, saleTime } from './after-sale-fixtures'
import { orderA, refundRequest } from './order-fixtures'
const intent: AfterSaleIntent = { case_id: saleInitial.public_id, request_key: saleKey, expected_version: 1, action: 'approve', message: 'Approved for the requested amount.', refund_request_id: '', amount_cents: 123, currency: 'USD' }
const parse = (value: unknown) => parseAfterSales(value, orderA.public_id, orderA.currency, true)
it('售后在接触任何私密字段前拒绝无资料权限，也不发送读取请求', async () => {
  const value = Object.defineProperty({}, 'snapshot', { get() { throw new Error('PRIVATE getter was read') } })
  expect(() => parseAfterSales(value, orderA.public_id, 'USD', false)).toThrow('客户信息权限')
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch)
  await expect(afterSaleAPI.get(orderA.public_id, 'USD', false)).rejects.toThrow('客户信息权限')
  expect(fetch).not.toHaveBeenCalled()
})
it('售后读取拒绝错订单/币种、重复/过量/未连续事件、未知状态和错误政策默认值', () => {
  const base = saleFixture(), c = structuredClone(saleInitial)
  for (const value of [
    { ...base, order_id: 'other-order' }, { ...base, next_action_key: 'invalid' },
    ...[{ ...c, currency: 'EUR' }, { ...c, state: 'unknown' }, { ...c, amount_cents: 1.5 }, { ...c, version: 2 }, { ...c, events: [{ ...c.events[0], version: 2 }] }, { ...c, state: 'completed' }, { ...c, refund_request_id: saleRefundID }, { ...c, reason: 'x'.repeat(2001) }, { ...c, terms: { ...c.terms, return_window_days: 7 } }].map(c => ({ ...base, snapshot: { ...base.snapshot, cases: [c] } })),
    { ...base, snapshot: { ...base.snapshot, cases: [c, c] } }, { ...base, snapshot: { ...base.snapshot, cases: Array(21).fill(c) } },
    { ...base, snapshot: { ...base.snapshot, can_request_refund: true } },
  ]) expect(() => parse(value)).toThrow()
  const parsed = parse({ ...base, private_server_field: 'not returned' })
  expect(parsed.cases[0]?.reason).toBe(c.reason)
  expect(parsed).not.toHaveProperty('private_server_field')
  expect(parsed.cases[0]).not.toHaveProperty('request_hash')
})
it('售后准备固定UUIDv7和版本，保留整数分并只允许适用字段；输入规范化不改原对象', () => {
  expect(prepareAfterSale({ ...intent, message: ' A\r\nB ' }).message).toBe('A\nB')
  expect(intent.message).toBe('Approved for the requested amount.')
  for (const input of [{ ...intent, request_key: '00000000-0000-4000-8000-000000000102' }, { ...intent, expected_version: 0 }, { ...intent, message: ' ' }, { ...intent, message: '字'.repeat(1334) }, { ...intent, message: 'A\nB\x00C' }, { ...intent, refund_request_id: saleRefundID }, { ...intent, action: 'complete' as const, refund_request_id: '' }]) expect(() => prepareAfterSale(input)).toThrow()
  expect(prepareAfterSale({ ...intent, action: 'complete', refund_request_id: saleRefundID, message: '' }).refund_request_id).toBe(saleRefundID)
})
it('审核只发送服务端白名单，重复调用保持同一键，不调用退款/库存接口', async () => {
  const c = nextSale(saleInitial, 'approve', 'approved', intent.message)
  const fetch = vi.fn(async (_url: string, _options: RequestInit) => new Response(JSON.stringify({ ok: true, order_id: orderA.public_id, case: c })))
  vi.stubGlobal('fetch', fetch)
  await afterSaleAPI.review(orderA.public_id, intent); await afterSaleAPI.review(orderA.public_id, intent)
  expect(fetch.mock.calls[0]?.[0]).toBe(`/admin/api/orders/${orderA.public_id}/after-sales/${saleInitial.public_id}/actions`)
  expect(fetch.mock.calls[0]?.[1].body).toBe(fetch.mock.calls[1]?.[1].body)
  expect(JSON.parse(fetch.mock.calls[0]![1].body as string)).toEqual({ request_key: saleKey, expected_version: 1, action: 'approve', message: intent.message, refund_request_id: '' })
  expect(fetch.mock.calls.every(([url]) => !url.includes('/refunds') && !url.includes('/inventory'))).toBe(true)
})
it('审核拒绝错对象、金额、行为、说明与未确认成功，原键可核对较晚的同一操作结果', async () => {
  const approved = nextSale(saleInitial, 'approve', 'approved', intent.message)
  let response: unknown = { ok: true, order_id: orderA.public_id, case: approved }
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(response))))
  for (const c of [{ ...approved, public_id: saleOtherID }, { ...approved, amount_cents: 124 }, nextSale(saleInitial, 'reject', 'rejected', intent.message), nextSale(saleInitial, 'approve', 'approved', 'different message')]) {
    response = { ok: true, order_id: orderA.public_id, case: c }
    await expect(afterSaleAPI.review(orderA.public_id, intent)).rejects.toThrow('原操作不一致')
  }
  response = { ok: false, order_id: orderA.public_id, case: approved }
  await expect(afterSaleAPI.review(orderA.public_id, intent)).rejects.toThrow('未确认')
  response = { ok: true, order_id: orderA.public_id, case: nextSale(approved, 'complete', 'completed', '', { refund_request_id: saleRefundID }) }
  expect((await afterSaleAPI.review(orderA.public_id, intent)).state).toBe('completed')
})
it('审核状态与实际退款独立；成功退款候选需金额/时间/原付款及对账匹配且未复用', () => {
  const c = nextSale(saleInitial, 'approve', 'approved', 'Approved')
  const refund = { ...refundRequest, public_id: saleRefundID, created_at: saleTime, state: 'succeeded' as const }
  const paid = { ...orderA, receipts: [{ ...orderA.receipts[0]!, refunded_cents: 123 }], refunded_cents: 123, refund_requests: [refund] }
  const history = parsedSaleFixture([c])
  expect(matchingRefunds(paid, c, history)).toHaveLength(1)
  for (const invalid of [{ ...refund, state: 'pending' as const }, { ...refund, amount_cents: 122 }, { ...refund, currency: 'EUR' }, { ...refund, created_at: '2000-01-01' }, { ...refund, payment_ref: 'other-payment' }]) expect(matchingRefunds({ ...paid, refund_requests: [invalid] }, c, history)).toHaveLength(0)
  expect(matchingRefunds({ ...paid, receipts: [{ ...paid.receipts[0]!, refund_reconciliation_required: true }] }, c, history)).toHaveLength(0)
  const other = nextSale({ ...c, public_id: saleOtherID }, 'complete', 'completed', '', { refund_request_id: saleRefundID })
  expect(matchingRefunds(paid, c, parsedSaleFixture([c, other]))).toHaveLength(0)
  expect(allowedAfterSaleActions(saleInitial)).toEqual(['approve', 'reject'])
  expect(allowedAfterSaleActions({ ...c, kind: 'return_refund', state: 'awaiting_return' })).toEqual(['receive_return'])
  expect(allowedAfterSaleActions(c)).toEqual(['complete'])
  expect(allowedAfterSaleActions(other as AfterSaleCase)).toEqual([])
})
