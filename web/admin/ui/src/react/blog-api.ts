// 博客接口 / Blog API
// 功能：校验文章、旧 Markdown 预览和媒体契约，只提交业务白名单字段
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:56:32

import { request } from './transport'
import { record, string, choice, bool, integer } from './api-values'
export interface BlogFields {
  title: string; slug: string; excerpt: string; seo_description: string; cover_path: string
  body_html: string; body_markdown: string; status: 'draft' | 'active'
}
export interface BlogPost extends BlogFields { public_id: string; published_at: string; updated_at: string }
export const emptyBlog: BlogFields = { title: '', slug: '', excerpt: '', seo_description: '', cover_path: '', body_html: '', body_markdown: '', status: 'draft' }
export const blogImagePath = /^blog\/[a-f0-9]{64}\.webp$/
function publicID(id: string) {
  if (!/^[a-zA-Z0-9-]{8,}$/.test(id)) throw new Error('文章标识无效')
  return encodeURIComponent(id)
}
export function parsePost(value: unknown): BlogPost {
  const data = record(value), id = string(data, 'public_id'), cover = string(data, 'cover_path')
  publicID(id)
  if (cover && !blogImagePath.test(cover)) throw new Error('文章封面地址无效')
  return { public_id: id, title: string(data, 'title'), slug: string(data, 'slug'), excerpt: string(data, 'excerpt'),
    seo_description: string(data, 'seo_description'), cover_path: cover, body_html: string(data, 'body_html'), body_markdown: string(data, 'body_markdown'),
    status: choice(data, 'status', ['draft', 'active']), published_at: string(data, 'published_at'), updated_at: string(data, 'updated_at') }
}
export function parsePosts(value: unknown): BlogPost[] {
  const rows = record(value).posts
  if (!Array.isArray(rows)) throw new Error('文章列表格式不正确')
  const posts = rows.map(parsePost)
  if (new Set(posts.map(post => post.public_id)).size !== posts.length) throw new Error('文章列表包含重复标识')
  return posts
}
function payload(post: BlogFields) {
  return { title: post.title, slug: post.slug, excerpt: post.excerpt, seo_description: post.seo_description,
    cover_path: post.cover_path, body_html: post.body_html, body_markdown: post.body_markdown, status: post.status }
}
async function ok(method: 'PUT' | 'DELETE', path: string, body?: ReturnType<typeof payload>) {
  if (!bool(record(await request(method, path, body)), 'ok')) throw new Error('文章操作未完成')
}
export const blogAPI = {
  list: async (signal?: AbortSignal) => parsePosts(await request('GET', '/blog/posts', undefined, signal)),
  get: async (id: string, signal: AbortSignal) => {
    const post = parsePost(await request('GET', `/blog/posts/${publicID(id)}`, undefined, signal))
    if (post.public_id !== id) throw new Error('文章详情与所选文章不一致')
    return post
  },
  create: async (post: BlogFields) => { const id = string(record(await request('POST', '/blog/posts', payload(post))), 'public_id'); publicID(id); return id },
  update: (id: string, post: BlogFields) => ok('PUT', `/blog/posts/${publicID(id)}`, payload(post)),
  remove: (id: string) => ok('DELETE', `/blog/posts/${publicID(id)}`),
  preview: async (markdown: string, signal: AbortSignal) => string(record(await request('POST', '/blog/preview', { body_markdown: markdown }, signal)), 'html'),
  upload: async (file: File) => {
    if (!['image/jpeg', 'image/png', 'image/webp'].includes(file.type) || !file.size) throw new Error('请选择非空的 JPEG、PNG 或 WebP 图片')
    const body = new FormData(); body.append('file', file)
    const data = record(await request('POST', '/blog/images', body)), path = string(data, 'path'), url = string(data, 'url')
    if (!blogImagePath.test(path) || url !== `/media/${path}`) throw new Error('服务器返回的博客图片地址无效')
    integer(data, 'width', 1, Number.MAX_SAFE_INTEGER); integer(data, 'height', 1, Number.MAX_SAFE_INTEGER)
    return { path, url }
  },
}
