// 配送接口 / Shipping API
// 功能：读取国家与运费规则，校验公开标识和整数金额，仅提交允许字段
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:01:04

import { request } from './transport'
import { record, string, bool, choice, integer } from './api-values'
import { moneyToCents, centsToMoney } from './money'
export const continents = ['Asia', 'Europe', 'North America', 'South America', 'Africa', 'Oceania', 'Antarctica', 'Other'] as const
export type Continent = typeof continents[number]
export const continentNames: Record<Continent, string> = { Asia: '亚洲', Europe: '欧洲', 'North America': '北美洲', 'South America': '南美洲', Africa: '非洲', Oceania: '大洋洲', Antarctica: '南极洲', Other: '其他' }
export interface Country { code: string; name: string; continent: Continent }
export interface CountryStatus { countries: Country[]; enabled: string[] }
export interface ShippingZone { public_id: string; name: string; countries: string; rate_cents: number; free_over_cents: number; active: boolean; is_default: boolean }
export interface RateFields { mode: 'free' | 'flat' | 'threshold'; rate: string; freeOver: string }
export interface RuleFields extends RateFields { name: string; countries: string[] }
function array(data: Record<string, unknown>, key: string): unknown[] {
  const value = data[key]; if (!Array.isArray(value)) throw new Error('配送数据格式不正确')
  return value
}
function code(value: unknown): value is string { return typeof value === 'string' && /^[A-Z]{2}$/.test(value) }
function zoneID(id: string): string {
  if (!/^[a-zA-Z0-9-]{8,}$/.test(id)) throw new Error('配送规则标识无效')
  return encodeURIComponent(id)
}
export function parseCountries(value: unknown): CountryStatus {
  const data = record(value), codes = new Set<string>()
  const countries = array(data, 'countries').map(value => {
    const item = record(value), countryCode = string(item, 'code')
    if (!code(countryCode) || codes.has(countryCode)) throw new Error('国家代码无效或重复')
    codes.add(countryCode)
    return { code: countryCode, name: string(item, 'name'), continent: choice(item, 'continent', continents) }
  })
  const enabled = array(data, 'enabled')
  if (!enabled.every(value => code(value) && codes.has(value))) throw new Error('可配送范围包含未知国家')
  return { countries, enabled: [...new Set(enabled as string[])] }
}
export function parseZones(value: unknown): ShippingZone[] {
  const ids = new Set<string>()
  let defaults = 0
  return array(record(value), 'zones').map(value => {
    const data = record(value), id = string(data, 'public_id'), countries = string(data, 'countries'), isDefault = bool(data, 'is_default')
    zoneID(id)
    if (ids.has(id) || (countries === '') !== isDefault || (countries && !countries.split(',').every(code)) || (isDefault && ++defaults > 1)) throw new Error('配送规则数据不一致')
    ids.add(id)
    return { public_id: id, name: string(data, 'name'), countries, is_default: isDefault, active: bool(data, 'active'), rate_cents: integer(data, 'rate_cents', 0, Number.MAX_SAFE_INTEGER), free_over_cents: integer(data, 'free_over_cents', 0, Number.MAX_SAFE_INTEGER) }
  })
}
export function rateFields(zone?: ShippingZone): RateFields {
  if (!zone) return { mode: 'flat', rate: '5.00', freeOver: '200.00' }
  return { mode: zone.free_over_cents > 0 ? 'threshold' : zone.rate_cents === 0 ? 'free' : 'flat', rate: centsToMoney(zone.rate_cents), freeOver: centsToMoney(zone.free_over_cents) }
}
export function ratePayload(input: RateFields) {
  const rate_cents = input.mode === 'free' ? 0 : moneyToCents(input.rate)
  const free_over_cents = input.mode === 'threshold' ? moneyToCents(input.freeOver) : 0
  if (input.mode === 'threshold' && free_over_cents <= 0) throw new Error('包邮门槛必须大于 0')
  return { rate_cents, free_over_cents }
}
export const shippingAPI = {
  countries: async (signal?: AbortSignal) => parseCountries(await request('GET', '/settings/shipping/countries', undefined, signal)),
  zones: async (signal?: AbortSignal) => parseZones(await request('GET', '/settings/shipping', undefined, signal)),
  load: async (signal: AbortSignal) => {
    const [countries, zones] = await Promise.all([shippingAPI.countries(signal), shippingAPI.zones(signal)])
    return { countries, zones }
  },
  saveCountries: async (countries: string[]) => {
    await request('PUT', '/settings/shipping/countries', { countries: [...new Set(countries)] })
    return shippingAPI.countries()
  },
  saveDefault: async (input: RateFields) => {
    await request('PUT', '/settings/shipping/default', { name: 'Default', countries: '', ...ratePayload(input) })
    return shippingAPI.zones()
  },
  saveRule: async (input: RuleFields, id = '') => {
    if (!input.name.trim() || !input.countries.length || !input.countries.every(code)) throw new Error('请填写规则名称并选择有效国家')
    await request(id ? 'PATCH' : 'POST', '/settings/shipping' + (id ? '/' + zoneID(id) : ''), { name: input.name.trim(), countries: [...new Set(input.countries)].join(','), ...ratePayload(input) })
    return shippingAPI.zones()
  },
  removeRule: async (id: string) => { await request('DELETE', '/settings/shipping/' + zoneID(id)); return shippingAPI.zones() },
}
