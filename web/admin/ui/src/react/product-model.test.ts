// 商品契约与金额测试 / Product Contract and Amount Tests
// 功能：验证矩阵和接口边界、精确金额库存、独立权限及部分成功后只重试未完成字段
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:22:28

import { expect, it, vi } from 'vitest'
import { generateVariants, parseAxes, productRights, productSlug, quantityInteger, saveVariantChanges } from './product-model'
import { productAPI, parseProduct, parseProducts, parseMedia } from './product-api'
import { moneyToCents, centsToMoney } from './money'
import { APIError } from './transport'
import { account } from './test-fixtures'
import { assetA, productA, summary } from './product-fixtures'
function response(value: unknown, status = 200) { return new Response(JSON.stringify(value), { status }) }
it('商品契约拒绝重复对象、未知状态、不安全价量，丢弃内部字段', () => {
  expect(() => parseProducts({ products: [summary(productA), summary(productA)] })).toThrow()
  expect(() => parseProduct({ ...productA, status: 'unknown' })).toThrow()
  expect(() => parseProduct({ ...productA, variants: [{ ...productA.variants[0], quantity: 1.5 }] })).toThrow()
  expect(() => parseProduct({ ...productA, category_public_ids: ['../private'] })).toThrow()
  expect(parseProduct({ ...productA, internal_id: 3 })).not.toHaveProperty('internal_id')
})
it('媒体只允许本地内容哈希原图/派生图，拒绝外部 URL 和重复派生尺寸', () => {
  expect(parseMedia(assetA).derivatives[0]?.url).toContain('/media/derived/')
  expect(() => parseMedia({ ...assetA, original_url: 'https://outside.invalid/x.png' })).toThrow('图片地址无效')
  expect(() => parseMedia({ ...assetA, derivatives: [assetA.derivatives[0], assetA.derivatives[0]] })).toThrow()
  expect(() => parseMedia({ ...assetA, width: -1 })).toThrow()
})
it('标题 slug 保留原英文转换规则，空白价格不默认免费，金额不四舍五入', () => {
  expect(productSlug('Café & Running!')).toBe('cafe-running'); expect(productSlug('中文')).toBe(''); expect(productSlug('a'.repeat(80))).toHaveLength(72)
  expect(moneyToCents('0')).toBe(0); expect(moneyToCents('1.01')).toBe(101); expect(centsToMoney(101)).toBe('1.01')
  for (const value of ['', '-1', '1.005', '1e3', 'Infinity', '90071992547409.92']) expect(() => moneyToCents(value)).toThrow()
})
it('库存保留精确整数边界，空白/负数/小数/指数/溢出不可提交', () => {
  expect(quantityInteger('0')).toBe(0); expect(quantityInteger('9007199254740991')).toBe(Number.MAX_SAFE_INTEGER)
  for (const value of ['', '-1', '1.5', '1e3', 'NaN', '9007199254740992']) expect(() => quantityInteger(value)).toThrow()
})
it('矩阵逐轴生成所有组合、保留零库存和空价格，不隐藏重复或未填轴', () => {
  const options = parseAxes([{ name: ' Size ', valuesText: 'S, M' }, { name: 'Color', valuesText: 'Black,White' }])
  const rows = generateVariants(options); expect(rows).toHaveLength(4); expect(rows[3]?.selections).toEqual([{ option: 'Size', value: 'M' }, { option: 'Color', value: 'White' }]); expect(rows[0]?.price).toBe(''); expect(rows[0]?.quantity).toBe('0')
  for (const axes of [[], [{ name: '', valuesText: 'S' }], [{ name: 'Size', valuesText: 'S,S' }], [{ name: 'A', valuesText: 'X' }, { name: 'A', valuesText: 'Y' }]]) expect(() => parseAxes(axes)).toThrow()
})
it('矩阵保留既有多轴能力，提前拒绝必然超过后端 1MB 的组合而不分配巨大数组', () => {
  expect(generateVariants(parseAxes([{ name: 'A', valuesText: '1,2' }, { name: 'B', valuesText: '1,2' }, { name: 'C', valuesText: '1,2' }]))).toHaveLength(8)
  const values = Array.from({ length: 10000 }, (_, i) => String(i)).join(',')
  expect(() => generateVariants(parseAxes([{ name: 'A', valuesText: values }, { name: 'B', valuesText: values }]))).toThrow('1MB')
})
it('改价/改库存/图片分别授权，员工不能翻译；演示只能写自己商品', () => {
  const rights = productRights(account('staff', ['products.view', 'products.price']), productA)
  expect(rights.price).toBe(true); expect(rights.inventory).toBe(false); expect(rights.basic).toBe(false); expect(rights.media).toBe(false); expect(rights.translate).toBe(false)
  expect(productRights(account('demo'), productA).media).toBe(false)
  expect(productRights(account('demo'), { ...productA, demo_owned: true }).inventory).toBe(true)
  expect(productRights(account('demo'), null).publish).toBe(false)
})
it('现有商品保存白名单不发送 slug/变体/媒体元数据，整数价格库存逐接口提交', async () => {
  const fetch = vi.fn(async (_url: string, _options: RequestInit) => response({ ok: true })); vi.stubGlobal('fetch', fetch)
  await productAPI.update(productA.public_id, productA)
  expect(JSON.parse(fetch.mock.calls[0]![1].body as string)).toEqual({ title: productA.title, title_zh: productA.title_zh, description: productA.description, seo_description: productA.seo_description, seo_description_zh: productA.seo_description_zh, status: 'active', category_public_ids: productA.category_public_ids })
  expect(() => productAPI.price(productA.variants[0]!.public_id, 1.5)).toThrow(); expect(() => productAPI.inventory(productA.variants[0]!.public_id, Number.MAX_SAFE_INTEGER + 1)).toThrow(); expect(() => productAPI.remove('../private')).toThrow()
  expect(fetch).toHaveBeenCalledTimes(1)
})
it('新建剔除所有界面字段，保留零价和整数库存，超过现有请求体限制前拒绝', async () => {
  const fetch = vi.fn(async (_url: string, _options: RequestInit) => response({ public_id: productA.public_id }, 201)); vi.stubGlobal('fetch', fetch)
  const input = { ...productA, options: [{ name: 'Size', values: ['S'] }], variants: [{ sku: 'SKU', price_cents: 0, quantity: 2, selections: [{ option: 'Size', value: 'S' }], price: '0.00' }] }
  expect(await productAPI.create(input)).toBe(productA.public_id)
  const body = JSON.parse(fetch.mock.calls[0]![1].body as string); expect(body.variants[0]).toEqual({ sku: 'SKU', price_cents: 0, quantity: 2, selections: [{ option: 'Size', value: 'S' }] }); expect(body).not.toHaveProperty('public_id'); expect(body).not.toHaveProperty('featured')
  await expect(productAPI.create({ ...input, description: '中'.repeat(400000) })).rejects.toThrow('1MB'); expect(fetch).toHaveBeenCalledTimes(1)
})
it('只修改价格时不重写旧库存，不修改时不发请求', async () => {
  const price = vi.spyOn(productAPI, 'price').mockResolvedValue(), inventory = vi.spyOn(productAPI, 'inventory').mockResolvedValue()
  const variant = productA.variants[0]!
  expect(await saveVariantChanges(variant, 202, variant.quantity)).toEqual({ price: 202, warning: '' }); expect(price).toHaveBeenCalledTimes(1); expect(inventory).not.toHaveBeenCalled()
  await saveVariantChanges(variant, variant.price_cents, variant.quantity); expect(price).toHaveBeenCalledTimes(1)
})
it('价格确认成功但库存失败保留局部结果，按新基线重试只保存库存', async () => {
  const price = vi.spyOn(productAPI, 'price').mockResolvedValue(), inventory = vi.spyOn(productAPI, 'inventory').mockRejectedValueOnce(new APIError(500, 'Inventory failed')).mockResolvedValue()
  const variant = productA.variants[0]!, result = await saveVariantChanges(variant, 202, 7)
  expect(result.price).toBe(202); expect(result.quantity).toBeUndefined(); expect(result.warning).toContain('价格已保存，库存保存未确认')
  await saveVariantChanges({ ...variant, price_cents: 202 }, 202, 7); expect(price).toHaveBeenCalledTimes(1); expect(inventory).toHaveBeenCalledTimes(2)
})
it('价格失败不继续写库存，库存 401 仍失效会话而非当普通部分失败', async () => {
  const price = vi.spyOn(productAPI, 'price').mockRejectedValueOnce(new Error('Price failed')).mockResolvedValue(), inventory = vi.spyOn(productAPI, 'inventory').mockRejectedValue(new APIError(401, 'Session expired'))
  await expect(saveVariantChanges(productA.variants[0]!, 202, 7)).rejects.toThrow('Price failed'); expect(inventory).not.toHaveBeenCalled()
  await expect(saveVariantChanges(productA.variants[0]!, 202, 7)).rejects.toMatchObject({ status: 401 }); expect(price).toHaveBeenCalledTimes(2)
})
it('媒体大小/类型在发送前校验，翻译未配置只返回提示不调用外部翻译接口', async () => {
  const fetch = vi.fn(async () => response({ configured: false })); vi.stubGlobal('fetch', fetch)
  await expect(productAPI.upload(productA.public_id, new File(['abc'], 'a.svg', { type: 'image/svg+xml' }), 100)).rejects.toThrow()
  await expect(productAPI.upload(productA.public_id, new File(['abc'], 'a.png', { type: 'image/png' }), 2)).rejects.toThrow('大小'); expect(fetch).not.toHaveBeenCalled()
  expect(await productAPI.translate('跑步上衣')).toBeNull(); expect(fetch).toHaveBeenCalledTimes(1)
})
