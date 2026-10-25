// 订单资金与权限契约测试 / Order Financial and Privacy Contract Tests
// 功能：验证安全整数资金、隐私先丢弃、退款快照/响应、履约屏障和原生导出范围
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:46:12

import { expect, it, vi } from 'vitest'
import { orderAPI, orderExportURL, parseOrder, parseOrders, safeTrackingURL } from './order-api'
import { canShip, money, receiptKey, refundableReceipts, refundDraft } from './order-model'
import { orderA, refundRequest, refundResult, orderSummary } from './order-fixtures'
function response(value: unknown, status = 200) { return new Response(JSON.stringify(value), { status }) }
it('订单和退款请求默认拒绝重复标识、未知状态、不安全金额和错误合计', () => {
  expect(() => parseOrders({ orders: [orderSummary(orderA), orderSummary(orderA)] }, true)).toThrow()
  expect(() => parseOrder({ ...orderA, payment_status: 'unknown' }, true)).toThrow()
  expect(() => parseOrder({ ...orderA, total_cents: Number.MAX_SAFE_INTEGER + 1 }, true)).toThrow()
  expect(() => parseOrder({ ...orderA, paid_cents: 1201 }, true)).toThrow('资金合计')
  expect(() => parseOrder({ ...orderA, receipts: [{ ...orderA.receipts[0], currency: 'EUR' }] }, true)).toThrow('币种与订单不一致')
  expect(() => parseOrder({ ...orderA, refund_requests: [refundRequest, refundRequest] }, true)).toThrow()
  expect(() => parseOrder({ ...orderA, receipts: [{ ...orderA.receipts[0], review_reason: 'unknown' }] }, true)).toThrow()
})
it('无客户资料权限先丢弃全部私密字段，即使格式异常也不保留或解析其内容', () => {
  const raw = { ...orderA, email: { secret: 'PrivateEmail' }, ship_name: { secret: 'PrivateName' }, ship_phone: 'PrivatePhone', ship_address: 'PrivateAddress', ship_country: 'PrivateCountry', tracking_number: 'PrivateTracking', tracking_url: 'https://private.invalid',
    receipts: [{ ...orderA.receipts[0], reference: { secret: 'PrivatePayment' } }], refunds: [{ provider: 'stripe', provider_refund_id: 'PrivateRefund', amount_cents: 1, created_at: '2026-10-25', observed_state: '' }], refund_requests: [{ ...refundRequest, payment_ref: 'PrivatePayment', gateway_ref: 'PrivateGateway' }] }
  const parsed = parseOrder(raw, false)
  expect(JSON.stringify(parsed)).not.toMatch(/Private|private\.invalid/); expect(parsed.email).toBe('已隐藏'); expect(parsed.tracking_url).toBe(''); expect(parsed.receipts[0]?.reference).toBe('已隐藏')
  expect(parseOrders({ orders: [{ ...orderSummary(orderA), email: { secret: 'PrivateEmail' } }] }, false)[0]?.email).toBe('已隐藏')
  expect(() => parseOrder(raw, true)).toThrow()
})
it('脱敏后的同通道多笔付款不会因相同隐藏参考号而丢失；有权限时拒绝重复真实付款', () => {
  const two = { ...orderA, paid_cents: 2400, receipts: [orderA.receipts[0], orderA.receipts[0]] }
  expect(parseOrder(two, false).receipts).toHaveLength(2); expect(() => parseOrder(two, true)).toThrow('重复付款')
})
it('发货分别检查资金、履约、核对与所有未决退款，不仅依赖订单 paid 状态', () => {
  expect(canShip(orderA)).toBe(true)
  for (const order of [{ ...orderA, payment_review: true }, { ...orderA, payment_status: 'unrecorded' as const }, { ...orderA, fulfillment_status: 'fulfilled' as const }, { ...orderA, refund_requests: [refundRequest] }, { ...orderA, refund_requests: [{ ...refundRequest, state: 'reconciliation' as const }] }, { ...orderA, receipts: [{ ...orderA.receipts[0]!, refund_reconciliation_required: true }] }]) expect(canShip(order)).toBe(false)
})
it('付款退款排除历史缺证据、核对屏障、全退和当前未决请求，但允许指定重复或迟到付款', () => {
  const receipt = orderA.receipts[0]!
  expect(refundableReceipts({ ...orderA, receipts: [{ ...receipt, review_reason: 'duplicate_payment' }] })).toHaveLength(1)
  expect(refundableReceipts({ ...orderA, receipts: [{ ...receipt, review_reason: 'late_payment' }] })).toHaveLength(1)
  for (const order of [{ ...orderA, receipts: [{ ...receipt, review_reason: 'legacy_refund' as const }] }, { ...orderA, receipts: [{ ...receipt, refunded_cents: 1200 }] }, { ...orderA, receipts: [{ ...receipt, refund_reconciliation_required: true }] }, { ...orderA, refund_requests: [refundRequest] }]) expect(refundableReceipts(order)).toHaveLength(0)
})
it('退款快照精确到分，固定指定付款及已退基线，拒绝 0/舍入/溢出/超余额', () => {
  const key = receiptKey(orderA.receipts[0]!)
  expect(refundDraft(orderA, key, '1.23')).toEqual({ provider: 'stripe', payment_ref: 'fixture-payment-a', currency: 'USD', amount_cents: 123, expected_refunded_cents: 0 })
  expect(money(Number.MAX_SAFE_INTEGER, 'USD')).toBe('90071992547409.91 USD')
  for (const value of ['', '0', '-1', '0.001', '1e2', '90071992547409.92', '12.01']) expect(() => refundDraft(orderA, key, value)).toThrow()
})
it('追踪链接拒绝脚本、凭证与反斜线，展示时忽略旧库非法链接', () => {
  for (const value of ['javascript:bad()', 'http://outside.invalid', '//outside.invalid', 'https://name:password@outside.invalid', 'https://outside.invalid/\\x']) expect(safeTrackingURL(value)).toBe(false)
  expect(safeTrackingURL('https://tracking.example.invalid/123')).toBe(true)
  expect(parseOrder({ ...orderA, tracking_url: 'javascript:bad()' }, true).tracking_url).toBe('')
})
it('导出 URL 只含合法状态和 UTC 日期，包含结束当天，不允许反向或无效日期', () => {
  expect(orderExportURL({ status: 'paid', from: '2026-10-01', to: '2026-10-01' })).toBe('/admin/api/orders/export?status=paid&from=2026-10-01&to=2026-10-01')
  expect(orderExportURL({ status: '', from: '', to: '' })).toBe('/admin/api/orders/export')
  for (const [from, to] of [['2026-10-02', '2026-10-01'], ['2026-02-30', ''], ['2026/10/01', '']]) expect(() => orderExportURL({ status: '', from: from!, to: to! })).toThrow()
})
it('退款请求只发送六项服务端字段，相同输入使用同一键，验证响应付款/金额/币种', async () => {
  const fetch = vi.fn(async (_url: string, _options: RequestInit) => response(refundResult, 202)); vi.stubGlobal('fetch', fetch)
  const input = { ...refundDraft(orderA, receiptKey(orderA.receipts[0]!), '1.23'), request_key: 'fixture-refund-idempotency-key' }
  await orderAPI.refund(orderA.public_id, input); await orderAPI.refund(orderA.public_id, input)
  expect(fetch.mock.calls[0]![1].body).toBe(fetch.mock.calls[1]![1].body)
  expect(JSON.parse(fetch.mock.calls[0]![1].body as string)).toEqual({ request_key: input.request_key, provider: 'stripe', payment_ref: 'fixture-payment-a', amount_cents: 123, expected_refunded_cents: 0 })
  await expect(orderAPI.refund(orderA.public_id, { ...input, amount_cents: 456 })).rejects.toThrow('响应与原请求不一致')
})
it('退款请求核对验证对象和原金额，平台清单核对不会调用新退款接口', async () => {
  const fetch = vi.fn(async (url: string, _options: RequestInit) => response(url.endsWith('/refund-check') ? { public_id: 'refund-check-fixture', refunded_cents: 123, pending: true } : refundResult)); vi.stubGlobal('fetch', fetch)
  await orderAPI.refreshRefund(orderA.public_id, refundRequest, 'fixture-gateway-ref')
  expect(fetch.mock.calls[0]![0]).toMatch(/\/refunds\/refund-fixture-a\/refresh$/)
  expect(JSON.parse(fetch.mock.calls[0]![1].body as string)).toEqual({ gateway_ref: 'fixture-gateway-ref' })
  expect((await orderAPI.reconcile(orderA.public_id, orderA.receipts[0]!)).pending).toBe(true)
  expect(fetch.mock.calls[1]![0]).toMatch(/\/refund-check$/)
  await expect(orderAPI.refreshRefund(orderA.public_id, { ...refundRequest, amount_cents: 456 }, '')).rejects.toThrow('所选请求不一致')
})
it('发货/撤销只写指定对象白名单，发货非法内容在发送前拒绝', async () => {
  const fetch = vi.fn(async (_url: string, _options: RequestInit) => response({ ok: true, status: 'fulfilled' })); vi.stubGlobal('fetch', fetch)
  await orderAPI.fulfill(orderA.public_id, { carrier: ' Fixture ', tracking_number: ' 123 ', tracking_url: '' })
  expect(JSON.parse(fetch.mock.calls[0]![1].body as string)).toEqual({ carrier: 'Fixture', tracking_number: '123', tracking_url: '' })
  await expect(orderAPI.fulfill(orderA.public_id, { carrier: '', tracking_number: '123', tracking_url: '' })).rejects.toThrow()
  await orderAPI.revoke(orderA.public_id); expect(JSON.parse(fetch.mock.calls[1]![1].body as string)).toEqual({}); expect(fetch).toHaveBeenCalledTimes(2)
})
