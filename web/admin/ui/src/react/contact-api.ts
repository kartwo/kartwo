// 客服收件箱接口 / Contact Inbox API
// 功能：校验私密留言与状态，演示脱敏优先且详情须匹配所选对象
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:17:21

import { request } from './transport'
import { record, string, choice, bool } from './api-values'
export const contactStatuses = { new: '新留言', processing: '处理中', closed: '已结案' } as const
export const contactTopics = { product: '商品', shipping: '配送', order: '订单', returns: '退货', other: '其他' } as const
export type ContactStatus = keyof typeof contactStatuses
export interface ContactMessage { public_id: string; topic: keyof typeof contactTopics; status: ContactStatus; name: string; email: string; body: string; order_ref: string; ip_address: string; country_code: string; created_at: string; updated_at: string; closed_at: string; anonymized_at: string; processing_by_name: string; processing_at: string; closed_by_name: string }
export interface ContactList { messages: ContactMessage[]; demo_hidden: boolean }
function messageID(value: string): string {
  if (!/^[a-zA-Z0-9-]{8,}$/.test(value)) throw new Error('留言标识无效')
  return encodeURIComponent(value)
}
export function parseMessage(value: unknown): ContactMessage {
  const data = record(value), id = string(data, 'public_id'), country = string(data, 'country_code')
  messageID(id)
  if (country !== '' && !/^[A-Z]{2}$/.test(country)) throw new Error('留言国家代码无效')
  return { public_id: id, topic: choice(data, 'topic', ['product', 'shipping', 'order', 'returns', 'other']), status: choice(data, 'status', ['new', 'processing', 'closed']), name: string(data, 'name'), email: string(data, 'email'), body: string(data, 'body'), order_ref: string(data, 'order_ref'), ip_address: string(data, 'ip_address'), country_code: country, created_at: string(data, 'created_at'), updated_at: string(data, 'updated_at'), closed_at: string(data, 'closed_at'), anonymized_at: string(data, 'anonymized_at'), processing_by_name: string(data, 'processing_by_name'), processing_at: string(data, 'processing_at'), closed_by_name: string(data, 'closed_by_name') }
}
export function parseContactList(value: unknown, demo: boolean): ContactList {
  const data = record(value), hidden = demo || (data.demo_hidden !== undefined && bool(data, 'demo_hidden'))
  if (hidden) return { messages: [], demo_hidden: true }
  if (!Array.isArray(data.messages)) throw new Error('客服留言列表格式不正确')
  const messages = data.messages.map(parseMessage)
  if (new Set(messages.map(row => row.public_id)).size !== messages.length) throw new Error('客服留言标识重复')
  return { messages, demo_hidden: false }
}
function matches(value: unknown, id: string): ContactMessage {
  const message = parseMessage(value)
  if (message.public_id !== id) throw new Error('返回的留言与当前选择不符，请重试')
  return message
}
export const contactAPI = {
  list: async (status: ContactStatus | '', demo: boolean, signal?: AbortSignal) => parseContactList(await request('GET', '/contact/messages' + (status ? '?status=' + encodeURIComponent(status) : ''), undefined, signal), demo),
  get: async (id: string, signal?: AbortSignal) => matches(await request('GET', '/contact/messages/' + messageID(id), undefined, signal), id),
  setStatus: async (id: string, status: ContactStatus) => matches(await request('PATCH', '/contact/messages/' + messageID(id), { status }), id),
}
