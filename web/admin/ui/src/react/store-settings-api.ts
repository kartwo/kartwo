// 店面配置接口 / Storefront Settings API
// 功能：类型化店铺品牌、博客开关和客服留存设置，复用安全上传传输
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:34:00

import { request } from './transport'
import { record, string, bool, choice, integer } from './api-values'
export interface ShopSettings { name: string; logo_url: string; source: 'db' | 'demo'; readonly: boolean }
export interface BlogSettings { enabled: boolean }
export interface ContactSettings { enabled: boolean; retention_months: number; ip_retention_days: number }
export function parseShop(value: unknown): ShopSettings {
  const data = record(value)
  const logo = string(data, 'logo_url')
  // The backend serves processed brand images locally; reject external or executable URLs.
  if (logo && !/^\/media\/brand\/[a-zA-Z0-9_-]+\.webp$/.test(logo)) throw new Error('Logo 地址无效，请重试。')
  const source = choice(data, 'source', ['db', 'demo'] as const)
  return { name: string(data, 'name'), logo_url: logo, source, readonly: bool(data, 'readonly') || source === 'demo' }
}
export function parseBlog(value: unknown): BlogSettings { return { enabled: bool(record(value), 'enabled') } }
export function parseContact(value: unknown): ContactSettings {
  const data = record(value)
  return { enabled: bool(data, 'enabled'), retention_months: integer(data, 'retention_months', 1, 24), ip_retention_days: integer(data, 'ip_retention_days', 7, 90) }
}
export const storeSettingsAPI = {
  shop: async (signal?: AbortSignal) => parseShop(await request('GET', '/settings/shop', undefined, signal)),
  saveShop: async (name: string) => parseShop(await request('PUT', '/settings/shop', { name: name.trim() })),
  uploadLogo: async (file: File) => {
    const body = new FormData(); body.append('file', file)
    return parseShop(await request('POST', '/settings/shop/logo', body))
  },
  removeLogo: async () => parseShop(await request('DELETE', '/settings/shop/logo')),
  blog: async (signal?: AbortSignal) => parseBlog(await request('GET', '/settings/blog', undefined, signal)),
  saveBlog: async (enabled: boolean) => parseBlog(await request('PUT', '/settings/blog', { enabled })),
  contact: async (signal?: AbortSignal) => parseContact(await request('GET', '/settings/contact', undefined, signal)),
  saveContact: async (input: ContactSettings) => parseContact(await request('PUT', '/settings/contact', {
    enabled: input.enabled, retention_months: input.retention_months, ip_retention_days: input.ip_retention_days,
  })),
}
