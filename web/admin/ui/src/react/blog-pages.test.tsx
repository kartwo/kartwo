// 博客页面迁移测试 / Blog Pages Migration Tests
// 功能：验证图文正文提交、Markdown 兼容、固定链接、上传互斥、读取竞态及员工/演示权限
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:56:32

import { Suspense } from 'react'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfigProvider } from 'antd'
import { MemoryRouter, useLocation, useNavigate } from 'react-router-dom'
import { expect, it, vi } from 'vitest'
import { AdminRoot } from './AdminRoot'
import { AdminTheme } from './theme'
import { SessionProvider } from './SessionProvider'
import { SessionController } from './session'
import { businessPages } from './BusinessPages'
import { PageLoading } from './SettingsPages'
import { account, fakeAPI, deferred } from './test-fixtures'
import { blogAPI, parsePost, parsePosts, type BlogPost } from './blog-api'
import type { Session } from './contracts'
const cover = `blog/${'a'.repeat(64)}.webp`, uploaded = `blog/${'b'.repeat(64)}.webp`
const postA: BlogPost = { public_id: 'article-fixture-a', title: 'Running News', slug: 'running-news', excerpt: 'Summary', seo_description: 'SEO', cover_path: cover,
  body_html: '<p>Body A</p>', body_markdown: '', status: 'active', published_at: '2026-10-25', updated_at: '2026-10-25' }
const postB: BlogPost = { ...postA, public_id: 'article-fixture-b', title: 'Other News', slug: 'other-news', body_html: '<p>Body B</p>', status: 'draft' }
function response(value: unknown, status = 200) { return new Response(JSON.stringify(value), { status }) }
function Controls() { const location = useLocation(), navigate = useNavigate(); return <><output aria-label="测试当前地址">{location.pathname}</output><button onClick={() => void navigate('/blog/article-fixture-b')}>测试切到 B</button></> }
function mount(path: string, session: Session = account()) {
  const controller = new SessionController(fakeAPI(session))
  render(<MemoryRouter initialEntries={[path]}><AdminTheme><ConfigProvider theme={{ token: { motion: false } }}><SessionProvider controller={controller}><Controls /><AdminRoot renderPage={route => {
    const Page = businessPages[route.path as keyof typeof businessPages]
    return <Suspense fallback={<PageLoading />}><Page /></Suspense>
  }} /></SessionProvider></ConfigProvider></AdminTheme></MemoryRouter>)
  return controller
}
function submit() { fireEvent.submit(screen.getByRole('textbox', { name: '标题' }).closest('form')!) }
function change(label: string, value: string) { fireEvent.change(screen.getByRole('textbox', { name: label }), { target: { value } }) }
it('文章契约白名单拒绝重复 ID、未知状态和非法封面，不回传元数据', async () => {
  expect(() => parsePosts({ posts: [postA, postA] })).toThrow()
  expect(() => parsePost({ ...postA, status: 'unknown' })).toThrow()
  expect(() => parsePost({ ...postA, cover_path: '../private' })).toThrow()
  expect(parsePost({ ...postA, admin_password: 'not-a-real-value' })).not.toHaveProperty('admin_password')
  const fetch = vi.fn(async (_url: string, options: RequestInit) => response(options.method === 'POST' ? { public_id: postA.public_id } : { ok: true })); vi.stubGlobal('fetch', fetch)
  await blogAPI.create(postA)
  await blogAPI.update(postA.public_id, postA)
  const options = fetch.mock.calls[1]?.[1] as RequestInit | undefined
  expect(JSON.parse(fetch.mock.calls[0]![1].body as string).body_html).toBe(postA.body_html)
  expect(options?.method).toBe('PUT'); expect(JSON.parse(options?.body as string)).toEqual({ title: postA.title, slug: postA.slug, excerpt: postA.excerpt, seo_description: postA.seo_description, cover_path: cover, body_html: postA.body_html, body_markdown: '', status: 'active' })
  expect(() => blogAPI.remove('../private')).toThrow(); expect(fetch).toHaveBeenCalledTimes(2)
})
it('媒体 API 仅接收支持的非空图片，拒绝外部 URL、伪造路径和非法尺寸', async () => {
  const fetch = vi.fn(async () => response({ path: uploaded, url: 'https://outside.invalid/x', width: 100, height: 100 })); vi.stubGlobal('fetch', fetch)
  await expect(blogAPI.upload(new File(['svg'], 'a.svg', { type: 'image/svg+xml' }))).rejects.toThrow(); expect(fetch).not.toHaveBeenCalled()
  await expect(blogAPI.upload(new File(['png'], 'a.png', { type: 'image/png' }))).rejects.toThrow('图片地址无效')
})
it('博客只读员工看列表和正文，没有新建删除保存或上传权限', async () => {
  const fetch = vi.fn(async (url: string) => response(url.endsWith('/article-fixture-a') ? postA : { posts: [postA] })); vi.stubGlobal('fetch', fetch)
  mount('/blog', account('staff', ['blog.view']))
  await userEvent.click(await screen.findByRole('link', { name: postA.title }))
  expect((await screen.findByRole('textbox', { name: '标题' }) as HTMLInputElement).disabled).toBe(true)
  expect(screen.getByRole('textbox', { name: '文章正文' }).textContent).toBe('Body A')
  expect(screen.queryByRole('button', { name: '保存文章' })).toBeNull(); expect(screen.queryByLabelText('上传封面图片')).toBeNull()
  submit(); await act(async () => {}); expect(fetch).toHaveBeenCalledTimes(2)
})
it('无写权限员工不能直接新建；演示不读取真实列表或文章', async () => {
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch)
  mount('/blog/new', account('staff', ['blog.view']))
  await screen.findByText('当前账号没有访问此页面的权限'); expect(fetch).not.toHaveBeenCalled()
})
it('演示列表即使可见也不向接口请求真实文章', async () => {
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch); mount('/blog', account('demo'))
  await screen.findByText('还没有文章'); expect(fetch).not.toHaveBeenCalled(); expect(screen.queryByRole('button', { name: '新建文章' })).toBeNull()
})
it('演示直接打开文章也不读取详情', async () => {
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch); mount('/blog/article-fixture-a', account('demo'))
  await screen.findByText('公开演示不可查看真实文章或创建文章。'); expect(fetch).not.toHaveBeenCalled()
})
it('列表删除确认可取消，成功后刷新失败仍移除已删除项并提示核对', async () => {
  let deleted = false
  const fetch = vi.fn(async (_url: string, options: RequestInit) => {
    if (options.method === 'DELETE') { deleted = true; return response({ ok: true }) }
    return deleted ? response({ error: 'Refresh unavailable' }, 500) : response({ posts: [postA] })
  }); vi.stubGlobal('fetch', fetch); mount('/blog')
  await userEvent.click(await screen.findByRole('button', { name: `删除文章：${postA.title}` }))
  await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: /取\s*消/ })); expect(deleted).toBe(false)
  await userEvent.click(screen.getByRole('button', { name: `删除文章：${postA.title}` })); await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: /删\s*除/ }))
  await screen.findByText('还没有文章'); expect(screen.getByText(/列表刷新失败/)).toBeTruthy(); expect(fetch.mock.calls.filter(([, options]) => options.method === 'DELETE')).toHaveLength(1)
})
it('新建链接自动避重，手动修改后不被标题覆盖，按标题生成可恢复', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => response({ posts: [postA] }))); mount('/blog/new')
  await screen.findByRole('textbox', { name: '标题' }); change('标题', 'Running News')
  expect((screen.getByRole('textbox', { name: '文章链接' }) as HTMLInputElement).value).toBe('running-news-2')
  change('文章链接', 'my-story'); change('标题', 'Running News Today')
  expect((screen.getByRole('textbox', { name: '文章链接' }) as HTMLInputElement).value).toBe('my-story')
  await userEvent.click(screen.getByRole('button', { name: '按标题生成' }))
  expect((screen.getByRole('textbox', { name: '文章链接' }) as HTMLInputElement).value).toBe('running-news-today')
})
it('新建保存读取最新编辑区图文，禁止重复提交，创建确认后返回独立列表', async () => {
  const pending = deferred<Response>(); let created = false
  const fetch = vi.fn(async (_url: string, options: RequestInit) => {
    if (options.method === 'POST') return pending.promise
    return response({ posts: created ? [postB] : [] })
  }); vi.stubGlobal('fetch', fetch); mount('/blog/new')
  await screen.findByRole('textbox', { name: '标题' }); change('标题', 'New Story')
  const editor = screen.getByRole('textbox', { name: '文章正文' }); editor.innerHTML = `<p>Latest content</p><img src="/media/${uploaded}" alt="Run">`; fireEvent.input(editor)
  submit(); submit(); await waitFor(() => expect(fetch.mock.calls.filter(([, options]) => options.method === 'POST')).toHaveLength(1))
  const sent = JSON.parse(fetch.mock.calls.find(([, options]) => options.method === 'POST')![1].body as string)
  expect(sent.body_html).toContain('Latest content'); expect(sent.body_html).toContain(`/media/${uploaded}`); expect(sent.body_markdown).toBe(''); expect(sent.status).toBe('draft'); expect(sent.slug).toBe('new-story')
  created = true; await act(async () => pending.resolve(response({ public_id: postB.public_id }, 201)))
  await waitFor(() => expect(screen.getByLabelText('测试当前地址').textContent).toBe('/blog'))
  expect(screen.queryByRole('textbox', { name: '文章正文' })).toBeNull(); expect(await screen.findByRole('link', { name: postB.title })).toBeTruthy()
})
it('编辑保存保留固定链接和发布状态，服务端失败仍保留正文供重试', async () => {
  const fetch = vi.fn(async (_url: string, options: RequestInit) => options.method === 'PUT' ? response({ error: 'Cannot save' }, 500) : response(postA)); vi.stubGlobal('fetch', fetch)
  mount('/blog/article-fixture-a', account('staff', ['blog.view', 'blog.manage']))
  await screen.findByRole('textbox', { name: '标题' }); expect((screen.getByRole('textbox', { name: '文章链接' }) as HTMLInputElement).disabled).toBe(true)
  const editor = screen.getByRole('textbox', { name: '文章正文' }); editor.innerHTML = '<p>Edited body</p>'; fireEvent.input(editor); change('标题', 'Updated')
  submit(); await screen.findByText('Cannot save')
  expect(editor.textContent).toBe('Edited body'); expect((screen.getByRole('textbox', { name: '标题' }) as HTMLInputElement).value).toBe('Updated')
  const body = JSON.parse(fetch.mock.calls.find(([, options]) => options.method === 'PUT')![1].body as string)
  expect(body.status).toBe('active'); expect(body.slug).toBe('running-news'); expect(body.body_html).toBe('<p>Edited body</p>'); expect(body).not.toHaveProperty('updated_at')
})
it('旧 Markdown 先经服务端预览转富文本，保存保留本地图片并清空 Markdown 字段', async () => {
  const fetch = vi.fn(async (url: string, options: RequestInit) => {
    if (url.endsWith('/preview')) return response({ html: `<h2>Legacy</h2><p>Original</p><img src="/media/${cover}" alt="Legacy">` })
    if (options.method === 'PUT') return response({ error: 'Keep editor' }, 500)
    return response({ ...postA, body_html: '', body_markdown: '## Legacy\n\nOriginal' })
  }); vi.stubGlobal('fetch', fetch); mount('/blog/article-fixture-a')
  const editor = await screen.findByRole('textbox', { name: '文章正文' }); expect(editor.querySelector('h2')?.textContent).toBe('Legacy'); expect(editor.querySelector('img')?.getAttribute('src')).toBe(`/media/${cover}`)
  submit(); await screen.findByText('Keep editor')
  const body = JSON.parse(fetch.mock.calls.find(([, options]) => options.method === 'PUT')![1].body as string)
  expect(body.body_markdown).toBe(''); expect(body.body_html).toContain('Original'); expect(body.body_html).toContain(`/media/${cover}`)
})
it('封面上传和正文上传共享保存锁，图片上传完成后插入正文且不改封面', async () => {
  const pending = deferred<Response>()
  const fetch = vi.fn(async (url: string, _options: RequestInit) => url.endsWith('/images') ? pending.promise : response(postA)); vi.stubGlobal('fetch', fetch); mount('/blog/article-fixture-a')
  const editor = await screen.findByRole('textbox', { name: '文章正文' })
  fireEvent.change(screen.getByLabelText('上传正文图片'), { target: { files: [new File(['fixture'], 'fixture.png', { type: 'image/png' })] } })
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2)); submit(); await act(async () => {})
  expect(fetch).toHaveBeenCalledTimes(2); expect(editor.getAttribute('contenteditable')).toBe('false')
  const request = fetch.mock.calls[1]![1] as RequestInit; expect(request.body).toBeInstanceOf(FormData); expect((request.body as FormData).get('file')).toBeInstanceOf(File)
  await act(async () => pending.resolve(response({ path: uploaded, url: `/media/${uploaded}`, width: 1200, height: 800 }, 201)))
  await waitFor(() => expect(editor.querySelector('img')?.getAttribute('src')).toBe(`/media/${uploaded}`))
  expect(screen.getByAltText('封面预览').getAttribute('src')).toBe(`/media/${cover}`); expect(editor.textContent).toContain('Body A')
})
it('封面上传失败保留旧封面，成功后只更新封面且不覆盖未保存标题和正文', async () => {
  let calls = 0
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    if (!url.endsWith('/images')) return response(postA)
    calls++; return calls === 1 ? response({ error: 'Image rejected' }, 400) : response({ path: uploaded, url: `/media/${uploaded}`, width: 100, height: 100 })
  })); mount('/blog/article-fixture-a'); await screen.findByRole('textbox', { name: '标题' })
  change('标题', 'Unsaved title'); const editor = screen.getByRole('textbox', { name: '文章正文' }); editor.innerHTML = '<p>Unsaved body</p>'; fireEvent.input(editor)
  const input = screen.getByLabelText('上传封面图片'), file = new File(['fixture'], 'image.png', { type: 'image/png' })
  fireEvent.change(input, { target: { files: [file] } }); await screen.findByText('Image rejected'); expect(screen.getByAltText('封面预览').getAttribute('src')).toBe(`/media/${cover}`)
  fireEvent.change(input, { target: { files: [file] } }); await waitFor(() => expect(screen.getByAltText('封面预览').getAttribute('src')).toBe(`/media/${uploaded}`))
  expect((screen.getByRole('textbox', { name: '标题' }) as HTMLInputElement).value).toBe('Unsaved title'); expect(editor.textContent).toBe('Unsaved body')
})
it('快速从 A 切换 B，迟到的 A 响应不能覆盖 B 的正文或身份', async () => {
  const pending = deferred<Response>()
  vi.stubGlobal('fetch', vi.fn(async (url: string) => url.endsWith('/article-fixture-a') ? pending.promise : response(postB)))
  mount('/blog/article-fixture-a'); await userEvent.click(await screen.findByRole('button', { name: '测试切到 B' }))
  const editor = await screen.findByRole('textbox', { name: '文章正文' }); expect(editor.textContent).toBe('Body B')
  await act(async () => pending.resolve(response(postA)))
  expect(editor.textContent).toBe('Body B'); expect((screen.getByRole('textbox', { name: '标题' }) as HTMLInputElement).value).toBe(postB.title)
})
it('详情返回错误对象或加载失败时不呈现空的可保存文章，重试恢复', async () => {
  let calls = 0
  vi.stubGlobal('fetch', vi.fn(async () => { calls++; return response(calls === 1 ? postB : postA) })); mount('/blog/article-fixture-a')
  await screen.findByText('文章详情与所选文章不一致'); expect(screen.queryByRole('button', { name: '保存文章' })).toBeNull()
  await userEvent.click(screen.getByRole('button', { name: /重\s*试/ })); expect((await screen.findByRole('textbox', { name: '标题' }) as HTMLInputElement).value).toBe(postA.title)
})
it('保存遇到 401 立即清理文章与会话，不带正文重新请求', async () => {
  const fetch = vi.fn(async (_url: string, options: RequestInit) => options.method === 'PUT' ? response({ error: 'Session expired' }, 401) : response(postA)); vi.stubGlobal('fetch', fetch)
  const controller = mount('/blog/article-fixture-a'); await screen.findByRole('textbox', { name: '标题' }); submit()
  await waitFor(() => expect(controller.getSnapshot().phase).toBe('login')); expect(screen.queryByRole('textbox', { name: '文章正文' })).toBeNull(); expect(fetch).toHaveBeenCalledTimes(2)
})
