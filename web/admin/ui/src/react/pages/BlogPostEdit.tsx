// 博客文章编辑 / Blog Post Editor
// 功能：独立新建编辑、标题生成固定链接、封面与正文上传、富文本与旧 Markdown 兼容
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:56:32

import { FilePicker } from '../FilePicker'
import { ArrowLeftOutlined, FormOutlined, SaveOutlined } from '@ant-design/icons'
import { useCallback, useLayoutEffect, useRef, useState } from 'react'
import { Alert, App, Button, Card, Form, Input, Select, Space, Typography } from 'antd'
import { useLocation, useNavigate } from 'react-router-dom'
import { blogAPI, emptyBlog, type BlogFields, type BlogPost } from '../blog-api'
import { uniqueBlogSlug } from '../../blogSlug.js'
import { RichTextEditor, type RichTextHandle } from '../RichTextEditor'
import { sanitizeRichHTML } from '../rich-html'
import { can } from '../access'
import { useSession } from '../SessionProvider'
import { useResource, useTask } from '../useResource'
import { PageState } from '../PageState'
interface Loaded { post: BlogPost | null; html: string; slugs: string[] }
function Editor({ loaded }: { loaded: Loaded }) {
  const { state } = useSession(), writable = can(state.session, 'blog.manage'), task = useTask(), navigate = useNavigate(), { message } = App.useApp()
  const [form] = Form.useForm<BlogFields>(), editor = useRef<RichTextHandle>(null), customized = useRef(false)
  const cover = Form.useWatch('cover_path', form) as string | undefined, title = Form.useWatch('title', form) as string | undefined, slug = Form.useWatch('slug', form) as string | undefined
  const lock = useRef(false)
  const [pendingImage, setPendingImage] = useState('')
  // Insert only after the upload lock releases and the editing host is writable again, preserving native undo.
  useLayoutEffect(() => { if (pendingImage && !task.busy && writable) { editor.current?.insertImage(pendingImage); setPendingImage('') } }, [pendingImage, task.busy, writable])
  function suggest(value: string) { if (!loaded.post && !customized.current) form.setFieldValue('slug', uniqueBlogSlug(value, loaded.slugs)) }
  async function upload(file: File, isCover: boolean) {
    if (!writable || lock.current) return
    lock.current = true
    try { await task.run(() => blogAPI.upload(file), image => { if (isCover) form.setFieldValue('cover_path', image.path); else setPendingImage(image.url); void message.success('图片已上传') }) }
    finally { lock.current = false }
  }
  async function save(values: BlogFields) {
    if (!writable || lock.current) return
    lock.current = true
    try {
      await task.run(async () => {
        if (!editor.current) throw new Error('正文编辑器尚未就绪')
        const post = { ...values, cover_path: values.cover_path ?? '', body_html: editor.current.getHTML(), body_markdown: '' }
        if (loaded.post) await blogAPI.update(loaded.post.public_id, post); else await blogAPI.create(post)
      }, () => { void message.success('文章已保存'); void navigate('/blog', { replace: true }) })
    } finally { lock.current = false }
  }
  return <Card title={loaded.post ? '编辑文章' : '新建文章'}>
    {!writable && <Alert type="info" title="当前账号仅可查看文章，保存、上传与排版操作不可用。" />}
    {loaded.post?.body_markdown && !loaded.post.body_html && <Alert type="info" title="这是旧 Markdown 文章，已转换为富文本供查看；保存时会改为富文本。" />}
    {task.error && <Alert type="error" showIcon title={task.error} />}
    <Form form={form} name="blog-post" layout="vertical" initialValues={loaded.post ?? emptyBlog} disabled={!writable || task.busy} onFinish={values => void save(values)}>
      <Form.Item label="标题" name="title" rules={[{ required: true, whitespace: true, message: '请输入文章标题' }]}><Input maxLength={160} onChange={event => suggest(event.target.value)} /></Form.Item>
      <Form.Item label="文章链接" required extra={loaded.post ? '保存后的链接固定。' : title && !slug ? '标题没有可用于链接的英文或数字，请手动填写英文链接。' : '按标题自动生成，保存前可修改；保存后固定，避免已有分享链接失效。'}>
        <div className="blog-slug-controls"><Form.Item name="slug" noStyle rules={[{ required: true, message: '请输入文章链接' }, { pattern: /^[a-z0-9]+(?:-[a-z0-9]+)*$/, message: '只能使用小写英文、数字和连字符' }]}><Input aria-label="文章链接" maxLength={100} disabled={!!loaded.post || !writable || task.busy} onChange={() => { customized.current = true }} /></Form.Item>{!loaded.post && writable && <Button icon={<FormOutlined aria-hidden />} onClick={() => { customized.current = false; suggest(form.getFieldValue('title') ?? '') }}>按标题生成</Button>}</div>
      </Form.Item>
      <Form.Item label="摘要" name="excerpt"><Input.TextArea maxLength={300} rows={3} /></Form.Item>
      <Form.Item label="SEO 描述" name="seo_description"><Input.TextArea maxLength={300} rows={2} /></Form.Item>
      <Form.Item label="封面图" name="cover_path" hidden><Input /></Form.Item>
      <Form.Item label="封面图片">
        {writable && <FilePicker label="上传封面图片" accept="image/jpeg,image/png,image/webp" disabled={task.busy} onChange={event => { const file = event.currentTarget.files?.[0]; event.currentTarget.value = ''; if (file) void upload(file, true) }} />}
        {cover ? <img className="blog-cover-preview" src={`/media/${cover}`} alt="封面预览" /> : <Typography.Text type="secondary">尚未设置封面，店面列表将使用正文第一张本地图片。</Typography.Text>}
      </Form.Item>
      <Form.Item label="正文" extra="选中文字后使用工具栏排版。粘贴仅保留文字；图片需上传，视频支持 YouTube 和 Vimeo 链接。">
        <RichTextEditor ref={editor} initialHTML={loaded.html} disabled={!writable || task.busy} upload={file => void upload(file, false)} />
      </Form.Item>
      {task.busy && <Typography.Paragraph role="status">正在保存或处理图片，请稍候…</Typography.Paragraph>}
      <Form.Item label="状态" name="status"><Select options={[{ value: 'draft', label: '草稿' }, { value: 'active', label: '发布' }]} /></Form.Item>
      <Space wrap><Button icon={<ArrowLeftOutlined aria-hidden />} disabled={task.busy} onClick={() => void navigate('/blog')}>返回文章列表</Button>{writable && <Button icon={<SaveOutlined aria-hidden />} type="primary" htmlType="submit" loading={task.busy}>保存文章</Button>}</Space>
    </Form>
  </Card>
}
function Loader({ id }: { id: string }) {
  const resource = useResource(useCallback(async (signal: AbortSignal): Promise<Loaded> => {
    if (!id) return { post: null, html: '', slugs: (await blogAPI.list(signal)).map(post => post.slug) }
    const post = await blogAPI.get(id, signal)
    const html = post.body_html || (post.body_markdown ? await blogAPI.preview(post.body_markdown, signal) : '')
    return { post, html: sanitizeRichHTML(html), slugs: [] }
  }, [id]))
  return <PageState {...resource} retry={resource.refresh}>{resource.data && <Editor loaded={resource.data} />}</PageState>
}
export default function BlogPostEdit() {
  const { state } = useSession(), { pathname } = useLocation(), id = pathname === '/blog/new' ? '' : pathname.slice('/blog/'.length)
  if (state.session?.role === 'demo') return <Alert type="info" title="公开演示不可查看真实文章或创建文章。" />
  return <div className="read-page"><Typography.Paragraph type="secondary">先保存草稿，确认内容后再发布。文章链接保存后保持不变。</Typography.Paragraph><Loader key={id || 'new'} id={id} /></div>
}
