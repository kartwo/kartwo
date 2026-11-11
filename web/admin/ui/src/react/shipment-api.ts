// 运单通知接口 / Shipment Notice API
// 功能：严格解析有界历史、资料权限和通知状态，保存版本与原请求以安全恢复未知操作
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 16:17:04

import { bool, choice, integer, record, string } from './api-values'
import { orderID, safeTrackingURL, shipmentPayload, type ShipmentInput } from './order-api'
import { request } from './transport'
export const noticeStates = { pending: '等待发送', sending: '正在投递', sent: '邮件服务器已接收', failed: '发送失败', skipped: '未发送', delivery_unknown: '投递结果待核对', superseded: '旧通知已作废' }
export const noticeOrigins = { shipped: '首次发货', tracking_updated: '运单更正', retry: '失败后重试', resend_after_check: '核对后重发' }
export const noticeResolutions = { '': '', received: '商家已核对收到', retried: '已创建重试', not_received: '已核对未收到并重发', superseded: '已由新运单替代' }
export const noticeReasons = { '': '', smtp_unconfigured: '当时未配置邮件服务，通知没有发送。', shipment_superseded: '旧通知已被新的运单信息替代。', delivery_unknown: '无法确认是否已发送，请先核对收件箱或邮件服务商记录。', status_unavailable: '通知状态暂时无法核对，后台将稍后再查。', delivery_failed: '邮件发送失败，请检查邮件设置和服务商记录。' }
export type NoticeAction = 'retry' | 'received' | 'resend_after_check'
export interface ShipmentRevision { public_id: string; version: number; carrier: string; tracking_number: string; tracking_url: string; legacy: boolean; created_at: string }
export interface ShipmentNotice { public_id: string; revision_id: string; version: number; sequence: number; status: keyof typeof noticeStates; attempts: number; origin: keyof typeof noticeOrigins; resolution: keyof typeof noticeResolutions; reason: keyof typeof noticeReasons; created_at: string; next_attempt_at: string; sent_at: string; resolved_at: string }
export interface ShipmentHistory { order_id: string; has_shipment: boolean; current_revision: string; current_version: number; shipped_at: string; revision_count: number; notice_count: number; revisions: ShipmentRevision[]; notices: ShipmentNotice[] }
export interface TrackingUpdate extends ShipmentInput { expected_revision: string }
export interface NoticeActionInput { notice_id: string; request_key: string; action: NoticeAction; confirm_unknown: boolean }
export type ShipmentIntent = { kind: 'fulfill'; fields: ShipmentInput } | { kind: 'update'; fields: TrackingUpdate } | { kind: 'notice'; fields: NoticeActionInput }
const uuid7 = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
function publicID(value: string) { if (!uuid7.test(value)) throw new Error('运单或通知标识无效'); return value }
function rows(value: unknown) { if (!Array.isArray(value)) throw new Error('运单通知历史格式不正确'); return value as unknown[] }
function count(data: Record<string, unknown>, key: string) { return integer(data, key, 0, Number.MAX_SAFE_INTEGER) }
function timestamp(data: Record<string, unknown>, key: string, optional = false) { const value = string(data, key); if ((!optional || value) && !Number.isFinite(Date.parse(value))) throw new Error('通知时间格式不正确'); return value }
export function parseShipmentHistory(value: unknown, id: string, customer: boolean): ShipmentHistory {
  const data = record(value), order = string(data, 'order_id'); if (order !== id) throw new Error('运单通知与所选订单不一致')
  const has_shipment = bool(data, 'has_shipment'), current_version = count(data, 'current_version'), current_revision = string(data, 'current_revision'), revision_count = count(data, 'revision_count'), notice_count = count(data, 'notice_count')
  if (current_revision) publicID(current_revision)
  const revisions = rows(data.revisions).map(value => { const row = record(value), url = customer ? string(row, 'tracking_url') : ''; return { public_id: publicID(string(row, 'public_id')), version: integer(row, 'version', 1, Number.MAX_SAFE_INTEGER), carrier: string(row, 'carrier'), tracking_number: customer ? string(row, 'tracking_number') : '已隐藏', tracking_url: safeTrackingURL(url) ? url : '', legacy: bool(row, 'legacy'), created_at: timestamp(row, 'created_at') } })
  const notices = rows(data.notices).map(value => { const row = record(value); return { public_id: publicID(string(row, 'public_id')), revision_id: publicID(string(row, 'revision_id')), version: integer(row, 'version', 1, Number.MAX_SAFE_INTEGER), sequence: integer(row, 'sequence', 1, Number.MAX_SAFE_INTEGER), status: choice(row, 'status', Object.keys(noticeStates) as (keyof typeof noticeStates)[]), attempts: count(row, 'attempts'), origin: choice(row, 'origin', Object.keys(noticeOrigins) as (keyof typeof noticeOrigins)[]), resolution: choice(row, 'resolution', Object.keys(noticeResolutions) as (keyof typeof noticeResolutions)[]), reason: choice(row, 'reason', Object.keys(noticeReasons) as (keyof typeof noticeReasons)[]), created_at: timestamp(row, 'created_at'), next_attempt_at: timestamp(row, 'next_attempt_at'), sent_at: timestamp(row, 'sent_at', true), resolved_at: timestamp(row, 'resolved_at', true) } })
  if (revisions.length !== Math.min(20, revision_count) || notices.length !== Math.min(50, notice_count) || new Set(revisions.map(row => row.public_id)).size !== revisions.length || new Set(revisions.map(row => row.version)).size !== revisions.length || new Set(notices.map(row => row.public_id)).size !== notices.length || new Set(notices.map(row => `${row.revision_id}:${row.sequence}`)).size !== notices.length) throw new Error('运单通知历史数量或标识不一致')
  if (!has_shipment && (current_version || current_revision || revision_count || notice_count) || has_shipment && (current_version === 0 ? !!current_revision || !!revision_count || !!notice_count : !current_revision || revisions[0]?.public_id !== current_revision || revisions[0]?.version !== current_version)) throw new Error('运单当前版本与历史不一致')
  for (let index = 0; index < revisions.length; index++) if (revisions[index]!.version > current_version || index > 0 && revisions[index]!.version >= revisions[index - 1]!.version) throw new Error('运单版本顺序不一致')
  for (let index = 0; index < notices.length; index++) {
    const row = notices[index]!, revision = revisions.find(item => item.version === row.version), prior = notices[index - 1]
    if (row.version > current_version || revision && revision.public_id !== row.revision_id || prior && (row.version > prior.version || row.version === prior.version && row.sequence >= prior.sequence)) throw new Error('通知与运单版本不一致')
  }
  if (current_version > 0 && notices[0]?.revision_id !== current_revision) throw new Error('当前运单的通知记录不完整，请刷新核对')
  return { order_id: order, has_shipment, current_revision, current_version, shipped_at: timestamp(data, 'shipped_at', !has_shipment), revision_count, notice_count, revisions, notices }
}
export function trackingUpdatePayload(input: TrackingUpdate) { if (typeof input.expected_revision !== 'string') throw new Error('缺少当前运单版本，请刷新核对'); if (input.expected_revision) publicID(input.expected_revision); return { ...shipmentPayload(input), expected_revision: input.expected_revision } }
export function noticeActionPayload(input: NoticeActionInput) {
  publicID(input.notice_id)
  if (!/^[a-zA-Z0-9_-]{16,128}$/.test(input.request_key) || !['retry', 'received', 'resend_after_check'].includes(input.action) || input.action !== 'retry' && input.confirm_unknown !== true) throw new Error('请先核对邮件投递结果再确认操作')
  return { request_key: input.request_key, action: input.action, confirm_unknown: input.confirm_unknown }
}
export function prepareShipmentIntent(intent: ShipmentIntent): ShipmentIntent {
  if (intent.kind === 'fulfill') return { kind: 'fulfill', fields: shipmentPayload(intent.fields) }
  if (intent.kind === 'update') return { kind: 'update', fields: trackingUpdatePayload(intent.fields) }
  return { kind: 'notice', fields: { ...noticeActionPayload(intent.fields), notice_id: intent.fields.notice_id } }
}
export const shipmentAPI = {
  get: async (id: string, customer: boolean, signal?: AbortSignal) => parseShipmentHistory(await request('GET', `/orders/${orderID(id)}/shipment`, undefined, signal), id, customer),
  update: async (id: string, input: TrackingUpdate) => { if (!bool(record(await request('PATCH', `/orders/${orderID(id)}/shipment`, trackingUpdatePayload(input))), 'ok')) throw new Error('运单更正结果尚未确认，请继续原请求核对') },
  manage: async (id: string, input: NoticeActionInput) => {
    const data = record(await request('POST', `/orders/${orderID(id)}/shipment-notices/${encodeURIComponent(publicID(input.notice_id))}/actions`, noticeActionPayload(input)))
    const notice = publicID(string(data, 'notice_id'))
    if (!bool(data, 'ok') || string(data, 'action') !== input.action || (input.action === 'received' ? notice !== input.notice_id : notice === input.notice_id)) throw new Error('通知操作结果尚未确认，请继续原请求核对')
    return notice
  },
}
