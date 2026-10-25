// 安全配置接口 / Security Settings API
// 功能：账号、翻译及备份契约，过滤只读配置写入且不保存回传秘密
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:55:07

import { request, type JSONValue } from './transport'
import { record, string, bool, choice, integer } from './api-values'
export interface AccountSettings { username: string; readonly: boolean }
export interface AccountFields { username: string; current_password: string; new_password: string }
export interface TranslationFields { plan: 'developer' | 'growth'; api_key: string }
export interface TranslationStatus { provider: 'deepl'; plan: TranslationFields['plan']; has_api_key: boolean; price_note: string; developer_quota: string; growth_quota: string }
export interface BackupFields { interval: string; retention: string; webdav_enabled: boolean; webdav_url: string; webdav_path: string; webdav_username: string; webdav_password: string }
export const backupKeys = ['interval', 'retention', 'webdav_enabled', 'webdav_url', 'webdav_path', 'webdav_username', 'webdav_password'] as const
export type BackupKey = typeof backupKeys[number]
export interface BackupStatus extends Omit<BackupFields, 'webdav_password' | 'retention'> { retention: number; readonly: Record<BackupKey, boolean>; demo_readonly: boolean; webdav_password_set: boolean; restart_required: boolean }
export function parseAccount(value: unknown): AccountSettings {
  const data = record(value); return { username: string(data, 'username'), readonly: bool(data, 'readonly') }
}
export function parseTranslation(value: unknown): TranslationStatus {
  const data = record(value)
  return { provider: choice(data, 'provider', ['deepl']), plan: choice(data, 'plan', ['developer', 'growth']), has_api_key: bool(data, 'has_api_key'), price_note: string(data, 'price_note'), developer_quota: string(data, 'developer_quota'), growth_quota: string(data, 'growth_quota') }
}
export function parseBackup(value: unknown): BackupStatus {
  const data = record(value), demo = bool(data, 'demo_readonly')
  const readonly = {} as Record<BackupKey, boolean>
  for (const key of backupKeys) {
    const locked = bool(data, `${key}_readonly`)
    const source = key === 'webdav_password' ? '' : choice(data, `${key}_source`, ['env', 'db', 'default'])
    readonly[key] = locked || demo || source === 'env'
  }
  const enabled = data.webdav_enabled
  if (enabled !== true && enabled !== false && enabled !== 'true' && enabled !== 'false') throw new Error('WebDAV 启用状态无法识别，请重试。')
  return { interval: string(data, 'interval'), retention: integer(data, 'retention', 1, 365), webdav_enabled: enabled === true || enabled === 'true', webdav_url: string(data, 'webdav_url'), webdav_path: string(data, 'webdav_path'), webdav_username: string(data, 'webdav_username'), readonly, demo_readonly: demo, webdav_password_set: bool(data, 'webdav_password_set'), restart_required: bool(data, 'restart_required') }
}
export function backupPayload(input: BackupFields, status: BackupStatus): Record<string, JSONValue> {
  const payload: Record<string, JSONValue> = {}
  if (status.demo_readonly) return payload
  for (const key of backupKeys) {
    if (status.readonly[key] || (key === 'webdav_password' && input[key] === '')) continue
    const value = input[key]
    payload[key] = typeof value === 'string' && key !== 'webdav_password' ? value.trim() : value
  }
  return payload
}
function accepted(value: unknown) {
  if (!bool(record(value), 'ok')) throw new Error('服务器未确认操作成功，请重试。')
}
export const securitySettingsAPI = {
  account: async (signal?: AbortSignal) => parseAccount(await request('GET', '/account', undefined, signal)),
  saveOwner: async (input: AccountFields) => {
    const data = record(await request('PUT', '/account', { username: input.username.trim(), current_password: input.current_password, new_password: input.new_password }))
    accepted(data)
    if (!bool(data, 'signed_out')) throw new Error('账号已更新，请重新登录以确认会话状态。')
  },
  saveStaff: async (input: AccountFields) => accepted(await request('PUT', '/account/password', { current_password: input.current_password, new_password: input.new_password })),
  translation: async (signal?: AbortSignal) => parseTranslation(await request('GET', '/settings/translation', undefined, signal)),
  saveTranslation: async (input: TranslationFields) => parseTranslation(await request('PUT', '/settings/translation', { plan: input.plan, api_key: input.api_key.trim() })),
  backup: async (signal?: AbortSignal) => parseBackup(await request('GET', '/settings/backup', undefined, signal)),
  saveBackup: async (input: BackupFields, status: BackupStatus) => parseBackup(await request('PUT', '/settings/backup', backupPayload(input, status))),
  testBackup: async () => accepted(await request('POST', '/settings/backup/test')),
}
