// 售后隔离样本 / Isolated After Sale Fixtures
// 功能：固定虚构UUIDv7、完整事件和空历史，供接口与真实页面回归；不包含经营资料
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 22:45:01

import type { Order } from './order-api'
import type { AfterSaleCase, AfterSaleEvent, AfterSaleHistory } from './after-sale-api'
import { orderA } from './order-fixtures'
export const saleCaseID = '00000000-0000-7000-8000-000000000101'
export const saleKey = '00000000-0000-7000-8000-000000000102'
export const saleRefundID = '00000000-0000-7000-8000-000000000103'
export const saleOtherID = '00000000-0000-7000-8000-000000000104'
export const saleTime = '2026-10-25T12:00:00.000Z'
export const saleInitial: AfterSaleCase = {
  public_id: saleCaseID, kind: 'refund', state: 'requested', version: 1, amount_cents: 123, currency: 'USD', reason: 'PRIVATE-SALE-REASON: one sample tee does not fit',
  terms: { configured: false, return_window_days: 0, return_shipping_payer: '', support_email: '' }, merchant_message: '', return_instructions: '', return_carrier: '', return_tracking_number: '', refund_request_id: '', refund_needs_review: false,
  created_at: saleTime, updated_at: saleTime, events: [{ version: 1, actor: 'customer', action: 'request', state: 'requested', message: 'PRIVATE-SALE-REASON: one sample tee does not fit', carrier: '', tracking_number: '', created_at: saleTime }],
}
export function nextSale(item: AfterSaleCase, action: AfterSaleEvent['action'], state: AfterSaleCase['state'], message: string, changes: Partial<AfterSaleCase> = {}): AfterSaleCase {
  const version = item.version + 1
  return { ...item, ...changes, state, version, merchant_message: message, events: [...item.events, { version, actor: 'merchant', action, state, message, carrier: '', tracking_number: '', created_at: saleTime }] }
}
export function saleFixture(cases: AfterSaleCase[] = [structuredClone(saleInitial)], order: Order = orderA, key = saleKey) {
  const active = cases.some(c => ['requested', 'awaiting_return', 'returned', 'approved'].includes(c.state))
  const max = order.receipts.length ? Math.max(0, Math.min(order.total_cents, order.paid_cents - order.refunded_cents)) : ['paid', 'fulfilled'].includes(order.status) ? order.total_cents : 0
  return { order_id: order.public_id, next_action_key: key, snapshot: { cases, max_request_cents: max, can_request_refund: !active && max > 0, can_request_return: !active && max > 0 && order.fulfillment_status === 'fulfilled' } }
}
export function parsedSaleFixture(cases: AfterSaleCase[] = [structuredClone(saleInitial)], order: Order = orderA): AfterSaleHistory {
  const raw = saleFixture(cases, order)
  return { order_id: raw.order_id, next_action_key: raw.next_action_key, ...raw.snapshot }
}
