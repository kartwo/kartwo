// 商品与媒体接口 / Products and Media API
// 功能：校验商品、变体及本地图片契约，限定整数金额库存和请求白名单
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:22:28

import { request, type JSONValue } from './transport'
import { record, string, bool, choice, integer } from './api-values'
export type ProductStatus = 'draft' | 'active' | 'archived'
export interface ProductFields { title: string; title_zh: string; description: string; seo_description: string; seo_description_zh: string; status: ProductStatus; category_public_ids: string[] }
export interface ProductSummary { public_id: string; title: string; slug: string; status: ProductStatus; featured: boolean; demo_owned: boolean; updated_at: string }
export interface Variant { public_id: string; sku: string; price_cents: number; quantity: number; options: { name: string; value: string }[] }
export interface Product extends Omit<ProductSummary, 'updated_at'>, ProductFields { slug_zh: string; variants: Variant[] }
export interface ProductOption { name: string; values: string[] }
export interface Selection { option: string; value: string }
export interface CreateProduct extends ProductFields { slug: string; slug_zh: string; options: ProductOption[]; variants: { sku: string; price_cents: number; quantity: number; selections: Selection[] }[] }
export interface ProductMedia { public_id: string; mime: string; width: number; height: number; size_bytes: number; alt_text: string; original_url: string; derivatives: { label: string; url: string; width: number; height: number }[] }
export const emptyProduct = { title: '', title_zh: '', slug: '', slug_zh: '', description: '', seo_description: '', seo_description_zh: '', status: 'draft' as ProductStatus, category_public_ids: [] as string[] }
export function productID(id: string) { if (!/^[a-zA-Z0-9-]{8,}$/.test(id)) throw new Error('商品、变体或图片标识无效'); return encodeURIComponent(id) }
function rows(value: unknown): unknown[] { if (!Array.isArray(value)) throw new Error('商品或图片列表格式不正确'); return value }
function unique<T extends { public_id: string }>(items: T[]) { if (new Set(items.map(item => item.public_id)).size !== items.length) throw new Error('商品、变体或图片标识重复'); return items }
function id(data: Record<string, unknown>) { const value = string(data, 'public_id'); productID(value); return value }
function status(data: Record<string, unknown>) { return choice(data, 'status', ['draft', 'active', 'archived']) }
function safeInteger(value: number) { if (!Number.isSafeInteger(value) || value < 0) throw new Error('金额和库存必须为非负安全整数'); return value }
export function parseProducts(value: unknown): ProductSummary[] {
  return unique(rows(record(value).products).map(value => { const data = record(value); return { public_id: id(data), title: string(data, 'title'), slug: string(data, 'slug'), status: status(data), featured: bool(data, 'featured'), demo_owned: bool(data, 'demo_owned'), updated_at: string(data, 'updated_at') } }))
}
export function parseProduct(value: unknown): Product {
  const data = record(value)
  const categories = rows(data.category_public_ids).map(value => { if (typeof value !== 'string') throw new Error('商品分类标识无效'); productID(value); return value })
  if (new Set(categories).size !== categories.length) throw new Error('商品分类标识重复')
  return { public_id: id(data), title: string(data, 'title'), title_zh: string(data, 'title_zh'), slug: string(data, 'slug'), slug_zh: string(data, 'slug_zh'), description: string(data, 'description'),
    seo_description: string(data, 'seo_description'), seo_description_zh: string(data, 'seo_description_zh'), status: status(data), category_public_ids: categories, featured: bool(data, 'featured'), demo_owned: bool(data, 'demo_owned'),
    variants: unique(rows(data.variants).map(value => { const row = record(value); return { public_id: id(row), sku: string(row, 'sku'), price_cents: integer(row, 'price_cents', 0, Number.MAX_SAFE_INTEGER), quantity: integer(row, 'quantity', 0, Number.MAX_SAFE_INTEGER),
      options: rows(row.options).map(value => { const option = record(value); return { name: string(option, 'name'), value: string(option, 'value') } }) } })) }
}
function mediaURL(raw: string) {
  if (!/^\/media\/(?:originals\/[a-f0-9]{64}\.(?:jpg|png|webp)|derived\/[a-f0-9]{64}_[a-z0-9_-]+\.webp)$/.test(raw)) throw new Error('商品图片地址无效')
  return raw
}
export function parseMedia(value: unknown): ProductMedia {
  const data = record(value)
  const derivatives = rows(data.derivatives).map(value => { const row = record(value); return { label: string(row, 'label'), url: mediaURL(string(row, 'url')), width: integer(row, 'width', 1, Number.MAX_SAFE_INTEGER), height: integer(row, 'height', 1, Number.MAX_SAFE_INTEGER) } })
  if (new Set(derivatives.map(item => item.label)).size !== derivatives.length) throw new Error('图片派生尺寸重复')
  return { public_id: id(data), mime: choice(data, 'mime', ['image/jpeg', 'image/png', 'image/webp']), width: integer(data, 'width', 1, Number.MAX_SAFE_INTEGER), height: integer(data, 'height', 1, Number.MAX_SAFE_INTEGER),
    size_bytes: integer(data, 'size_bytes', 0, Number.MAX_SAFE_INTEGER), alt_text: string(data, 'alt_text'), original_url: mediaURL(string(data, 'original_url')), derivatives }
}
export function productFields(input: ProductFields) {
  return { title: input.title, title_zh: input.title_zh, description: input.description, seo_description: input.seo_description, seo_description_zh: input.seo_description_zh,
    status: input.status, category_public_ids: input.category_public_ids.map(value => { productID(value); return value }) }
}
function jsonLimit<T extends JSONValue>(body: T): T { if (new TextEncoder().encode(JSON.stringify(body)).length > (1 << 20)) throw new Error('商品内容超过接口的 1MB 上限，请减少文字或变体组合'); return body }
async function ok(method: 'PATCH' | 'DELETE', path: string, body?: JSONValue) { if (!bool(record(await request(method, path, body)), 'ok')) throw new Error('服务器未确认商品操作成功，请刷新核对') }
export const productAPI = {
  list: async (signal?: AbortSignal) => parseProducts(await request('GET', '/products', undefined, signal)),
  get: async (publicID: string, signal?: AbortSignal) => { const product = parseProduct(await request('GET', `/products/${productID(publicID)}`, undefined, signal)); if (product.public_id !== publicID) throw new Error('返回商品与所选商品不符'); return product },
  create: async (input: CreateProduct) => id(record(await request('POST', '/products', jsonLimit({ ...productFields(input), slug: input.slug, slug_zh: input.slug_zh,
    options: input.options.map(option => ({ name: option.name, values: [...option.values] })), variants: input.variants.map(variant => ({ sku: variant.sku, price_cents: safeInteger(variant.price_cents), quantity: safeInteger(variant.quantity), selections: variant.selections.map(selection => ({ option: selection.option, value: selection.value })) })) })))),
  update: (publicID: string, input: ProductFields) => ok('PATCH', `/products/${productID(publicID)}`, jsonLimit(productFields(input))),
  remove: (publicID: string) => ok('DELETE', `/products/${productID(publicID)}`),
  featured: (publicID: string, featured: boolean) => ok('PATCH', `/products/${productID(publicID)}/featured`, { featured }),
  price: (publicID: string, price_cents: number) => ok('PATCH', `/variants/${productID(publicID)}/price`, { price_cents: safeInteger(price_cents) }),
  inventory: (publicID: string, quantity: number) => ok('PATCH', `/variants/${productID(publicID)}/inventory`, { quantity: safeInteger(quantity) }),
  media: async (publicID: string, signal?: AbortSignal) => unique(rows(record(await request('GET', `/products/${productID(publicID)}/media`, undefined, signal)).media).map(parseMedia)),
  upload: async (publicID: string, file: File, maxBytes: number) => {
    if (!['image/jpeg', 'image/png', 'image/webp'].includes(file.type) || !file.size) throw new Error('请选择非空的 JPEG、PNG 或 WebP 图片')
    if (file.size > maxBytes) throw new Error('图片超过当前允许的大小')
    const body = new FormData(); body.append('file', file)
    return parseMedia(await request('POST', `/products/${productID(publicID)}/media`, body))
  },
  deleteMedia: (publicID: string) => ok('DELETE', `/media/${productID(publicID)}`),
  alt: (publicID: string, alt_text: string) => ok('PATCH', `/media/${productID(publicID)}`, { alt_text: alt_text.trim() }),
  translate: async (text: string) => {
    if (!text.trim()) throw new Error('请先填写中文辅助内容')
    if (!bool(record(await request('GET', '/settings/translation')), 'configured')) return null
    const translated = string(record(await request('POST', '/translation/text', { text })), 'text')
    if (!translated.trim()) throw new Error('翻译结果为空，请重试或手工填写')
    return translated
  },
}
