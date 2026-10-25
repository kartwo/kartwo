// 商品新建与编辑 / Product Create and Edit
// 功能：组合按权限独立的基本信息、矩阵、变体、图片；保护创建重试和路由读取生命周期
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:22:28

import { useCallback, useState } from 'react'
import { Alert, App, Button, Card } from 'antd'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { catalogAPI, type Category } from '../catalog-api'
import { productAPI, type Product } from '../product-api'
import { productRights, parseAxes, quantityInteger } from '../product-model'
import { moneyToCents } from '../money'
import { ProductFields, type ProductFormValues } from '../ProductFields'
import { ProductOperations } from '../ProductOperations'
import { ExistingVariants, NewVariants, initialAxes, type Matrix } from '../ProductVariants'
import { ProductMedia } from '../ProductMedia'
import { useSession } from '../SessionProvider'
import { useResource, useTask } from '../useResource'
import { PageState } from '../PageState'
interface Loaded { product: Product | null; categories: Category[]; demoCount: number }
function Editor({ loaded }: { loaded: Loaded }) {
  const { state } = useSession(), rights = productRights(state.session, loaded.product), task = useTask(), navigate = useNavigate(), { message } = App.useApp()
  const [matrix, setMatrix] = useState<Matrix>({ axes: initialAxes.map(axis => ({ ...axis })), rows: [], signature: '' })
  async function save(values: ProductFormValues) {
    if (!rights.basic) return
    await task.run(async () => {
      if (loaded.product) { await productAPI.update(loaded.product.public_id, values); return '' }
      const options = parseAxes(matrix.axes)
      if (!matrix.rows.length) throw new Error('请先生成变体组合并填写价格')
      if (matrix.signature !== JSON.stringify(options)) throw new Error('变体轴已改变，请重新生成组合后再保存')
      return productAPI.create({ ...values, options, variants: matrix.rows.map((row, index) => {
        try { return { sku: row.sku, price_cents: moneyToCents(row.price), quantity: quantityInteger(row.quantity), selections: row.selections } }
        catch (failure) { throw new Error(`第 ${index + 1} 个变体：${failure instanceof Error ? failure.message : '价格或库存无效'}`) }
      }) })
    }, id => { void message.success(loaded.product ? '基本信息已保存' : '商品已创建'); if (id) void navigate(`/products/${id}`, { replace: true }) })
  }
  const max = state.session?.demo_max_products ?? 0
  const quotaFull = rights.demo && !loaded.product && (!Number.isSafeInteger(max) || max <= 0 || loaded.demoCount >= max)
  if (quotaFull) return <Alert type="info" title="本次演示的临时商品额度已满，请返回列表删除自己的临时商品后重试。" action={<Link to="/products">返回商品列表</Link>} />
  return <ProductOperations.Provider value={task}><div className="read-page">
    <div><Button disabled={task.busy} onClick={() => void navigate('/products')}>返回商品列表</Button></div>
    {rights.protectedDemo ? <Alert type="info" title="这是受保护的示例商品。公开演示可以查看，不能修改或删除。" /> : rights.demo && <Alert type="info" title="临时商品只在本次会话中保留，始终为草稿，不会出现在顾客店面。" />}
    {task.error && <Alert type="error" showIcon title={task.error} />}
    {task.busy && <div role="status">正在保存、翻译或处理图片，请稍候…</div>}
    <Card title={loaded.product ? '商品基本信息' : '新建商品'}><ProductFields product={loaded.product} categories={loaded.categories} save={save}>{!loaded.product && <NewVariants matrix={matrix} change={setMatrix} />}</ProductFields></Card>
    {loaded.product && <><ExistingVariants product={loaded.product} /><ProductMedia product={loaded.product} /></>}
  </div></ProductOperations.Provider>
}
function Loader({ id }: { id: string }) {
  const { state } = useSession(), demo = state.session?.role === 'demo'
  const resource = useResource(useCallback(async (signal: AbortSignal): Promise<Loaded> => {
    const [categories, product, products] = await Promise.all([catalogAPI.categories(signal), id ? productAPI.get(id, signal) : null, !id && demo ? productAPI.list(signal) : null])
    return { categories, product, demoCount: products?.filter(item => item.demo_owned).length ?? 0 }
  }, [id, demo]))
  return <PageState {...resource} retry={resource.refresh}>{resource.data && <Editor loaded={resource.data} />}</PageState>
}
export default function ProductEdit() {
  const { pathname } = useLocation(), id = pathname === '/products/new' ? '' : pathname.slice('/products/'.length)
  return <Loader key={id || 'new'} id={id} />
}
