// 订单与退款接口 / Orders and Refunds API
// 功能：严格解析资金与履约状态，按权限先丢弃私密字段，保留原退款键与精确金额
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:46:12

import { record, string, bool, choice, integer } from './api-values'
import { request } from './transport'
export const orderStatuses = { pending: '待付款', paid: '已付款', fulfilled: '已发货', refunded: '已退款', cancelled: '已取消', payment_review: '收款异常待处理' } as const
export type OrderStatus = keyof typeof orderStatuses
export const refundStates = { prepared: '待提交', pending: '处理中', succeeded: '已成功', failed: '未成功', review: '待核对', reconciliation: '结果变更待对账' } as const
export type RefundState = keyof typeof refundStates
export type Provider = 'stripe' | 'paypal'
export interface OrderSummary { public_id: string; status: OrderStatus; email: string; currency: string; total_cents: number; payment_provider: string; created_at: string }
export interface Receipt { provider: Provider; reference: string; currency: string; amount_cents: number; refunded_cents: number; review_reason: '' | 'late_payment' | 'duplicate_payment' | 'legacy_refund'; source: 'verified' | 'legacy'; created_at: string; refund_reconciliation_required: boolean; refund_checked_at: string }
export interface RefundRequest { public_id: string; provider: Provider; payment_ref: string; gateway_ref: string; currency: string; amount_cents: number; state: RefundState; reason: string; created_at: string }
export interface Order extends OrderSummary {
  ship_name: string; ship_phone: string; ship_address: string; ship_country: string; subtotal_cents: number; shipping_cents: number; shipping_rule_name: string
  tracking_carrier: string; tracking_number: string; tracking_url: string; payment_status: 'unrecorded' | 'unpaid' | 'paid' | 'partially_refunded' | 'refunded' | 'review'; fulfillment_status: 'unfulfilled' | 'fulfilled'
  paid_cents: number; refunded_cents: number; payment_review: boolean; receipts: Receipt[]; refund_requests: RefundRequest[]
  lines: { title: string; spec: string; sku: string; unit_cents: number; quantity: number; line_cents: number }[]
  refunds: { provider: Provider; provider_refund_id: string; amount_cents: number; created_at: string; observed_state: '' | 'pending' | 'succeeded' | 'failed' }[]
}
export interface RefundDraft { provider: Provider; payment_ref: string; amount_cents: number; expected_refunded_cents: number; currency: string }
export interface RefundInput extends RefundDraft { request_key: string }
export interface RefundResult { public_id: string; state: Exclude<RefundState, 'reconciliation'>; provider: Provider; amount_cents: number; currency: string; reason: string }
export interface ShipmentInput { carrier: string; tracking_number: string; tracking_url: string }
export function orderID(value: string) { if (!/^[a-zA-Z0-9-]{8,}$/.test(value)) throw new Error('订单或退款请求标识无效'); return encodeURIComponent(value) }
function publicID(data: Record<string, unknown>) { const value = string(data, 'public_id'); orderID(value); return value }
function rows(value: unknown): unknown[] { if (!Array.isArray(value)) throw new Error('订单记录格式不正确'); return value }
function cents(data: Record<string, unknown>, key: string) { return integer(data, key, 0, Number.MAX_SAFE_INTEGER) }
function currency(data: Record<string, unknown>) { const value = string(data, 'currency'); if (!/^[a-zA-Z]{3}$/.test(value)) throw new Error('订单币种无效'); return value.toUpperCase() }
function provider(data: Record<string, unknown>) { return choice(data, 'provider', ['stripe', 'paypal']) }
function unique<T extends { public_id: string }>(items: T[]) { if (new Set(items.map(item => item.public_id)).size !== items.length) throw new Error('订单或退款请求标识重复'); return items }
function summary(value: unknown, customer: boolean): OrderSummary {
  const data = record(value)
  return { public_id: publicID(data), status: choice(data, 'status', Object.keys(orderStatuses) as OrderStatus[]), email: customer ? string(data, 'email') : '已隐藏', currency: currency(data), total_cents: cents(data, 'total_cents'), payment_provider: string(data, 'payment_provider'), created_at: string(data, 'created_at') }
}
export function parseOrders(value: unknown, customer: boolean) { return unique(rows(record(value).orders).map(item => summary(item, customer))) }
export function safeTrackingURL(value: string): boolean {
  if (!value) return true
  if (/[\s<>"'\\]/.test(value)) return false
  try { const url = new URL(value); return url.protocol === 'https:' && !!url.hostname && !url.username && !url.password } catch { return false }
}
export function parseOrder(value: unknown, customer: boolean): Order {
  const data = record(value), base = summary(data, customer)
  const receipts = rows(data.receipts).map(value => {
    const row = record(value), amount = cents(row, 'amount_cents'), refunded = cents(row, 'refunded_cents')
    if (refunded > amount) throw new Error('收款已退金额超出原金额')
    return { provider: provider(row), reference: customer ? string(row, 'reference') : '已隐藏', currency: currency(row), amount_cents: amount, refunded_cents: refunded,
      review_reason: choice(row, 'review_reason', ['', 'late_payment', 'duplicate_payment', 'legacy_refund']), source: choice(row, 'source', ['verified', 'legacy']), created_at: string(row, 'created_at'), refund_reconciliation_required: bool(row, 'refund_reconciliation_required'), refund_checked_at: string(row, 'refund_checked_at') }
  })
  if (customer && new Set(receipts.map(row => JSON.stringify([row.provider, row.reference]))).size !== receipts.length) throw new Error('收款记录包含重复付款')
  const requests = unique(rows(data.refund_requests).map(value => { const row = record(value); return { public_id: publicID(row), provider: provider(row), payment_ref: customer ? string(row, 'payment_ref') : '已隐藏', gateway_ref: customer ? string(row, 'gateway_ref') : '已隐藏',
    currency: currency(row), amount_cents: integer(row, 'amount_cents', 1, Number.MAX_SAFE_INTEGER), state: choice(row, 'state', Object.keys(refundStates) as RefundState[]), reason: string(row, 'reason'), created_at: string(row, 'created_at') } }))
  if ([...receipts, ...requests].some(row => row.currency !== base.currency)) throw new Error('收款或退款币种与订单不一致')
  const paid = cents(data, 'paid_cents'), refunded = cents(data, 'refunded_cents')
  if (receipts.reduce((sum, row) => sum + BigInt(row.amount_cents), 0n) !== BigInt(paid) || receipts.reduce((sum, row) => sum + BigInt(row.refunded_cents), 0n) !== BigInt(refunded)) throw new Error('订单资金合计与收款记录不一致，请核对服务端记录')
  const tracking = customer ? string(data, 'tracking_url') : ''
  return { ...base, ship_name: customer ? string(data, 'ship_name') : '已隐藏', ship_phone: customer ? string(data, 'ship_phone') : '已隐藏', ship_address: customer ? string(data, 'ship_address') : '已隐藏', ship_country: customer ? string(data, 'ship_country') : '已隐藏',
    subtotal_cents: cents(data, 'subtotal_cents'), shipping_cents: cents(data, 'shipping_cents'), shipping_rule_name: string(data, 'shipping_rule_name'), tracking_carrier: string(data, 'tracking_carrier'), tracking_number: customer ? string(data, 'tracking_number') : '已隐藏',
    tracking_url: safeTrackingURL(tracking) ? tracking : '', payment_status: choice(data, 'payment_status', ['unrecorded', 'unpaid', 'paid', 'partially_refunded', 'refunded', 'review']), fulfillment_status: choice(data, 'fulfillment_status', ['unfulfilled', 'fulfilled']), paid_cents: paid, refunded_cents: refunded, payment_review: bool(data, 'payment_review'), receipts, refund_requests: requests,
    lines: rows(data.lines).map(value => { const row = record(value); return { title: string(row, 'title'), spec: string(row, 'spec'), sku: string(row, 'sku'), unit_cents: cents(row, 'unit_cents'), quantity: integer(row, 'quantity', 1, Number.MAX_SAFE_INTEGER), line_cents: cents(row, 'line_cents') } }),
    refunds: rows(data.refunds).map(value => { const row = record(value); return { provider: provider(row), provider_refund_id: customer ? string(row, 'provider_refund_id') : '已隐藏', amount_cents: cents(row, 'amount_cents'), created_at: string(row, 'created_at'), observed_state: choice(row, 'observed_state', ['', 'pending', 'succeeded', 'failed']) } }),
  }
}
export function parseRefundResult(value: unknown): RefundResult {
  const data = record(value)
  return { public_id: publicID(data), provider: provider(data), state: choice(data, 'state', ['prepared', 'pending', 'succeeded', 'failed', 'review']), amount_cents: integer(data, 'amount_cents', 1, Number.MAX_SAFE_INTEGER), currency: currency(data), reason: string(data, 'reason') }
}
function accepted(value: unknown) { if (!bool(record(value), 'ok')) throw new Error('服务器未确认操作成功，请核对订单') }
export function shipmentPayload(input: ShipmentInput) {
  const carrier = input.carrier.trim(), tracking_number = input.tracking_number.trim(), tracking_url = input.tracking_url.trim()
  if (!carrier || !tracking_number || new TextEncoder().encode(tracking_number).length > 128 || !safeTrackingURL(tracking_url)) throw new Error('请填写承运商和有效运单号，追踪链接须为 HTTPS 地址')
  return { carrier, tracking_number, tracking_url }
}
function refundPayload(input: RefundInput) {
  if (!/^[a-zA-Z0-9_-]{16,128}$/.test(input.request_key) || !input.payment_ref || input.payment_ref.length > 255 || !['stripe', 'paypal'].includes(input.provider) || !Number.isSafeInteger(input.amount_cents) || input.amount_cents <= 0 || !Number.isSafeInteger(input.expected_refunded_cents) || input.expected_refunded_cents < 0) throw new Error('退款参数无效')
  return { request_key: input.request_key, provider: input.provider, payment_ref: input.payment_ref, amount_cents: input.amount_cents, expected_refunded_cents: input.expected_refunded_cents }
}
export const orderAPI = {
  list: async (customer: boolean, signal?: AbortSignal) => parseOrders(await request('GET', '/orders', undefined, signal), customer),
  get: async (id: string, customer: boolean, signal?: AbortSignal) => { const result = parseOrder(await request('GET', `/orders/${orderID(id)}`, undefined, signal), customer); if (result.public_id !== id) throw new Error('订单详情与所选订单不一致'); return result },
  refund: async (id: string, input: RefundInput) => { const result = parseRefundResult(await request('POST', `/orders/${orderID(id)}/refunds`, refundPayload(input))); if (result.provider !== input.provider || result.amount_cents !== input.amount_cents || result.currency !== input.currency) throw new Error('退款响应与原请求不一致，请继续原请求核对'); return result },
  refreshRefund: async (id: string, refund: Pick<RefundRequest, 'public_id' | 'provider' | 'amount_cents' | 'currency'>, gateway_ref: string) => { if (gateway_ref.length > 255) throw new Error('支付平台退款编号过长'); const result = parseRefundResult(await request('POST', `/orders/${orderID(id)}/refunds/${orderID(refund.public_id)}/refresh`, { gateway_ref })); if (result.public_id !== refund.public_id || result.provider !== refund.provider || result.amount_cents !== refund.amount_cents || result.currency !== refund.currency) throw new Error('退款响应与所选请求不一致'); return result },
  reconcile: async (id: string, receipt: Receipt) => { const data = record(await request('POST', `/orders/${orderID(id)}/refund-check`, { provider: receipt.provider, payment_ref: receipt.reference })); const amount = cents(data, 'refunded_cents'); if (amount > receipt.amount_cents) throw new Error('核对金额超出原收款'); return { public_id: publicID(data), refunded_cents: amount, pending: bool(data, 'pending') } },
  revoke: async (id: string) => accepted(await request('POST', `/orders/${orderID(id)}/revoke-access`, {})),
  fulfill: async (id: string, input: ShipmentInput) => {
    const data = record(await request('POST', `/orders/${orderID(id)}/fulfill`, shipmentPayload(input))); accepted(data); if (string(data, 'status') !== 'fulfilled') throw new Error('发货结果尚未确认，请刷新核对')
  },
}
export interface ExportFilter { status: OrderStatus | ''; from: string; to: string }
export function orderExportURL(filter: ExportFilter) {
  if (filter.status && !Object.hasOwn(orderStatuses, filter.status)) throw new Error('订单状态无效')
  for (const value of [filter.from, filter.to]) if (value && (!/^\d{4}-\d{2}-\d{2}$/.test(value) || !Number.isFinite(Date.parse(`${value}T00:00:00Z`)) || new Date(`${value}T00:00:00Z`).toISOString().slice(0, 10) !== value)) throw new Error('请填写有效日期')
  if (filter.from && filter.to && filter.from > filter.to) throw new Error('结束日期不能早于开始日期')
  const query = new URLSearchParams(); if (filter.status) query.set('status', filter.status); if (filter.from) query.set('from', filter.from); if (filter.to) query.set('to', filter.to)
  return '/admin/api/orders/export' + (query.size ? `?${query}` : '')
}
