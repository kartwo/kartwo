// 经营概览接口 / Dashboard API
// 功能：严格解析经营聚合与开店配置状态，整数分在展示层按币种格式化
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:27:44

import { request } from './transport'
import { record, string, bool, integer } from './api-values'
export interface DashboardStatus {
  currency: string
  orders: { today: { count: number; sales_cents: number }; week: { count: number; sales_cents: number }; pending_fulfillment: number }
  products: { count: number; zero_stock: number; low_stock: number }
  setup: { has_products: boolean; payment_configured: boolean; domain_configured: boolean; ready: boolean }
}
function count(data: Record<string, unknown>, key: string) { return integer(data, key, 0, Number.MAX_SAFE_INTEGER) }
export function parseDashboard(value: unknown): DashboardStatus {
  const data = record(value), orders = record(data.orders), today = record(orders.today), week = record(orders.week), products = record(data.products), setup = record(data.setup), currency = string(data, 'currency')
  if (!/^[A-Z]{3}$/.test(currency)) throw new Error('概览币种格式不正确')
  const readiness = { has_products: bool(setup, 'has_products'), payment_configured: bool(setup, 'payment_configured'), domain_configured: bool(setup, 'domain_configured'), ready: bool(setup, 'ready') }
  if (readiness.ready !== (readiness.has_products && readiness.payment_configured && readiness.domain_configured)) throw new Error('开店配置状态不一致，请刷新核对')
  return { currency, orders: { today: { count: count(today, 'count'), sales_cents: count(today, 'sales_cents') }, week: { count: count(week, 'count'), sales_cents: count(week, 'sales_cents') }, pending_fulfillment: count(orders, 'pending_fulfillment') }, products: { count: count(products, 'count'), zero_stock: count(products, 'zero_stock'), low_stock: count(products, 'low_stock') }, setup: readiness }
}
export function dashboardMoney(cents: number, currency: string): string {
  try { return new Intl.NumberFormat(undefined, { style: 'currency', currency }).format(cents / 100) }
  catch { return (cents / 100).toFixed(2) + ' ' + currency }
}
export const dashboardAPI = { load: async (signal?: AbortSignal) => parseDashboard(await request('GET', '/dashboard', undefined, signal)) }
