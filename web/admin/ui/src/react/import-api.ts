// 商品导入接口 / Product Import API
// 功能：CSV 预览与幂等执行契约，严格绑定任务标识和格式，支持只读核对
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:27:44

import { request } from './transport'
import { record, string, choice, integer } from './api-values'
export type ImportFormat = 'generic' | 'shopify'
export interface ImportPreview { public_id: string; format: ImportFormat; status: 'previewed' | 'rejected' | 'succeeded'; total_rows: number; product_count: number; variant_count: number; errors: { row: number; message: string }[] }
function jobID(id: string): string {
  if (!/^[a-zA-Z0-9-]{8,}$/.test(id)) throw new Error('导入任务标识无效')
  return encodeURIComponent(id)
}
export function parseImport(value: unknown): ImportPreview {
  const data = record(value), id = string(data, 'public_id'), status = choice(data, 'status', ['previewed', 'rejected', 'succeeded'])
  jobID(id)
  const rows = data.errors === null ? [] : data.errors
  if (!Array.isArray(rows)) throw new Error('导入错误列表缺失或格式不正确')
  const errors = rows.map(value => { const row = record(value); return { row: integer(row, 'row', 0, Number.MAX_SAFE_INTEGER), message: string(row, 'message') } })
  const total_rows = integer(data, 'total_rows', 0, Number.MAX_SAFE_INTEGER), product_count = integer(data, 'product_count', 0, Number.MAX_SAFE_INTEGER), variant_count = integer(data, 'variant_count', 0, Number.MAX_SAFE_INTEGER)
  if (status !== 'rejected' && (errors.length || !product_count || !variant_count)) throw new Error('导入任务状态与检查结果不一致')
  return { public_id: id, format: choice(data, 'format', ['generic', 'shopify']), status, total_rows, product_count, variant_count, errors }
}
function match(value: unknown, preview: ImportPreview) {
  const next = parseImport(value)
  if (next.public_id !== preview.public_id || next.format !== preview.format) throw new Error('返回结果不属于当前导入任务，请重新核对')
  return next
}
export const importAPI = {
  preview: async (file: File, format: ImportFormat) => {
    if (file.size > 5 * 1024 * 1024) throw new Error('CSV 文件不能超过 5MB')
    const body = new FormData(); body.append('file', file); body.append('format', format)
    const result = parseImport(await request('POST', '/imports/csv/preview', body))
    if (result.format !== format) throw new Error('预览格式与所选来源不一致，请重试')
    return result
  },
  execute: async (preview: ImportPreview) => {
    if (preview.status !== 'previewed' || preview.errors.length) throw new Error('请先通过预览检查后再确认导入')
    return match(await request('POST', '/imports/' + jobID(preview.public_id) + '/execute'), preview)
  },
  check: async (preview: ImportPreview) => match(await request('GET', '/imports/' + jobID(preview.public_id)), preview),
}
