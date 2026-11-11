// 商品测试样本 / Product Test Fixtures
// 功能：隔离测试专用的商品、变体和本地媒体 API 样本，不包含真实店铺数据
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:22:28

import type { Product, ProductSummary, ProductMedia } from './product-api'
export const productA: Product = { public_id: 'product-fixture-a', title: 'Running Shirt', title_zh: '跑步上衣', slug: 'running-shirt', slug_zh: '跑步上衣', description: 'Original description', seo_description: 'SEO', seo_description_zh: '中文描述', status: 'active', featured: false, demo_owned: false, category_public_ids: ['category-fixture-a'],
  variants: [{ public_id: 'variant-fixture-a', sku: 'SHIRT-A', price_cents: 101, quantity: 5, options: [{ name: 'Size', value: 'S' }] }] }
export const productB: Product = { ...productA, public_id: 'product-fixture-b', title: 'Other Shirt', slug: 'other-shirt', description: 'Body B' }
export function summary(product: Product): ProductSummary { return { public_id: product.public_id, title: product.title, slug: product.slug, status: product.status, featured: product.featured, demo_owned: product.demo_owned, updated_at: '2026-10-25' } }
export const productCategory = { public_id: 'category-fixture-a', name: 'Tops', slug: 'tops', position: 0, product_count: 1 }
export const assetA: ProductMedia = { public_id: 'media-fixture-a', mime: 'image/png', width: 1200, height: 800, size_bytes: 2000, alt_text: 'Original alt', original_url: `/media/originals/${'a'.repeat(64)}.png`, derivatives: [{ label: 'thumb', url: `/media/derived/${'a'.repeat(64)}_thumb.webp`, width: 200, height: 133 }] }
