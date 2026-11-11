// 运单通知测试样本 / Shipment Notice Fixtures
// 功能：隔离测试的版本、投递状态和历史数据，不包含真实客户或投递目标
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 16:25:07

import type { Order } from './order-api'
import type { ShipmentHistory, ShipmentNotice, ShipmentRevision } from './shipment-api'
import { orderA } from './order-fixtures'
export const revisionA: ShipmentRevision = { public_id: '019a0000-0000-7000-8000-000000000001', version: 1, carrier: 'Fixture Carrier', tracking_number: 'PRIVATE-TRACK-1', tracking_url: 'https://carrier.example.invalid/fixture-tracking', legacy: false, created_at: '2026-10-20T00:00:00.000Z' }
export const noticeA: ShipmentNotice = { public_id: '019a0000-0000-7000-8000-000000000101', revision_id: revisionA.public_id, version: 1, sequence: 1, status: 'failed', attempts: 5, origin: 'shipped', resolution: '', reason: 'delivery_failed', created_at: '2026-10-20T00:00:00.000Z', next_attempt_at: '2026-10-20T00:00:00.000Z', sent_at: '', resolved_at: '' }
export const noticeBID = '019a0000-0000-7000-8000-000000000102'
export const shippedOrder: Order = { ...orderA, status: 'fulfilled', fulfillment_status: 'fulfilled', tracking_carrier: revisionA.carrier, tracking_number: revisionA.tracking_number, tracking_url: revisionA.tracking_url }
export function shipmentFixture(order_id = shippedOrder.public_id): ShipmentHistory { return { order_id, has_shipment: true, current_revision: revisionA.public_id, current_version: 1, shipped_at: revisionA.created_at, revision_count: 1, notice_count: 1, revisions: [{ ...revisionA }], notices: [{ ...noticeA }] } }
export function legacyShipmentFixture(order_id = shippedOrder.public_id): ShipmentHistory { return { order_id, has_shipment: true, current_revision: '', current_version: 0, shipped_at: revisionA.created_at, revision_count: 0, notice_count: 0, revisions: [], notices: [] } }
