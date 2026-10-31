// 商品编辑模型 / Product Editing Model
// 功能：精确生成变体、识别过期组合、解析库存与权限，保留部分保存结果
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:22:28

import { can } from './access'
import type { Session } from './contracts'
import { productAPI, type Product, type ProductOption, type Selection, type Variant } from './product-api'
import { APIError, errorMessage } from './transport'
export interface AxisDraft { name: string; valuesText: string }
export interface DraftVariant { selections: Selection[]; sku: string; price: string; quantity: string }
export function productRights(session: Session | null, product: Product | null) {
  const demo = session?.role === 'demo', own = demo && (!product || product.demo_owned)
  return { demo, protectedDemo: demo && !!product && !product.demo_owned,
    basic: own || can(session, product ? 'products.edit' : 'products.create'), publish: !demo && can(session, 'products.publish'),
    price: own || can(session, 'products.price'), inventory: own || can(session, 'products.inventory'), media: own || can(session, 'products.media'), translate: session?.role === 'owner' }
}
export function productSlug(value: string) { return value.toLowerCase().normalize('NFKD').replace(/[\u0300-\u036f]/g, '').replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 72).replace(/-+$/g, '') }
export function quantityInteger(value: string): number {
  const text = value.trim()
  if (!/^\d+$/.test(text) || text.length > 16 || BigInt(text) > BigInt(Number.MAX_SAFE_INTEGER)) throw new Error('库存须为非负安全整数，不能为空或小数')
  return Number(text)
}
export function parseAxes(axes: AxisDraft[]): ProductOption[] {
  if (!axes.length) throw new Error('至少需要一个变体轴')
  const options = axes.map(axis => ({ name: axis.name.trim(), values: axis.valuesText.split(',').map(value => value.trim()).filter(Boolean) }))
  if (options.some(option => !option.name || !option.values.length)) throw new Error('每个变体轴都需要名称和至少一个取值')
  if (new Set(options.map(option => option.name)).size !== options.length) throw new Error('变体轴名称不能重复')
  if (options.some(option => new Set(option.values).size !== option.values.length)) throw new Error('同一变体轴的取值不能重复')
  return options
}
export function generateVariants(options: ProductOption[]): DraftVariant[] {
  const count = options.reduce((total, option) => total * option.values.length, 1)
  const bytes = (text: string) => new TextEncoder().encode(text).length
  const minimumSelections = options.map(option => ({ option: option.name, value: option.values.reduce((small, value) => bytes(JSON.stringify(value)) < bytes(JSON.stringify(small)) ? value : small) }))
  const minimumRowBytes = bytes(JSON.stringify({ sku: '', price_cents: 0, quantity: 0, selections: minimumSelections }))
  // This is a lower bound against the existing server JSON limit, not a new business variant quota.
  if (!Number.isSafeInteger(count) || count * minimumRowBytes > (1 << 20)) throw new Error('变体组合过多，已超过单次接口的 1MB 上限，请减少轴取值')
  let selections: Selection[][] = [[]]
  for (const option of options) selections = selections.flatMap(items => option.values.map(value => [...items, { option: option.name, value }]))
  return selections.map(items => ({ selections: items, sku: '', price: '', quantity: '0' }))
}
export interface VariantUpdate { price?: number; quantity?: number; warning: string }
export async function saveVariantChanges(variant: Variant, price: number | undefined, quantity: number | undefined): Promise<VariantUpdate> {
  const result: VariantUpdate = { warning: '' }
  if (price !== undefined && price !== variant.price_cents) {
    try { await productAPI.price(variant.public_id, price); result.price = price }
    catch (failure) {
      if (failure instanceof APIError && failure.status === 401) throw failure
      throw new Error(`价格保存未确认，未继续保存库存。请核对后重试：${errorMessage(failure)}`)
    }
  }
  if (quantity !== undefined && quantity !== variant.quantity) {
    try { await productAPI.inventory(variant.public_id, quantity); result.quantity = quantity }
    catch (failure) {
      if (failure instanceof APIError && failure.status === 401) throw failure
      if (result.price === undefined) throw new Error(`库存保存未确认，请核对后重试：${errorMessage(failure)}`)
      result.warning = `价格已保存，库存保存未确认：${errorMessage(failure)}。请核对后重试库存；不会自动重复保存价格。`
    }
  }
  return result
}
