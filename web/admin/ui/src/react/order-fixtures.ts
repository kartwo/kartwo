// 订单测试样本 / Order Test Fixtures
// 功能：隔离测试的资金、隐私和履约样本，不含真实订单与客户资料
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:46:12

import type { Order, OrderSummary, RefundRequest, RefundResult } from './order-api'
export const orderA: Order = { public_id: 'order-fixture-a', status: 'paid', email: 'fixture@example.invalid', currency: 'USD', subtotal_cents: 1000, shipping_cents: 200, total_cents: 1200, shipping_rule_name: 'Fixture Shipping',
  payment_provider: 'stripe', created_at: '2026-10-25T00:00:00Z', ship_name: 'Fixture Customer', ship_phone: 'TEST-PHONE', ship_address: 'Fixture Address', ship_country: 'US', tracking_carrier: '', tracking_number: '', tracking_url: '',
  payment_status: 'paid', fulfillment_status: 'unfulfilled', paid_cents: 1200, refunded_cents: 0, payment_review: false,
  receipts: [{ provider: 'stripe', reference: 'fixture-payment-a', currency: 'USD', amount_cents: 1200, refunded_cents: 0, review_reason: '', source: 'verified', created_at: '2026-10-25', refund_reconciliation_required: false, refund_checked_at: '' }],
  refund_requests: [], refunds: [], lines: [{ title: 'Fixture Shirt', spec: 'Size S', sku: 'FIXTURE-S', unit_cents: 1000, quantity: 1, line_cents: 1000 }] }
export const orderB: Order = { ...orderA, public_id: 'order-fixture-b', email: 'other@example.invalid', ship_name: 'Other Fixture Customer' }
export const refundRequest: RefundRequest = { public_id: 'refund-fixture-a', provider: 'stripe', payment_ref: 'fixture-payment-a', gateway_ref: '', currency: 'USD', amount_cents: 123, state: 'review', reason: 'unknown_result', created_at: '2026-10-25' }
export const refundResult: RefundResult = { public_id: 'refund-fixture-a', provider: 'stripe', currency: 'USD', amount_cents: 123, state: 'pending', reason: 'gateway_pending' }
export function orderSummary(order: Order): OrderSummary { return { public_id: order.public_id, status: order.status, email: order.email, currency: order.currency, total_cents: order.total_cents, payment_provider: order.payment_provider, created_at: order.created_at } }
