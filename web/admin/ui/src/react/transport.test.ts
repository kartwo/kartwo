// 请求边界验证 / Request Boundary Tests
// 功能：校验真实 fetch 选项、CSRF、上传和非 JSON 错误处理
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:13:54

import { afterEach, expect, it, vi } from 'vitest'
import { request, APIError } from './transport'
afterEach(() => { document.cookie = 'kartwo_csrf=; Max-Age=0' })
it('写入带同源凭据与当前 CSRF，读取不带 CSRF，所有请求禁缓存', async () => {
  const fetch = vi.fn(async () => new Response(JSON.stringify({ ok: true }), { status: 200 }))
  vi.stubGlobal('fetch', fetch)
  document.cookie = 'kartwo_csrf=first%2Bvalue'
  await request('PUT', '/settings/blog', { enabled: true })
  document.cookie = 'kartwo_csrf=rotated'
  await request('DELETE', '/blog/posts/post-id')
  await request('GET', '/me')
  expect(fetch).toHaveBeenNthCalledWith(1, '/admin/api/settings/blog', expect.objectContaining({ credentials: 'same-origin', cache: 'no-store', method: 'PUT', body: '{"enabled":true}', headers: expect.objectContaining({ 'X-CSRF-Token': 'first+value', 'Content-Type': 'application/json' }) }))
  expect(fetch).toHaveBeenNthCalledWith(2, '/admin/api/blog/posts/post-id', expect.objectContaining({ headers: expect.objectContaining({ 'X-CSRF-Token': 'rotated' }) }))
  expect(fetch).toHaveBeenNthCalledWith(3, '/admin/api/me', expect.objectContaining({ headers: { Accept: 'application/json' } }))
})
it('FormData 由浏览器设置 boundary，保留取消信号并允许空响应', async () => {
  const fetch = vi.fn(async () => new Response(null, { status: 204 }))
  vi.stubGlobal('fetch', fetch)
  const form = new FormData(); form.append('file', new Blob(['test']), 'fixture.txt')
  const signal = new AbortController().signal
  expect(await request('POST', '/blog/images', form, signal)).toBeNull()
  expect(fetch).toHaveBeenCalledWith('/admin/api/blog/images', expect.objectContaining({ signal, body: form, headers: { Accept: 'application/json', 'X-CSRF-Token': '' } }))
})
it('保留 401/429 状态，拒绝代理 HTML 成功页，不向界面传递 HTML 错误正文', async () => {
  const fetch = vi.fn().mockResolvedValueOnce(new Response('{"error":"登录尝试过多"}', { status: 429 }))
    .mockResolvedValueOnce(new Response('{"error":"请登录"}', { status: 401 }))
    .mockResolvedValueOnce(new Response('<html>proxy failure</html>', { status: 502 }))
    .mockResolvedValueOnce(new Response('<html>login redirect</html>', { status: 200 }))
  vi.stubGlobal('fetch', fetch)
  await expect(request('POST', '/login', {})).rejects.toMatchObject({ status: 429, message: '登录尝试过多' })
  await expect(request('GET', '/me')).rejects.toMatchObject({ status: 401 })
  await expect(request('GET', '/me')).rejects.toMatchObject({ message: 'HTTP 502' })
  await expect(request('GET', '/me')).rejects.toBeInstanceOf(APIError)
})
it('错误 Cookie 编码不导致崩溃，拒绝跨源或不规范路径', async () => {
  const fetch = vi.fn(async () => new Response('{}'))
  vi.stubGlobal('fetch', fetch)
  document.cookie = 'kartwo_csrf=%'
  await request('POST', '/logout')
  expect(fetch).toHaveBeenCalledWith('/admin/api/logout', expect.objectContaining({ headers: expect.objectContaining({ 'X-CSRF-Token': '' }) }))
  for (const path of ['https://example.com', '//example.com', '/\\example']) await expect(request('GET', path)).rejects.toThrow('无效')
  expect(fetch).toHaveBeenCalledTimes(1)
})
