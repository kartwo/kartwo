// 订单操作模型 / Order Operation Model
// 功能：分离资金和履约门槛，按付款生成退款快照，精确金额及状态文案
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:46:12

import { refundCents } from '../refund-amount.js'
import { centsToMoney } from './money'
import type { Order, Receipt, RefundDraft } from './order-api'
export const receiptKey = (receipt: Receipt) => JSON.stringify([receipt.provider, receipt.reference])
export const activeRefund = (state: string) => ['prepared', 'pending', 'review', 'reconciliation'].includes(state)
export function canShip(order: Order) {
  return order.status === 'paid' && order.fulfillment_status === 'unfulfilled' && !order.payment_review && ['paid', 'partially_refunded'].includes(order.payment_status) && order.paid_cents > order.refunded_cents && !order.receipts.some(receipt => receipt.refund_reconciliation_required) && !order.refund_requests.some(request => activeRefund(request.state))
}
export function refundableReceipts(order: Order) {
  return order.receipts.filter(receipt => receipt.refunded_cents < receipt.amount_cents && receipt.review_reason !== 'legacy_refund' && !receipt.refund_reconciliation_required && !order.refund_requests.some(request => request.provider === receipt.provider && request.payment_ref === receipt.reference && activeRefund(request.state)))
}
export function refundDraft(order: Order, key: string, amount: string): RefundDraft {
  const receipt = refundableReceipts(order).find(item => receiptKey(item) === key), cents = refundCents(amount)
  if (!receipt || cents === null || cents > receipt.amount_cents - receipt.refunded_cents) throw new Error('请选择付款，并填写不超过可退余额的金额（最多两位小数，须大于 0）')
  return { provider: receipt.provider, payment_ref: receipt.reference, currency: receipt.currency, amount_cents: cents, expected_refunded_cents: receipt.refunded_cents }
}
export const money = (value: number, currency: string) => `${centsToMoney(value)} ${currency}`
export const reviewText = (reason: Receipt['review_reason']) => ({ '': '正常收款', late_payment: '超时到账', duplicate_payment: '重复付款', legacy_refund: '历史退款待核对' })[reason]
export const paymentTexts = { unrecorded: '尚无可核对收款记录', unpaid: '未付款', paid: '已收款', partially_refunded: '部分退款', refunded: '已全额退款', review: '待核对' }
export const refundReason = (reason: string) => ({ refund_status_changed: '平台退款状态已变化，需核对全部退款记录后确认金额。', credentials_locked: '收款凭证未解锁，解锁后将自动继续查询。', configuration_changed: '收款配置已变更，请恢复原配置后继续核对。', gateway_unconfirmed: '尚未确认网关结果，请继续原请求。', gateway_pending: '支付平台正在处理。', unknown_result: '请在支付平台找到这笔退款的编号，填写后核对。', result_mismatch: '返回信息不一致，请核对支付平台退款记录。', rejected: '支付平台拒绝了退款参数，请核对可退余额。', gateway_failed: '支付平台确认本次退款未成功。' } as Record<string, string>)[reason] ?? (reason ? '请核对支付平台处理结果。' : '')
