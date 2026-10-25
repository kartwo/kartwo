// 商品图片管理 / Product Media Management
// 功能：本地安全缩略图、上传与 alt 编辑和确认删除，保留成功结果与演示额度
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:22:28

import { useCallback, useEffect, useState } from 'react'
import { Alert, App, Button, Card, Empty, Form, Input, Modal, Typography } from 'antd'
import { productAPI, type Product, type ProductMedia as Asset } from './product-api'
import { productRights } from './product-model'
import { writeAndReload } from './catalog-api'
import { useSession } from './SessionProvider'
import { useResource } from './useResource'
import { useProductOperations } from './ProductOperations'
import { PageState } from './PageState'
function MediaCard({ asset, writable, busy, save, remove }: { asset: Asset; writable: boolean; busy: boolean; save: (text: string) => void; remove: () => void }) {
  const [form] = Form.useForm<{ alt_text: string }>()
  useEffect(() => { form.setFieldValue('alt_text', asset.alt_text) }, [asset.alt_text, form])
  const thumbnail = asset.derivatives.find(item => item.label === 'thumb') ?? asset.derivatives[0]
  return <Card size="small"><img className="product-media-preview" src={thumbnail?.url ?? asset.original_url} alt={asset.alt_text || '商品图片预览'} />
    <Form form={form} layout="vertical" onFinish={values => { if (writable) save(values.alt_text) }} disabled={!writable || busy}>
      <Form.Item name="alt_text" label="图片 alt（英文）"><Input aria-label={`图片说明：${asset.public_id}`} maxLength={250} /></Form.Item>
      {writable && <div className="product-media-actions"><Button htmlType="submit" disabled={busy}>保存 alt</Button><Button danger aria-label={`删除图片：${asset.public_id}`} disabled={busy} onClick={remove}>删除图片</Button></div>}
    </Form>
  </Card>
}
export function ProductMedia({ product }: { product: Product }) {
  const { state } = useSession(), rights = productRights(state.session, product), task = useProductOperations(), { message } = App.useApp()
  const resource = useResource(useCallback((signal: AbortSignal) => productAPI.media(product.public_id, signal), [product.public_id]))
  const [warning, setWarning] = useState(''), [deleting, setDeleting] = useState<Asset | null>(null)
  const demoMax = state.session?.demo_max_image_bytes ?? 0, maxBytes = rights.demo ? Number.isSafeInteger(demoMax) && demoMax > 0 ? demoMax : 0 : 10 << 20
  const quotaFull = rights.demo && (resource.data?.length ?? 0) >= 1
  function upload(file: File) {
    if (!rights.media || !resource.data || quotaFull || !maxBytes) return
    void task.run(() => writeAndReload(() => productAPI.upload(product.public_id, file, maxBytes), () => productAPI.media(product.public_id)), result => {
      resource.setData(current => result.data ?? [...(current ?? []).filter(item => item.public_id !== result.value.public_id), result.value]); setWarning(result.warning); void message.success('图片已上传')
    })
  }
  function saveAlt(asset: Asset, text: string) {
    if (!rights.media) return
    void task.run(() => productAPI.alt(asset.public_id, text), () => { resource.setData(current => (current ?? []).map(item => item.public_id === asset.public_id ? { ...item, alt_text: text.trim() } : item)); void message.success('图片 alt 已保存') })
  }
  function remove() {
    if (!rights.media || !deleting) return
    const id = deleting.public_id
    void task.run(() => writeAndReload(() => productAPI.deleteMedia(id), () => productAPI.media(product.public_id)), result => {
      resource.setData(current => result.data ?? (current ?? []).filter(item => item.public_id !== id)); setDeleting(null); setWarning(result.warning); void message.success('图片已删除')
    })
  }
  return <Card title="图片">
    <Typography.Paragraph type="secondary">{rights.demo ? '演示商品最多 1 张图片，文件大小按当前会话额度限制。' : '支持 JPEG、PNG、WebP，单张最大 10MB。系统会压缩、生成多种尺寸并去除 EXIF。'}</Typography.Paragraph>
    {warning && <Alert type="warning" title={warning} />}
    <PageState {...resource} retry={resource.refresh}>{resource.data && <div className="read-page">
      {rights.media && <input type="file" aria-label="上传商品图片" accept="image/jpeg,image/png,image/webp" disabled={task.busy || quotaFull || !maxBytes} onChange={event => { const file = event.currentTarget.files?.[0]; event.currentTarget.value = ''; if (file) upload(file) }} />}
      {quotaFull && <Typography.Paragraph type="secondary">演示图片额度已满；删除当前图片后可重新上传。</Typography.Paragraph>}
      {!resource.data.length && <Empty description="还没有图片，选择一张上传。" />}
      <div className="product-media-grid">{resource.data.map(asset => <MediaCard key={asset.public_id} asset={asset} writable={rights.media} busy={task.busy} save={text => saveAlt(asset, text)} remove={() => { task.clearError(); setDeleting(asset) }} />)}</div>
    </div>}</PageState>
    <Modal open={!!deleting} title="删除商品图片" okText="删除" cancelText="取消" confirmLoading={task.busy} okButtonProps={{ danger: true }} onOk={remove} onCancel={() => { if (!task.busy) setDeleting(null) }}>{task.error && <Alert type="error" title={task.error} />}确定删除这张图片？</Modal>
  </Card>
}
