// 售后审核接口 / After Sale Review API
// 功能：严格有界私密契约、订单与币种绑定、服务端原键及版本快照，审核不调用资金接口
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 22:31:16

import { bool, choice, integer, record, string } from './api-values'
import { orderID, type Order, type RefundRequest } from './order-api'
import { request } from './transport'

export const afterSaleStates = { requested: '待审核', awaiting_return: '已同意退货，待收到包裹', returned: '已收到退货，待检查审核', approved: '退款已获批准，待实际处理', completed: '已关联成功退款办结', rejected: '已拒绝', withdrawn: '顾客已撤回' } as const
export const afterSaleActions = { request: '提交申请', approve: '审核通过', reject: '拒绝申请', receive_return: '确认收到退货', complete: '关联成功退款办结', withdraw: '撤回申请', return_tracking: '提供退回运单' } as const
export type AfterSaleState = keyof typeof afterSaleStates
export type AfterSaleAction = 'approve' | 'reject' | 'receive_return' | 'complete'
export interface AfterSaleTerms { configured: boolean; return_window_days: number; return_shipping_payer: '' | 'buyer' | 'merchant'; support_email: string }
export interface AfterSaleEvent { version: number; actor: 'customer' | 'merchant'; action: keyof typeof afterSaleActions; state: AfterSaleState; message: string; carrier: string; tracking_number: string; created_at: string }
export interface AfterSaleCase {
  public_id: string; kind: 'refund' | 'return_refund'; state: AfterSaleState; version: number; amount_cents: number; currency: string; reason: string
  terms: AfterSaleTerms; merchant_message: string; return_instructions: string; return_carrier: string; return_tracking_number: string
  refund_request_id: string; refund_needs_review: boolean; created_at: string; updated_at: string; events: AfterSaleEvent[]
}
export interface AfterSaleHistory { order_id: string; next_action_key: string; cases: AfterSaleCase[]; max_request_cents: number; can_request_refund: boolean; can_request_return: boolean }
export interface AfterSaleIntent { case_id: string; request_key: string; expected_version: number; action: AfterSaleAction; message: string; refund_request_id: string; amount_cents: number; currency: string }
export interface AfterSaleEditor { case: AfterSaleCase; intent: AfterSaleIntent; open: boolean; receipt_confirmed: boolean }
const uuid7 = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
function id(value: string) { if (!uuid7.test(value)) throw new Error('售后或请求标识无效，请重新读取记录'); return value }
function text(data: Record<string, unknown>, key: string, max: number, required = false) {
  const value = string(data, key)
  if (new TextEncoder().encode(value).length > max || required && !value.trim() || /[\u0000-\u0008\u000b-\u001f\u007f-\u009f\u2028\u2029]/.test(value)) throw new Error('售后说明格式无效，请核对服务端记录')
  return value
}
function date(data: Record<string, unknown>, key: string) { const value = string(data, key); if (!Number.isFinite(Date.parse(value))) throw new Error('售后时间格式无效'); return value }
function list(value: unknown, max: number): unknown[] { if (!Array.isArray(value) || value.length > max) throw new Error('售后记录数量或格式无效'); return value }
export function parseAfterSaleCase(value: unknown, currency: string): AfterSaleCase {
  const row = record(value), rawTerms = record(row.terms), version = integer(row, 'version', 1, 50)
  if (string(row, 'currency') !== currency) throw new Error('售后币种与订单不一致')
  const terms: AfterSaleTerms = { configured: bool(rawTerms, 'configured'), return_window_days: integer(rawTerms, 'return_window_days', 0, 365), return_shipping_payer: choice(rawTerms, 'return_shipping_payer', ['', 'buyer', 'merchant']), support_email: text(rawTerms, 'support_email', 254) }
  if (terms.configured ? terms.return_window_days < 1 || !terms.return_shipping_payer || !terms.support_email : !!(terms.return_window_days || terms.return_shipping_payer || terms.support_email)) throw new Error('售后政策快照不一致')
  const events = list(row.events, 50).map((value, index) => {
    const e = record(value), number = integer(e, 'version', 1, 50)
    if (number !== index + 1) throw new Error('售后历史版本不连续')
    return { version: number, actor: choice(e, 'actor', ['customer', 'merchant']), action: choice(e, 'action', Object.keys(afterSaleActions) as (keyof typeof afterSaleActions)[]), state: choice(e, 'state', Object.keys(afterSaleStates) as AfterSaleState[]), message: text(e, 'message', 4000), carrier: text(e, 'carrier', 128), tracking_number: text(e, 'tracking_number', 128), created_at: date(e, 'created_at') }
  })
  const result: AfterSaleCase = { public_id: id(string(row, 'public_id')), kind: choice(row, 'kind', ['refund', 'return_refund']), state: choice(row, 'state', Object.keys(afterSaleStates) as AfterSaleState[]), version, amount_cents: integer(row, 'amount_cents', 1, Number.MAX_SAFE_INTEGER), currency, reason: text(row, 'reason', 2000, true), terms,
    merchant_message: text(row, 'merchant_message', 4000), return_instructions: text(row, 'return_instructions', 4000), return_carrier: text(row, 'return_carrier', 128), return_tracking_number: text(row, 'return_tracking_number', 128),
    refund_request_id: row.refund_request_id === undefined ? '' : string(row, 'refund_request_id'), refund_needs_review: bool(row, 'refund_needs_review'), created_at: date(row, 'created_at'), updated_at: date(row, 'updated_at'), events }
  if (events.length !== version || events[0]?.action !== 'request' || events[0]?.actor !== 'customer' || events[0]?.state !== 'requested' || events.at(-1)?.state !== result.state) throw new Error('售后当前状态与历史不一致')
  if (result.kind === 'refund' && ['awaiting_return', 'returned'].includes(result.state) || (result.state === 'completed') !== !!result.refund_request_id || result.refund_needs_review && result.state !== 'completed') throw new Error('售后类型或退款关联不一致')
  if (result.refund_request_id) id(result.refund_request_id)
  return result
}
export function parseAfterSales(value: unknown, order: string, currency: string, customer: boolean): AfterSaleHistory {
  if (!customer) throw new Error('查看售后需要客户信息权限') // Reject before touching private fields.
  if (!/^[A-Z]{3}$/.test(currency)) throw new Error('订单币种无效')
  const data = record(value), snapshot = record(data.snapshot)
  if (string(data, 'order_id') !== order) throw new Error('售后记录与所选订单不一致')
  const cases = list(snapshot.cases, 20).map(value => parseAfterSaleCase(value, currency))
  const refunds = cases.filter(c => c.refund_request_id).map(c => c.refund_request_id)
  if (new Set(cases.map(c => c.public_id)).size !== cases.length || new Set(refunds).size !== refunds.length || cases.filter(activeAfterSale).length > 1) throw new Error('售后记录包含重复或冲突申请')
  const result = { order_id: order, next_action_key: id(string(data, 'next_action_key')), cases, max_request_cents: integer(snapshot, 'max_request_cents', 0, Number.MAX_SAFE_INTEGER), can_request_refund: bool(snapshot, 'can_request_refund'), can_request_return: bool(snapshot, 'can_request_return') }
  if (result.can_request_return && !result.can_request_refund || result.can_request_refund && (!result.max_request_cents || cases.some(activeAfterSale) || cases.length >= 20)) throw new Error('售后可申请状态不一致')
  return result
}
export function activeAfterSale(c: AfterSaleCase) { return ['requested', 'awaiting_return', 'returned', 'approved'].includes(c.state) }
export function allowedAfterSaleActions(c: AfterSaleCase): AfterSaleAction[] {
  if (c.state === 'requested' || c.kind === 'return_refund' && c.state === 'returned') return ['approve', 'reject']
  if (c.kind === 'return_refund' && c.state === 'awaiting_return') return ['receive_return']
  return c.state === 'approved' ? ['complete'] : []
}
export function matchingRefunds(order: Order, c: AfterSaleCase, history: AfterSaleHistory): RefundRequest[] {
  const used = new Set(history.cases.filter(item => item.public_id !== c.public_id).map(item => item.refund_request_id))
  return order.refund_requests.filter(r => uuid7.test(r.public_id) && r.state === 'succeeded' && r.amount_cents === c.amount_cents && r.currency === c.currency && !used.has(r.public_id) && Date.parse(r.created_at) >= Date.parse(c.created_at) && order.receipts.some(p => p.provider === r.provider && p.reference === r.payment_ref && p.currency === r.currency && !p.refund_reconciliation_required && p.refunded_cents >= r.amount_cents))
}
export function prepareAfterSale(input: AfterSaleIntent): AfterSaleIntent {
  const message = input.message.replaceAll('\r\n', '\n').trim()
  if (!['approve', 'reject', 'receive_return', 'complete'].includes(input.action) || !Number.isSafeInteger(input.expected_version) || input.expected_version < 1 || input.expected_version >= 50 || !Number.isSafeInteger(input.amount_cents) || input.amount_cents <= 0 || !/^[A-Z]{3}$/.test(input.currency)) throw new Error('售后操作或版本无效，请重新核对')
  text({ message }, 'message', 4000, input.action !== 'complete')
  if (input.action === 'complete') id(input.refund_request_id)
  else if (input.refund_request_id) throw new Error('审核不能同时关联退款')
  return { case_id: id(input.case_id), request_key: id(input.request_key), expected_version: input.expected_version, action: input.action, message, refund_request_id: input.refund_request_id, amount_cents: input.amount_cents, currency: input.currency }
}
export const afterSaleAPI = {
  get: async (order: string, currency: string, customer: boolean, signal?: AbortSignal) => {
    if (!customer) throw new Error('查看售后需要客户信息权限')
    return parseAfterSales(await request('GET', `/orders/${orderID(order)}/after-sales`, undefined, signal), order, currency, customer)
  },
  review: async (order: string, input: AfterSaleIntent): Promise<AfterSaleCase> => {
    const p = prepareAfterSale(input)
    const data = record(await request('POST', `/orders/${orderID(order)}/after-sales/${p.case_id}/actions`, { request_key: p.request_key, expected_version: p.expected_version, action: p.action, message: p.message, refund_request_id: p.refund_request_id }))
    if (!bool(data, 'ok') || string(data, 'order_id') !== order) throw new Error('售后审核结果未确认，请继续原请求')
    const c = parseAfterSaleCase(data.case, p.currency), event = c.events[p.expected_version]
    if (c.public_id !== p.case_id || c.amount_cents !== p.amount_cents || !event || event.actor !== 'merchant' || event.action !== p.action || event.message !== p.message || p.action === 'complete' && c.refund_request_id !== p.refund_request_id) throw new Error('售后响应与原操作不一致，请继续原请求')
    return c
  },
}
