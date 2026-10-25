// 内容页面管理 / Content Pages Management
// 功能：固定页面新建编辑发布与删除，保留 Markdown 原文和固定链接，保护读取切换竞态
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:34:25

import { useCallback, useEffect, useRef, useState } from 'react'
import { Alert, App, Button, Card, Empty, Form, Input, Modal, Select, Tag, Typography } from 'antd'
import { catalogAPI, emptyContent, writeAndReload, type ContentFields, type ContentPage } from '../catalog-api'
import { useResource, useTask } from '../useResource'
import { useSession } from '../SessionProvider'
import { can } from '../access'
import { PageState } from '../PageState'
function ContentEditor({ page, writable, busy, save }: { page: ContentPage | null; writable: boolean; busy: boolean; save: (values: ContentFields) => void }) {
  const [form] = Form.useForm<ContentFields>()
  useEffect(() => { form.setFieldsValue(page ?? emptyContent) }, [page, form])
  return <Card title={page ? '编辑内容页' : '新建内容页'}>
    <Form name="content-page" form={form} layout="vertical" disabled={!writable || busy} onFinish={values => { if (writable) save(values) }}>
      <Form.Item name="title" label="标题" rules={[{ required: true, whitespace: true, message: '请输入标题' }]}><Input /></Form.Item>
      <Form.Item name="slug" label="链接 slug" extra={page ? '保存后的链接固定。' : '保存后链接固定，避免已有链接失效。'} rules={[{ required: true, whitespace: true, message: '请输入链接 slug' }]}><Input disabled={!!page || !writable || busy} /></Form.Item>
      <Form.Item name="seo_description" label="SEO 描述"><Input.TextArea rows={2} /></Form.Item>
      <Form.Item name="body_markdown" label="内容（安全 Markdown）"><Input.TextArea rows={12} /></Form.Item>
      <Form.Item name="status" label="状态"><Select options={[{ value: 'draft', label: '草稿' }, { value: 'active', label: '发布' }]} /></Form.Item>
      {writable && <Button type="primary" htmlType="submit" loading={busy}>保存内容页</Button>}
    </Form>
  </Card>
}
export default function Merchandising() {
  const { state } = useSession(), resource = useResource(catalogAPI.pages), task = useTask(), { message } = App.useApp()
  const writable = can(state.session, 'content.manage'), lock = useRef(false)
  const [selected, setSelected] = useState(''), [newPage, setNewPage] = useState<ContentPage | null>(null)
  const [deleting, setDeleting] = useState<ContentPage | null>(null), [warning, setWarning] = useState('')
  const detail = useResource(useCallback((signal: AbortSignal) => selected ? catalogAPI.page(selected, signal) : Promise.resolve(null), [selected]))
  const page = selected ? detail.data?.public_id === selected ? detail.data : null : newPage
  function open(id: string) { if (!lock.current) { task.clearError(); if (id === selected) void detail.refresh(); else setSelected(id) } }
  function reset() { if (!lock.current) { setSelected(''); setNewPage(null); task.clearError() } }
  async function save(values: ContentFields) {
    if (!writable || lock.current || (selected && !page)) return
    lock.current = true
    try {
      await task.run(async () => { setWarning(''); return writeAndReload(() => page ? catalogAPI.savePage(page.public_id, values).then(() => page.public_id) : catalogAPI.createPage(values), catalogAPI.pages) }, result => {
        const saved: ContentPage = { ...values, public_id: result.value, updated_at: page?.updated_at ?? '' }
        if (selected) detail.setData(saved); else setNewPage(saved)
        resource.setData(current => result.data ?? (page ? (current ?? []).map(item => item.public_id === page.public_id ? saved : item) : [...(current ?? []), saved]))
        setWarning(result.warning); void message.success('内容页已保存')
      })
    } finally { lock.current = false }
  }
  async function remove() {
    if (!writable || !deleting || lock.current) return
    const id = deleting.public_id
    lock.current = true
    try {
      await task.run(() => writeAndReload(() => catalogAPI.deletePage(id), catalogAPI.pages), result => {
        resource.setData(current => result.data ?? (current ?? []).filter(item => item.public_id !== id)); setDeleting(null); setWarning(result.warning)
        if (page?.public_id === id) { setSelected(''); setNewPage(null) }
        void message.success('内容页已删除')
      })
    } finally { lock.current = false }
  }
  return <div className="read-page">
    <Typography.Paragraph type="secondary">内容页显示在店面页脚。支持安全 Markdown 的标题、列表和段落；HTML 不会执行。</Typography.Paragraph>
    {!writable && <Alert type="info" title="公开演示中内容页为只读，可选择页面查看原文。" />}
    {task.error && !deleting && <Alert type="error" showIcon title={task.error} />}{warning && <Alert type="warning" title={warning} />}
    <PageState {...resource} retry={resource.refresh}>{resource.data && <div className="content-columns"><Card title="内容页">
      {writable && <Button type="primary" disabled={task.busy} onClick={reset}>新建内容页</Button>}
      {!resource.data.length && <Empty description="尚无内容页" />}
      {resource.data.map(item => <div className="content-page-row" key={item.public_id}><Button className="content-title-button" type="text" disabled={task.busy} onClick={() => open(item.public_id)}>{item.title}</Button><Tag>{item.status === 'active' ? '已发布' : '草稿'}</Tag>{writable && <Button danger aria-label={`删除内容页：${item.title}`} disabled={task.busy} onClick={() => { task.clearError(); setDeleting(item) }}>删除</Button>}</div>)}
    </Card><div>{selected ? <PageState {...detail} retry={detail.refresh}>{page && <ContentEditor key={page.public_id} page={page} writable={writable} busy={task.busy} save={values => void save(values)} />}</PageState> : <ContentEditor key={newPage?.public_id ?? 'new'} page={newPage} writable={writable} busy={task.busy} save={values => void save(values)} />}</div></div>}</PageState>
    <Modal open={!!deleting} title="删除内容页" okText="删除" cancelText="取消" confirmLoading={task.busy} okButtonProps={{ danger: true }} onCancel={() => { if (!task.busy) setDeleting(null) }} onOk={() => void remove()}>{task.error && <Alert type="error" showIcon title={task.error} />}确定删除「{deleting?.title}」？</Modal>
  </div>
}
