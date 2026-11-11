// 分类管理 / Category Management
// 功能：分类名称链接排序维护，按独立权限开放编辑并保留关联商品删除保护
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:34:25

import { DeleteOutlined, ReloadOutlined, SaveOutlined } from '@ant-design/icons'
import { useEffect, useRef, useState } from 'react'
import { Alert, App, Button, Card, Empty, Form, Input, InputNumber, Modal, Space, Tag, Typography } from 'antd'
import { catalogAPI, categorySlug, writeAndReload, type Category, type CategoryFields } from '../catalog-api'
import { useResource, useTask } from '../useResource'
import { useSession } from '../SessionProvider'
import { can } from '../access'
import { PageState } from '../PageState'
function CategoryForm({ category, writable, busy, save, remove }: { category?: Category; writable: boolean; busy: boolean; save: (values: CategoryFields, reset: (position: number) => void) => void; remove?: () => void }) {
  const [form] = Form.useForm<CategoryFields>(), automatic = useRef(true)
  const id = category?.public_id, name = category?.name, slug = category?.slug, position = category?.position
  useEffect(() => { if (id) form.setFieldsValue({ name, slug, position }) }, [id, name, slug, position, form])
  function reset(position: number) { automatic.current = true; form.resetFields(); form.setFieldValue('position', position) }
  return <Card title={category?.name || '新建分类'} extra={category && <Tag>{category.product_count} 件商品</Tag>}>
    <Form name={`category-${id ?? 'new'}`} form={form} layout="vertical" disabled={!writable || busy} initialValues={{ name: '', slug: '', position: 0 }} onValuesChange={(changed: Partial<CategoryFields>) => {
      if (id) return
      if ('slug' in changed) automatic.current = !changed.slug
      else if (changed.name !== undefined && automatic.current) form.setFieldValue('slug', categorySlug(changed.name))
    }} onFinish={values => { if (writable) save(values, reset) }}>
      <div className="category-fields"><Form.Item name="name" label="分类名称" rules={[{ required: true, whitespace: true, message: '请输入分类名称' }]}><Input /></Form.Item>
        <Form.Item name="slug" label="链接 slug" rules={[{ required: true, message: '请输入链接 slug' }, { transform: (value: string) => value.trim(), pattern: /^[a-z0-9]+(?:-[a-z0-9]+)*$/, message: '仅支持小写英文、数字及单个连字符分隔' }]}><Input placeholder="running-tops" /></Form.Item>
        <Form.Item name="position" label="排序" rules={[{ required: true, type: 'integer', min: 0, max: Number.MAX_SAFE_INTEGER, message: '排序须为非负安全整数' }]}><InputNumber min={0} max={Number.MAX_SAFE_INTEGER} /></Form.Item>
      </div>
      {writable && <Space wrap><Button icon={<SaveOutlined aria-hidden />} type="primary" htmlType="submit" loading={busy}>{category ? '保存分类' : '创建分类'}</Button>{category && <Button icon={<DeleteOutlined aria-hidden />} danger disabled={busy || category.product_count > 0} onClick={remove}>删除分类</Button>}</Space>}
      {!!category?.product_count && <Typography.Paragraph type="secondary">有关联商品，删除前须先将商品移到其他分类。</Typography.Paragraph>}
    </Form>
  </Card>
}
export default function CategoryManagement() {
  const { state } = useSession(), resource = useResource(catalogAPI.categories), task = useTask(), { message } = App.useApp()
  const writable = can(state.session, 'categories.manage'), [deleting, setDeleting] = useState<Category | null>(null), [warning, setWarning] = useState('')
  async function save(values: CategoryFields, category: Category | undefined, reset: (position: number) => void) {
    if (!writable) return
    await task.run(async () => { setWarning(''); return writeAndReload(() => category ? catalogAPI.saveCategory(category.public_id, values).then(() => category.public_id) : catalogAPI.createCategory(values), catalogAPI.categories) }, result => {
      const fallback: Category = { ...values, name: values.name.trim(), slug: values.slug.trim(), public_id: result.value, product_count: category?.product_count ?? 0 }
      resource.setData(current => result.data ?? (category ? (current ?? []).map(item => item.public_id === category.public_id ? fallback : item) : [...(current ?? []), fallback]))
      setWarning(result.warning)
      if (!category) reset(result.data?.length ?? (resource.data?.length ?? 0) + 1)
      void message.success(category ? '分类已保存' : '分类已创建')
    })
  }
  async function remove() {
    if (!writable || !deleting || deleting.product_count > 0) return
    const id = deleting.public_id
    await task.run(() => writeAndReload(() => catalogAPI.deleteCategory(id), catalogAPI.categories), result => {
      resource.setData(current => result.data ?? (current ?? []).filter(item => item.public_id !== id)); setDeleting(null); setWarning(result.warning); void message.success('分类已删除')
    })
  }
  return <div className="read-page">
    <Typography.Paragraph type="secondary">分类用于整理商品，并按排序显示在店面导航和分类页。删除前须先把关联商品移到其他分类。</Typography.Paragraph>
    {!writable && <Alert type="info" title="当前账号只允许查看分类；临时商品仍可选择这些分类。" />}
    {task.error && !deleting && <Alert type="error" showIcon title={task.error} />}
    {warning && <Alert type="warning" title={warning} />}
    <PageState {...resource} retry={resource.refresh}>{resource.data && <>
      {writable && <CategoryForm writable busy={task.busy} save={(values, reset) => void save(values, undefined, reset)} />}
      {!resource.data.length && <Empty description="尚无分类，请先新建一个" />}
      {resource.data.map(category => <CategoryForm key={category.public_id} category={category} writable={writable} busy={task.busy} save={(values, reset) => void save(values, category, reset)} remove={() => { task.clearError(); setDeleting(category) }} />)}
    </>}</PageState>
    <div><Button icon={<ReloadOutlined aria-hidden />} disabled={resource.loading || task.busy} onClick={() => { if (!task.busy) { setWarning(''); void resource.refresh() } }}>刷新分类</Button></div>
    <Modal open={!!deleting} title="删除分类" okText="删除" cancelText="取消" confirmLoading={task.busy} okButtonProps={{ danger: true }} onCancel={() => { if (!task.busy) setDeleting(null) }} onOk={() => void remove()}>
      {task.error && <Alert type="error" showIcon title={task.error} />}确定删除「{deleting?.name}」？有关联商品的分类仍由服务器保护，不能直接删除。
    </Modal>
  </div>
}
