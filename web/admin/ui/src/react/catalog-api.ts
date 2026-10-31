// 分类与内容接口 / Categories and Content API
// 功能：校验分类/固定页面契约，过滤界面元数据，区分写入成功与后续刷新失败
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:34:25

import { request, APIError, errorMessage } from './transport'
import { record, string, bool, choice, integer } from './api-values'
export interface CategoryFields { name: string; slug: string; position: number }
export interface Category extends CategoryFields { public_id: string; product_count: number }
export interface ContentFields { title: string; slug: string; body_markdown: string; seo_description: string; status: 'draft' | 'active' }
export interface ContentPage extends ContentFields { public_id: string; updated_at: string }
export const emptyContent: ContentFields = { title: '', slug: '', body_markdown: '', seo_description: '', status: 'draft' }
function publicID(id: string): string {
  if (!/^[a-zA-Z0-9-]{8,}$/.test(id)) throw new Error('分类或内容页标识无效')
  return encodeURIComponent(id)
}
function array(data: Record<string, unknown>, key: string): unknown[] { const rows = data[key]; if (!Array.isArray(rows)) throw new Error('分类或内容列表格式不正确'); return rows }
function unique<T extends { public_id: string }>(rows: T[]): T[] {
  if (new Set(rows.map(row => row.public_id)).size !== rows.length) throw new Error('列表含重复标识')
  return rows
}
export function parseCategories(value: unknown): Category[] {
  return unique(array(record(value), 'categories').map(value => {
    const data = record(value), id = string(data, 'public_id'); publicID(id)
    return { public_id: id, name: string(data, 'name'), slug: string(data, 'slug'), position: integer(data, 'position', 0, Number.MAX_SAFE_INTEGER), product_count: integer(data, 'product_count', 0, Number.MAX_SAFE_INTEGER) }
  }))
}
export function parseContent(value: unknown): ContentPage {
  const data = record(value), id = string(data, 'public_id'); publicID(id)
  return { public_id: id, title: string(data, 'title'), slug: string(data, 'slug'), body_markdown: string(data, 'body_markdown'), seo_description: string(data, 'seo_description'), status: choice(data, 'status', ['draft', 'active']), updated_at: string(data, 'updated_at') }
}
function created(value: unknown): string { const id = string(record(value), 'public_id'); publicID(id); return id }
function accepted(value: unknown): void { if (!bool(record(value), 'ok')) throw new Error('服务器未确认保存成功，请刷新核对') }
function categoryPayload(input: CategoryFields) {
  if (!Number.isSafeInteger(input.position) || input.position < 0) throw new Error('分类排序须为非负安全整数')
  return { name: input.name.trim(), slug: input.slug.trim(), position: input.position }
}
function contentPayload(input: ContentFields) {
  return { title: input.title, slug: input.slug, body_markdown: input.body_markdown, seo_description: input.seo_description, status: input.status }
}
export function categorySlug(value: string): string { return value.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') }
export const catalogAPI = {
  categories: async (signal?: AbortSignal) => parseCategories(await request('GET', '/categories', undefined, signal)),
  createCategory: async (input: CategoryFields) => created(await request('POST', '/categories', categoryPayload(input))),
  saveCategory: async (id: string, input: CategoryFields) => accepted(await request('PATCH', '/categories/' + publicID(id), categoryPayload(input))),
  deleteCategory: async (id: string) => accepted(await request('DELETE', '/categories/' + publicID(id))),
  pages: async (signal?: AbortSignal) => unique(array(record(await request('GET', '/content-pages', undefined, signal)), 'pages').map(parseContent)),
  page: async (id: string, signal?: AbortSignal) => {
    const page = parseContent(await request('GET', '/content-pages/' + publicID(id), undefined, signal))
    if (page.public_id !== id) throw new Error('返回内容与选择的页面不符，请重试')
    return page
  },
  createPage: async (input: ContentFields) => created(await request('POST', '/content-pages', contentPayload(input))),
  savePage: async (id: string, input: ContentFields) => accepted(await request('PATCH', '/content-pages/' + publicID(id), contentPayload(input))),
  deletePage: async (id: string) => accepted(await request('DELETE', '/content-pages/' + publicID(id))),
}
export async function writeAndReload<T, U>(write: () => Promise<T>, load: () => Promise<U>) {
  const value = await write()
  try { return { value, data: await load(), warning: '' } }
  catch (error) {
    if (error instanceof APIError && error.status === 401) throw error
    return { value, data: null, warning: `保存操作已完成，列表刷新失败：${errorMessage(error)}。请刷新核对。` }
  }
}
