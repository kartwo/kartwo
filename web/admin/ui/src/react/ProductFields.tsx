// 商品基本信息 / Product Content Fields
// 功能：中英文辅助内容、手动翻译、固定链接、分类和上架状态，按字段权限保护编辑
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:22:28

import { useRef, useState, type ReactNode } from 'react'
import { Alert, App, Button, Form, Input, Select, Typography } from 'antd'
import { Link } from 'react-router-dom'
import type { Category } from './catalog-api'
import { emptyProduct, productAPI, type Product, type ProductFields as Fields } from './product-api'
import { productRights, productSlug } from './product-model'
import { useSession } from './SessionProvider'
import { useProductOperations } from './ProductOperations'
export type ProductFormValues = Fields & { slug: string; slug_zh: string }
export function ProductFields({ product, categories, save, children }: { product: Product | null; categories: Category[]; save: (values: ProductFormValues) => Promise<void>; children?: ReactNode }) {
  const { state } = useSession(), rights = productRights(state.session, product), task = useProductOperations(), { message } = App.useApp()
  const [form] = Form.useForm<ProductFormValues>(), customized = useRef(false), [needsTranslation, setNeedsTranslation] = useState(false)
  const status = Form.useWatch('status', form) as string | undefined
  const known = new Set(categories.map(category => category.public_id))
  const missing = (product?.category_public_ids ?? []).filter(id => !known.has(id))
  function suggest(title: string) { if (!product && !customized.current && !form.getFieldValue('slug_zh')?.trim()) form.setFieldValue('slug', productSlug(title)) }
  function translate(source: 'title_zh' | 'slug_zh' | 'seo_description_zh', destination: 'title' | 'slug' | 'seo_description') {
    if (!rights.translate || !rights.basic) return
    void task.run(() => productAPI.translate(form.getFieldValue(source) ?? ''), text => {
      setNeedsTranslation(text === null)
      if (text === null) return
      form.setFieldValue(destination, destination === 'slug' ? productSlug(text) : text)
      if (destination === 'title') suggest(text)
      if (destination === 'slug') customized.current = true
      void message.success('已翻译为英文，请确认后保存')
    })
  }
  return <Form name="product-fields" form={form} layout="vertical" initialValues={product ?? emptyProduct} disabled={!rights.basic || task.busy} onFinish={values => {
    if (!rights.basic) return
    void save({ ...values, category_public_ids: values.category_public_ids ?? [], status: rights.demo ? 'draft' : rights.publish ? values.status : product?.status ?? 'draft' })
  }}>
    <div className="settings-columns">
      <div><Form.Item name="title_zh" label="中文标题（可选）"><Input placeholder="例如：速干运动短袖" /></Form.Item>{rights.translate && <Button disabled={task.busy} onClick={() => translate('title_zh', 'title')}>翻译为英文标题</Button>}</div>
      <Form.Item name="title" label="英文标题" rules={[{ required: true, whitespace: true, message: '请输入英文标题' }]}><Input onChange={event => suggest(event.target.value)} /></Form.Item>
    </div>
    {!product ? <div className="settings-columns"><div><Form.Item name="slug_zh" label="中文 slug（可选）"><Input /></Form.Item>{rights.translate && <Button disabled={task.busy} onClick={() => translate('slug_zh', 'slug')}>翻译 slug</Button>}</div><Form.Item name="slug" label="英文 slug" extra="保存后链接固定。英文标题可自动生成链接；填写中文 slug 后请手动翻译或填写英文链接。" rules={[{ required: true, whitespace: true, message: '请输入英文 slug' }]}><Input onChange={() => { customized.current = true }} /></Form.Item></div>
      : <Form.Item name="slug" label="商品链接" extra="已保存商品的链接固定。"><Input disabled /></Form.Item>}
    {needsTranslation && <Alert type="warning" title="尚未设置翻译 API，无法生成英文内容。" action={<Link to="/translation">前往翻译服务设置</Link>} />}
    <Form.Item name="description" label="描述"><Input.TextArea rows={3} /></Form.Item>
    <Form.Item name="seo_description_zh" label="中文 SEO 描述（可选）"><Input.TextArea rows={2} /></Form.Item>
    {rights.translate && <Button disabled={task.busy} onClick={() => translate('seo_description_zh', 'seo_description')}>翻译为英文 SEO 描述</Button>}
    <Form.Item name="seo_description" label="英文 SEO 描述" extra="留空时使用英文描述。"><Input.TextArea rows={2} /></Form.Item>
    <Form.Item name="status" label="状态"><Select disabled={!rights.publish || !rights.basic || task.busy} options={[{ value: 'draft', label: '草稿' }, { value: 'active', label: '上架' }, { value: 'archived', label: '归档' }]} /></Form.Item>
    <Form.Item name="category_public_ids" label="商品分类"><Select mode="multiple" placeholder="选择分类，可多选" options={[...categories.map(category => ({ value: category.public_id, label: category.name })), ...missing.map(id => ({ value: id, label: `未出现在分类目录中的分类：${id}` }))]} /></Form.Item>
    <Typography.Paragraph type={status === 'draft' ? 'warning' : 'secondary'}>{status === 'active' ? '保存为上架后，顾客能在店面看到并下单。' : status === 'archived' ? '保存为归档后，商品不会出现在店面，记录仍会保留。' : '草稿不会出现在店面，顾客看不到、也买不了。状态修改需保存后生效。'}</Typography.Paragraph>
    {children}
    {rights.basic && <Button type="primary" htmlType="submit" loading={task.busy}>{product ? '保存基本信息' : rights.demo ? '保存临时草稿商品' : '保存商品'}</Button>}
  </Form>
}
