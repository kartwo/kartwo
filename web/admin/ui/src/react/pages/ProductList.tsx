// 商品列表 / Product List
// 功能：按权限管理商品、精选与确认删除，显示当前演示会话的临时商品额度
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:22:28

import { useState } from 'react'
import { Alert, App, Button, Card, Modal, Space, Table, Tag, Typography } from 'antd'
import { Link } from 'react-router-dom'
import { productAPI, type ProductSummary } from '../product-api'
import { can } from '../access'
import { writeAndReload } from '../catalog-api'
import { useSession } from '../SessionProvider'
import { useResource, useTask } from '../useResource'
import { PageState } from '../PageState'
export default function ProductList() {
  const { state } = useSession(), resource = useResource(productAPI.list), task = useTask(), { message } = App.useApp()
  const demo = state.session?.role === 'demo', count = resource.data?.filter(item => item.demo_owned).length ?? 0, max = state.session?.demo_max_products ?? 0
  const create = demo || can(state.session, 'products.create'), quotaFull = demo && (!Number.isSafeInteger(max) || max <= 0 || count >= max)
  const [deleting, setDeleting] = useState<ProductSummary | null>(null), [warning, setWarning] = useState('')
  const canDelete = (item: ProductSummary) => demo ? item.demo_owned : can(state.session, 'products.delete')
  function remove() {
    if (!deleting || !canDelete(deleting)) return
    const id = deleting.public_id
    void task.run(() => writeAndReload(() => productAPI.remove(id), productAPI.list), result => { resource.setData(current => result.data ?? (current ?? []).filter(item => item.public_id !== id)); setDeleting(null); setWarning(result.warning); void message.success('商品已删除') })
  }
  function featured(item: ProductSummary) {
    if (demo || !can(state.session, 'products.publish') || item.status !== 'active') return
    void task.run(() => productAPI.featured(item.public_id, !item.featured), () => { resource.setData(current => (current ?? []).map(row => row.public_id === item.public_id ? { ...row, featured: !item.featured } : row)); void message.success(item.featured ? '已取消首页精选' : '已设为首页精选') })
  }
  return <div className="read-page"><div className="read-page-toolbar"><div><Typography.Paragraph type="secondary">草稿与归档商品不会出现在店面。上架后可设置为首页精选。</Typography.Paragraph>{demo && <Typography.Text>临时商品 {count} / {max}</Typography.Text>}</div>{create && (!quotaFull && !task.busy && !resource.loading && !resource.error ? <Link to="/products/new"><Button type="primary">新建商品</Button></Link> : <Button type="primary" disabled>新建商品</Button>)}</div>
    {task.error && !deleting && <Alert type="error" title={task.error} />}{warning && <Alert type="warning" title={warning} />}
    <PageState {...resource} retry={resource.refresh}>{resource.data && <Card><Table<ProductSummary> rowKey="public_id" size="middle" scroll={{ x: true }} dataSource={resource.data} locale={{ emptyText: '还没有商品' }} pagination={{ pageSize: 20, showSizeChanger: false }} columns={[
      { title: '标题', key: 'title', render: (_, item) => <><Link to={`/products/${item.public_id}`}>{item.title}</Link> {demo && item.demo_owned && <Tag>我的临时商品</Tag>}</> },
      { title: '链接', dataIndex: 'slug' },
      { title: '状态', key: 'status', render: (_, item) => <Tag>{item.status === 'active' ? '上架' : item.status === 'archived' ? '归档' : '草稿 · 店面看不到'}</Tag> },
      { title: '操作', key: 'actions', render: (_, item) => <Space wrap>{!demo && can(state.session, 'products.publish') && <Button disabled={task.busy || item.status !== 'active'} aria-label={`${item.featured ? '取消' : '设置'}精选：${item.title}`} onClick={() => featured(item)}>{item.featured ? '取消精选' : '设为精选'}</Button>}{canDelete(item) && <Button danger disabled={task.busy} aria-label={`删除商品：${item.title}`} onClick={() => { task.clearError(); setDeleting(item) }}>删除</Button>}{demo && !item.demo_owned && <Typography.Text type="secondary">只读</Typography.Text>}</Space> },
    ]} /></Card>}</PageState>
    <Modal open={!!deleting} title="删除商品" okText="删除" cancelText="取消" confirmLoading={task.busy} okButtonProps={{ danger: true }} onOk={remove} onCancel={() => { if (!task.busy) setDeleting(null) }}>{task.error && <Alert type="error" title={task.error} />}确定删除商品「{deleting?.title}」？删除后不能从后台恢复。</Modal>
  </div>
}
