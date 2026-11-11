// 后台可观测接口 / Admin Observability API
// 功能：只读审计白名单解析及本机诊断状态校验，演示身份再次屏蔽标识
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:17:21

import { request } from './transport'
import { record, string, bool, choice, integer } from './api-values'
export interface AuditEvent { public_id: string; action: string; target_type: string; target_public_id: string; created_at: string; admin_username: string }
export interface Diagnostics {
  database: { status: 'ok' }
  media: { asset_count: number; original_bytes: number; derivative_bytes: number; total_bytes: number }
  disk: { available: boolean; total_bytes: number; free_bytes: number; used_bytes: number; health: '' | 'normal' | 'warning' | 'critical'; hint: string; message: string }
  backups: { automatic_count: number; upgrade_count: number; total_bytes: number; latest_at: string | null; message: string }
}
function count(data: Record<string, unknown>, key: string) { return integer(data, key, 0, Number.MAX_SAFE_INTEGER) }
export function parseAudit(value: unknown, demo = false): AuditEvent[] {
  const data = record(value)
  if (!Array.isArray(data.events)) throw new Error('日志列表格式不正确')
  const seen = new Set<string>()
  return data.events.map((value, index) => {
    const row = record(value), id = demo ? `demo-event-${index + 1}` : string(row, 'public_id')
    if (!id || seen.has(id)) throw new Error('日志标识缺失或重复'); seen.add(id)
    return { public_id: id, action: string(row, 'action'), target_type: string(row, 'target_type'), target_public_id: demo ? '已隐藏' : string(row, 'target_public_id'), created_at: string(row, 'created_at'), admin_username: demo ? '店主' : string(row, 'admin_username') }
  })
}
export function parseDiagnostics(value: unknown): Diagnostics {
  const data = record(value), database = record(data.database), media = record(data.media), disk = record(data.disk), backups = record(data.backups)
  const available = bool(disk, 'available'), latest = backups.latest_at
  if (latest !== null && typeof latest !== 'string') throw new Error('备份时间格式不正确')
  return {
    database: { status: choice(database, 'status', ['ok']) },
    media: { asset_count: count(media, 'asset_count'), original_bytes: count(media, 'original_bytes'), derivative_bytes: count(media, 'derivative_bytes'), total_bytes: count(media, 'total_bytes') },
    disk: { available, total_bytes: count(disk, 'total_bytes'), free_bytes: count(disk, 'free_bytes'), used_bytes: count(disk, 'used_bytes'), health: available ? choice(disk, 'health', ['normal', 'warning', 'critical']) : '', hint: string(disk, 'hint'), message: string(disk, 'message') },
    backups: { automatic_count: count(backups, 'automatic_count'), upgrade_count: count(backups, 'upgrade_count'), total_bytes: count(backups, 'total_bytes'), latest_at: latest, message: string(backups, 'message') },
  }
}
export function formatBytes(value: number): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let size = value, index = 0
  while (size >= 1024 && index < units.length - 1) { size /= 1024; index++ }
  return index === 0 ? `${size} B` : `${size.toFixed(size >= 10 ? 0 : 1)} ${units[index]}`
}
export function backupTime(value: string | null): string {
  if (!value) return '尚未生成'
  const date = new Date(value); return Number.isNaN(date.getTime()) ? '暂不可用' : date.toLocaleString()
}
export const observabilityAPI = {
  audit: async (demo: boolean, signal?: AbortSignal) => parseAudit(await request('GET', '/audit-events', undefined, signal), demo),
  diagnostics: async (signal?: AbortSignal) => parseDiagnostics(await request('GET', '/diagnostics', undefined, signal)),
}
