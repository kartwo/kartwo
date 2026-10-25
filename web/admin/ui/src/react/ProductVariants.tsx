// 商品变体编辑 / Product Variant Editing
// 功能：变体组合生成与失效校验、逐行精确价量保存，区分价格库存权限及部分成功
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:22:28

import { useState } from 'react'
import { Alert, App, Button, Card, Empty, Form, Input, Modal, Space, Table, Typography } from 'antd'
import type { Product, Variant } from './product-api'
import { generateVariants, parseAxes, productRights, quantityInteger, saveVariantChanges, type AxisDraft, type DraftVariant } from './product-model'
import { centsToMoney, moneyToCents } from './money'
import { useSession } from './SessionProvider'
import { useProductOperations } from './ProductOperations'
export interface Matrix { axes: AxisDraft[]; rows: DraftVariant[]; signature: string }
export const initialAxes: AxisDraft[] = [{ name: '尺码', valuesText: 'S, M, L' }, { name: '颜色', valuesText: '黑, 白' }]
export function NewVariants({ matrix, change }: { matrix: Matrix; change: (matrix: Matrix) => void }) {
  const task = useProductOperations(), [confirming, setConfirming] = useState(false), [error, setError] = useState('')
  function generate() {
    try { const options = parseAxes(matrix.axes); change({ ...matrix, rows: generateVariants(options), signature: JSON.stringify(options) }); setConfirming(false); setError('') }
    catch (failure) { setError(failure instanceof Error ? failure.message : '无法生成变体组合') }
  }
  function updateAxis(index: number, values: Partial<AxisDraft>) { change({ ...matrix, axes: matrix.axes.map((axis, i) => i === index ? { ...axis, ...values } : axis) }) }
  function updateRow(index: number, values: Partial<DraftVariant>) { change({ ...matrix, rows: matrix.rows.map((row, i) => i === index ? { ...row, ...values } : row) }) }
  let stale = false
  try { stale = !!matrix.rows.length && matrix.signature !== JSON.stringify(parseAxes(matrix.axes)) } catch { stale = !!matrix.rows.length }
  return <div className="product-matrix">
    <Typography.Title level={4}>变体轴</Typography.Title>
    {matrix.axes.map((axis, index) => <div className="product-axis" key={index}><Input aria-label={`轴 ${index + 1} 名称`} value={axis.name} disabled={task.busy} onChange={event => updateAxis(index, { name: event.target.value })} /><Input aria-label={`轴 ${index + 1} 取值`} placeholder="以英文逗号分隔" value={axis.valuesText} disabled={task.busy} onChange={event => updateAxis(index, { valuesText: event.target.value })} /><Button danger disabled={task.busy} aria-label={`删除轴 ${index + 1}`} onClick={() => change({ ...matrix, axes: matrix.axes.filter((_, i) => i !== index) })}>删除</Button></div>)}
    <Space wrap><Button disabled={task.busy} onClick={() => change({ ...matrix, axes: [...matrix.axes, { name: '', valuesText: '' }] })}>添加轴</Button><Button disabled={task.busy} onClick={() => { setError(''); if (matrix.rows.length) setConfirming(true); else generate() }}>生成变体组合</Button></Space>
    {error && !confirming && <Alert type="error" title={error} />}{stale && <Alert type="warning" title="变体轴已改变，请重新生成组合后再保存；现有输入尚未清空。" />}
    {matrix.rows.length > 0 && <Table<DraftVariant & { index: number }> aria-label="新商品变体" rowKey="index" size="small" scroll={{ x: true }} pagination={{ pageSize: 20, showSizeChanger: false }} dataSource={matrix.rows.map((row, index) => ({ ...row, index }))} columns={[
      { title: '组合', key: 'selection', render: (_, row) => row.selections.map(item => `${item.option}=${item.value}`).join(' × ') },
      { title: 'SKU', key: 'sku', render: (_, row) => <Input aria-label={`变体 ${row.index + 1} SKU`} value={row.sku} disabled={task.busy} onChange={event => updateRow(row.index, { sku: event.target.value })} /> },
      { title: '价格（主币种单位）', key: 'price', render: (_, row) => <Input inputMode="decimal" aria-label={`变体 ${row.index + 1} 价格`} placeholder="必填，可为 0" value={row.price} disabled={task.busy} onChange={event => updateRow(row.index, { price: event.target.value })} /> },
      { title: '库存', key: 'quantity', render: (_, row) => <Input inputMode="numeric" aria-label={`变体 ${row.index + 1} 库存`} value={row.quantity} disabled={task.busy} onChange={event => updateRow(row.index, { quantity: event.target.value })} /> },
    ]} />}
    <Modal open={confirming} title="重新生成变体" okText="重新生成" cancelText="取消" onOk={generate} onCancel={() => setConfirming(false)}>{error && <Alert type="error" title={error} />}重新生成将清空当前组合中填写的 SKU、价格和库存。</Modal>
  </div>
}
function VariantRow({ variant, price, inventory }: { variant: Variant; price: boolean; inventory: boolean }) {
  const task = useProductOperations(), { message } = App.useApp(), [form] = Form.useForm<{ price: string; quantity: string }>()
  const [saved, setSaved] = useState(variant), [warning, setWarning] = useState('')
  function save(values: { price: string; quantity: string }) {
    if (!price && !inventory) return
    void task.run(async () => saveVariantChanges(saved, price ? moneyToCents(values.price) : undefined, inventory ? quantityInteger(values.quantity) : undefined), result => {
      setSaved(current => ({ ...current, price_cents: result.price ?? current.price_cents, quantity: result.quantity ?? current.quantity })); setWarning(result.warning)
      if (!result.warning) void message.success(result.price === undefined && result.quantity === undefined ? '此行没有修改' : '变体修改已保存')
    })
  }
  return <div className="product-variant-row" data-variant={variant.public_id}>
    <div><strong>{variant.options.map(option => `${option.name}=${option.value}`).join(' × ') || '默认变体'}</strong><Typography.Paragraph type="secondary">SKU：{variant.sku || '—'}</Typography.Paragraph></div>
    <Form form={form} name={`variant-${variant.public_id}`} layout="vertical" initialValues={{ price: centsToMoney(variant.price_cents), quantity: String(variant.quantity) }} onFinish={save}>
      <Form.Item name="price" label="价格（主币种单位）"><Input aria-label={`价格：${variant.public_id}`} inputMode="decimal" disabled={!price || task.busy} /></Form.Item>
      <Form.Item name="quantity" label="库存"><Input aria-label={`库存：${variant.public_id}`} inputMode="numeric" disabled={!inventory || task.busy} /></Form.Item>
      {(price || inventory) && <Button disabled={task.busy} htmlType="submit" aria-label={`保存变体：${variant.public_id}`}>保存此行</Button>}
    </Form>
    {warning && <Alert type="warning" showIcon title={warning} />}
  </div>
}
export function ExistingVariants({ product }: { product: Product }) {
  const { state } = useSession(), rights = productRights(state.session, product)
  return <Card title="变体与库存"><Typography.Paragraph type="secondary">仅保存当前行有修改且有权限的字段。价格和库存分别保存；如部分成功，会明确提示。修改库存是设置实存数量，并非追加数量。</Typography.Paragraph>{!product.variants.length && <Empty description="没有变体" />}{product.variants.map(variant => <VariantRow key={variant.public_id} variant={variant} price={rights.price} inventory={rights.inventory} />)}</Card>
}
