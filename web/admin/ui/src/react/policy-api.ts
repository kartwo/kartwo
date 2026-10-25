// 政策资料接口 / Policy Profile API
// 功能：校验七洲配送时效和经营资料，精确提交资料与页面生成选项
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:01:04

import { request } from './transport'
import { record, string, bool, choice, integer } from './api-values'
import { continents } from './shipping-api'
export const policyRegions = continents.filter(region => region !== 'Other')
export interface DeliveryEstimate { region: string; min_business_days: number; max_business_days: number }
export interface PolicyProfile { support_email: string; ship_from: string; processing_hours: number; return_window_days: number; return_shipping_payer: 'buyer' | 'merchant'; business_name: string; delivery_estimates: DeliveryEstimate[] }
export interface PolicyStatus { configured: boolean; profile: PolicyProfile }
export function parsePolicy(value: unknown): PolicyStatus {
  const data = record(value), profile = record(data.profile), raw = profile.delivery_estimates
  if (!Array.isArray(raw) || raw.length !== 7) throw new Error('店铺政策须包含七个洲的配送时效')
  const seen = new Set<string>()
  const delivery_estimates = raw.map(value => {
    const row = record(value), region = choice(row, 'region', policyRegions)
    if (seen.has(region)) throw new Error('配送时效地区重复')
    seen.add(region)
    const min_business_days = integer(row, 'min_business_days', 1, 120), max_business_days = integer(row, 'max_business_days', 1, 120)
    if (max_business_days < min_business_days) throw new Error('配送时效范围无效')
    return { region, min_business_days, max_business_days }
  })
  return { configured: bool(data, 'configured'), profile: { support_email: string(profile, 'support_email'), ship_from: string(profile, 'ship_from'), processing_hours: integer(profile, 'processing_hours', 1, 720), return_window_days: integer(profile, 'return_window_days', 1, 365), return_shipping_payer: choice(profile, 'return_shipping_payer', ['buyer', 'merchant']), business_name: string(profile, 'business_name'), delivery_estimates } }
}
export const policyAPI = {
  load: async (signal?: AbortSignal) => parsePolicy(await request('GET', '/settings/policy-profile', undefined, signal)),
  save: async (input: PolicyProfile) => {
    const result = await request('PUT', '/settings/policy-profile', {
      support_email: input.support_email.trim(), ship_from: input.ship_from.trim(), processing_hours: input.processing_hours, return_window_days: input.return_window_days, return_shipping_payer: input.return_shipping_payer, business_name: input.business_name.trim(),
      delivery_estimates: input.delivery_estimates.map(row => ({ region: row.region, min_business_days: row.min_business_days, max_business_days: row.max_business_days })),
    })
    if (!bool(record(result), 'ok')) throw new Error('服务器未确认政策资料保存成功')
  },
  generate: async (publish: boolean, overwrite: boolean) => {
    const result = record(await request('POST', '/content-pages/generate-footer', { publish, overwrite }))
    return { created: integer(result, 'created', 0, 7), updated: integer(result, 'updated', 0, 7), skipped: integer(result, 'skipped', 0, 7) }
  },
}
