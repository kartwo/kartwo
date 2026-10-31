// 博客列表 / Blog Post List
// 功能：独立列出草稿和发布文章、授权新建编辑及确认删除，演示不加载真实文章
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:56:32

import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'
import { useCallback, useState } from 'react'
import { Alert, App, Button, Card, Empty, Modal, Tag, Typography } from 'antd'
import { Link } from 'react-router-dom'
import { blogAPI, type BlogPost } from '../blog-api'
import { writeAndReload } from '../catalog-api'
import { can } from '../access'
import { useSession } from '../SessionProvider'
import { useResource, useTask } from '../useResource'
import { PageState } from '../PageState'
export default function BlogPosts() {
  const { state } = useSession(), demo = state.session?.role === 'demo', writable = can(state.session, 'blog.manage')
  const resource = useResource(useCallback((signal: AbortSignal) => demo ? Promise.resolve([]) : blogAPI.list(signal), [demo])), task = useTask(), { message } = App.useApp()
  const [deleting, setDeleting] = useState<BlogPost | null>(null), [warning, setWarning] = useState('')
  function remove() {
    if (!writable || !deleting) return
    const id = deleting.public_id
    void task.run(() => writeAndReload(() => blogAPI.remove(id), blogAPI.list), result => {
      resource.setData(current => result.data ?? (current ?? []).filter(item => item.public_id !== id)); setDeleting(null); setWarning(result.warning); void message.success('文章已删除')
    })
  }
  return <div className="read-page">
    <div className="read-page-toolbar"><Typography.Paragraph type="secondary">点击文章标题进入独立编辑页面。先保存草稿，确认内容后再发布。</Typography.Paragraph>{writable && <Link to="/blog/new"><Button icon={<PlusOutlined aria-hidden />} type="primary" disabled={task.busy}>新建文章</Button></Link>}</div>
    {demo && <Alert type="info" title="公开演示中不显示真实文章草稿。" />}{warning && <Alert type="warning" title={warning} />}
    <PageState {...resource} retry={resource.refresh}>{resource.data && <Card title="文章列表">
      {!resource.data.length && <Empty description="还没有文章" />}
      {resource.data.map(post => <div className="content-page-row" key={post.public_id}><Link className="blog-title-link" to={`/blog/${post.public_id}`}>{post.title}</Link><Tag>{post.status === 'active' ? '已发布' : '草稿'}</Tag>{writable && <Button icon={<DeleteOutlined aria-hidden />} danger disabled={task.busy} aria-label={`删除文章：${post.title}`} onClick={() => { task.clearError(); setDeleting(post) }}>删除</Button>}</div>)}
    </Card>}</PageState>
    <Modal open={!!deleting} title="删除文章" okText="删除" cancelText="取消" confirmLoading={task.busy} okButtonProps={{ danger: true }} onCancel={() => { if (!task.busy) setDeleting(null) }} onOk={remove}>{task.error && <Alert type="error" title={task.error} />}确定删除「{deleting?.title}」？</Modal>
  </div>
}
