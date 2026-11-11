// 商品页面迁移测试 / Product Pages Migration Tests
// 功能：验证创建、矩阵失效、部分价量保存、媒体/翻译、演示配额和独立权限及读取竞态
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:22:28

import { Suspense } from 'react'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfigProvider } from 'antd'
import { MemoryRouter, useLocation, useNavigate } from 'react-router-dom'
import { expect, it, vi } from 'vitest'
import { AdminRoot } from './AdminRoot'
import { AdminTheme } from './theme'
import { SessionProvider } from './SessionProvider'
import { SessionController } from './session'
import { businessPages } from './BusinessPages'
import { PageLoading } from './SettingsPages'
import { account, fakeAPI, deferred } from './test-fixtures'
import { assetA, productA, productB, productCategory, summary } from './product-fixtures'
import type { Session } from './contracts'
const variantID = productA.variants[0]!.public_id
function response(value: unknown, status = 200) { return new Response(JSON.stringify(value), { status }) }
function fixtureFetch(handle?: (url: string, options: RequestInit) => Response | undefined | Promise<Response | undefined>) {
  const fetch = vi.fn(async (url: string, options: RequestInit) => {
    const handled = await handle?.(url, options); if (handled) return handled
    if (options.method === 'GET') {
      if (url.endsWith('/categories')) return response({ categories: [productCategory] })
      if (url.endsWith('/media')) return response({ media: [assetA] })
      if (url.endsWith('/products')) return response({ products: [summary(productA)] })
      if (url.endsWith('/product-fixture-a')) return response(productA)
      if (url.endsWith('/product-fixture-b')) return response(productB)
    }
    throw new Error(`Unexpected fixture request ${options.method} ${url}`)
  }); vi.stubGlobal('fetch', fetch); return fetch
}
function Controls() { const location = useLocation(), navigate = useNavigate(); return <><output aria-label="测试当前地址">{location.pathname}</output><button onClick={() => void navigate('/products/product-fixture-b')}>测试切到商品 B</button></> }
function mount(path: string, session: Session = account()) {
  const controller = new SessionController(fakeAPI(session))
  render(<MemoryRouter initialEntries={[path]}><AdminTheme><ConfigProvider theme={{ token: { motion: false } }}><SessionProvider controller={controller}><Controls /><AdminRoot renderPage={route => {
    const Page = businessPages[route.path as keyof typeof businessPages]
    return <Suspense fallback={<PageLoading />}><Page /></Suspense>
  }} /></SessionProvider></ConfigProvider></AdminTheme></MemoryRouter>); return controller
}
function change(name: string | RegExp, value: string) { fireEvent.change(screen.getByRole('textbox', { name }), { target: { value } }) }
function submitBasic() { fireEvent.submit(screen.getByRole('textbox', { name: '英文标题' }).closest('form')!) }
function submitVariant() { fireEvent.submit(screen.getByRole('textbox', { name: `价格：${variantID}` }).closest('form')!) }
function writes(fetch: ReturnType<typeof fixtureFetch>) { return fetch.mock.calls.filter(([, options]) => options.method !== 'GET') }
async function singleMatrix() {
  await screen.findByRole('textbox', { name: '英文标题' }); change('英文标题', 'New Running Shirt'); change('轴 1 取值', 'S')
  await userEvent.click(screen.getByRole('button', { name: '删除轴 2' })); await userEvent.click(screen.getByRole('button', { name: '生成变体组合' }))
}
it('商品列表只读员工可进入详情，不能新建、删除或设置精选', async () => {
  fixtureFetch(); mount('/products', account('staff', ['products.view']))
  expect(await screen.findByRole('link', { name: productA.title })).toBeTruthy()
  expect(screen.queryByRole('button', { name: '新建商品' })).toBeNull(); expect(screen.queryByRole('button', { name: /删除商品/ })).toBeNull(); expect(screen.queryByRole('button', { name: /精选：/ })).toBeNull()
  await userEvent.click(screen.getByRole('link', { name: productA.title })); expect((await screen.findByRole('textbox', { name: '英文标题' }) as HTMLInputElement).disabled).toBe(true)
  expect(screen.queryByRole('button', { name: '保存基本信息' })).toBeNull(); expect(screen.queryByRole('button', { name: /保存变体/ })).toBeNull(); expect(screen.queryByLabelText('上传商品图片')).toBeNull()
})
it('精选仅上架商品且具发布权限可操作，不需要基本信息编辑权限', async () => {
  const fetch = fixtureFetch((url, options) => url.endsWith('/featured') && options.method === 'PATCH' ? response({ ok: true }) : undefined)
  mount('/products', account('staff', ['products.view', 'products.publish']))
  await userEvent.click(await screen.findByRole('button', { name: `设置精选：${productA.title}` }))
  await screen.findByRole('button', { name: `取消精选：${productA.title}` }); expect(writes(fetch)).toHaveLength(1); expect(JSON.parse(writes(fetch)[0]![1].body as string)).toEqual({ featured: true })
})
it('商品删除可取消，写入确认后刷新失败仍保留已删除事实', async () => {
  let deleted = false
  const fetch = fixtureFetch((url, options) => {
    if (options.method === 'DELETE') { deleted = true; return response({ ok: true }) }
    if (deleted && url.endsWith('/products')) return response({ error: 'List unavailable' }, 500)
  }); mount('/products')
  await userEvent.click(await screen.findByRole('button', { name: `删除商品：${productA.title}` })); await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: /取\s*消/ })); expect(deleted).toBe(false)
  await userEvent.click(screen.getByRole('button', { name: `删除商品：${productA.title}` })); await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: /删\s*除/ }))
  await screen.findByText('还没有商品'); expect(screen.getByText(/列表刷新失败/)).toBeTruthy(); expect(writes(fetch)).toHaveLength(1)
})
it('新建商品空价格和小数库存拒绝，显式零价按整数提交且创建后进入已有 ID', async () => {
  const fetch = fixtureFetch((url, options) => url.endsWith('/products') && options.method === 'POST' ? response({ public_id: productA.public_id }, 201) : undefined)
  mount('/products/new'); await singleMatrix()
  submitBasic(); await screen.findByText(/第 1 个变体：金额须为非负数/); expect(writes(fetch)).toHaveLength(0)
  change('变体 1 价格', '0'); change('变体 1 库存', '1.5'); submitBasic(); await screen.findByText(/库存须为非负安全整数/); expect(writes(fetch)).toHaveLength(0)
  change('变体 1 库存', '2'); submitBasic()
  await waitFor(() => expect(screen.getByLabelText('测试当前地址').textContent).toBe(`/products/${productA.public_id}`))
  const body = JSON.parse(writes(fetch)[0]![1].body as string); expect(body.slug).toBe('new-running-shirt'); expect(body.status).toBe('draft'); expect(body.options).toEqual([{ name: '尺码', values: ['S'] }]); expect(body.variants).toEqual([{ sku: '', price_cents: 0, quantity: 2, selections: [{ option: '尺码', value: 'S' }] }]); expect(body).not.toHaveProperty('public_id')
})
it('更改变体轴不静默复用旧组合，重新生成前可取消并保留已填价格', async () => {
  const fetch = fixtureFetch(); mount('/products/new'); await singleMatrix(); change('变体 1 价格', '1.01'); change('轴 1 取值', 'S,M')
  submitBasic(); await screen.findByText('变体轴已改变，请重新生成组合后再保存'); expect(writes(fetch)).toHaveLength(0)
  await userEvent.click(screen.getByRole('button', { name: '生成变体组合' })); await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: /取\s*消/ }))
  expect((screen.getByRole('textbox', { name: '变体 1 价格' }) as HTMLInputElement).value).toBe('1.01')
  await userEvent.click(screen.getByRole('button', { name: '生成变体组合' })); await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: '重新生成' }))
  expect((await screen.findByRole('textbox', { name: '变体 2 价格' }) as HTMLInputElement).value).toBe(''); expect((screen.getByRole('textbox', { name: '变体 1 价格' }) as HTMLInputElement).value).toBe('')
})
it('新建链接随英文标题变化直到手动修改；中文辅助 slug 不触发自动翻译', async () => {
  const fetch = fixtureFetch(); mount('/products/new'); await screen.findByRole('textbox', { name: '英文标题' })
  change('英文标题', 'Running Shirt'); expect((screen.getByRole('textbox', { name: /^英文 slug/ }) as HTMLInputElement).value).toBe('running-shirt')
  change(/^英文 slug/, 'custom-shirt'); change('英文标题', 'New Title'); expect((screen.getByRole('textbox', { name: /^英文 slug/ }) as HTMLInputElement).value).toBe('custom-shirt')
  change('中文 slug（可选）', '中文链接'); expect(writes(fetch)).toHaveLength(0)
})
it('只有改价权限的员工可改价格，不能提交库存、基本信息或上传图片', async () => {
  const fetch = fixtureFetch((url, options) => url.endsWith('/price') && options.method === 'PATCH' ? response({ ok: true }) : undefined)
  mount(`/products/${productA.public_id}`, account('staff', ['products.view', 'products.price']))
  const input = await screen.findByRole('textbox', { name: `价格：${variantID}` }); expect((input as HTMLInputElement).disabled).toBe(false)
  expect((screen.getByRole('textbox', { name: `库存：${variantID}` }) as HTMLInputElement).disabled).toBe(true)
  change(`价格：${variantID}`, '2.02'); submitVariant(); await waitFor(() => expect(writes(fetch)).toHaveLength(1)); expect(writes(fetch)[0]![0]).toBe(`/admin/api/variants/${variantID}/price`)
  submitBasic(); await act(async () => {}); expect(writes(fetch)).toHaveLength(1); expect(screen.queryByRole('button', { name: '翻译为英文标题' })).toBeNull()
})
it('只有库存权限的员工不被改价权限拦住，只提交整数库存', async () => {
  const fetch = fixtureFetch((url, options) => url.endsWith('/inventory') && options.method === 'PATCH' ? response({ ok: true }) : undefined)
  mount(`/products/${productA.public_id}`, account('staff', ['products.view', 'products.inventory']))
  await screen.findByRole('textbox', { name: `库存：${variantID}` }); change(`库存：${variantID}`, '7'); submitVariant()
  await waitFor(() => expect(writes(fetch)).toHaveLength(1)); expect(writes(fetch)[0]![0]).toMatch(/\/inventory$/); expect(JSON.parse(writes(fetch)[0]![1].body as string)).toEqual({ quantity: 7 }); expect((screen.getByRole('textbox', { name: `价格：${variantID}` }) as HTMLInputElement).disabled).toBe(true)
})
it('价格已保存但库存失败时显示局部成功，保留输入，再次保存只重试库存', async () => {
  let inventoryCalls = 0
  const fetch = fixtureFetch((url, options) => {
    if (options.method === 'PATCH' && url.endsWith('/price')) return response({ ok: true })
    if (options.method === 'PATCH' && url.endsWith('/inventory')) { inventoryCalls++; return inventoryCalls === 1 ? response({ error: 'Inventory unavailable' }, 500) : response({ ok: true }) }
  }); mount(`/products/${productA.public_id}`)
  await screen.findByRole('textbox', { name: `价格：${variantID}` }); change(`价格：${variantID}`, '2.02'); change(`库存：${variantID}`, '7'); submitVariant()
  await screen.findByText(/价格已保存，库存保存未确认/); expect((screen.getByRole('textbox', { name: `库存：${variantID}` }) as HTMLInputElement).value).toBe('7')
  submitVariant(); await waitFor(() => expect(inventoryCalls).toBe(2)); expect(writes(fetch).filter(([url]) => url.endsWith('/price'))).toHaveLength(1)
})
it('字段、变体、翻译和上传共用锁，阻止并发重提和跨操作覆盖', async () => {
  const pending = deferred<Response>()
  const fetch = fixtureFetch((url, options) => options.method === 'PATCH' && url.endsWith('/price') ? pending.promise : undefined)
  mount(`/products/${productA.public_id}`); await screen.findByRole('textbox', { name: `价格：${variantID}` }); await screen.findByLabelText('上传商品图片')
  change(`价格：${variantID}`, '2'); submitVariant(); submitVariant(); submitBasic()
  await waitFor(() => expect(writes(fetch)).toHaveLength(1)); expect((screen.getByRole('textbox', { name: '英文标题' }) as HTMLInputElement).disabled).toBe(true); expect((screen.getByLabelText('上传商品图片') as HTMLInputElement).disabled).toBe(true)
  await act(async () => pending.resolve(response({ ok: true }))); await waitFor(() => expect((screen.getByLabelText('上传商品图片') as HTMLInputElement).disabled).toBe(false))
})
it('没有发布权限的基本编辑保留原状态和分类，不提交固定链接或变体字段', async () => {
  const fetch = fixtureFetch((url, options) => options.method === 'PATCH' && url.endsWith('/product-fixture-a') ? response({ ok: true }) : undefined)
  mount(`/products/${productA.public_id}`, account('staff', ['products.view', 'products.edit']))
  await screen.findByRole('textbox', { name: '英文标题' }); change('英文标题', 'Edited title'); submitBasic()
  await waitFor(() => expect(writes(fetch)).toHaveLength(1)); const body = JSON.parse(writes(fetch)[0]![1].body as string)
  expect(body.status).toBe('active'); expect(body.category_public_ids).toEqual(productA.category_public_ids); expect(body.title).toBe('Edited title'); expect(body).not.toHaveProperty('slug'); expect(body).not.toHaveProperty('variants'); expect((screen.getByRole('combobox', { name: '状态' }) as HTMLInputElement).disabled).toBe(true)
})
it('商品图片上传确认后刷新失败保留新图和未保存基本信息，alt 独立白名单保存', async () => {
  let uploaded = false
  const assetB = { ...assetA, public_id: 'media-fixture-b', alt_text: '' }
  const fetch = fixtureFetch((url, options) => {
    if (url.endsWith('/media') && options.method === 'POST') { uploaded = true; return response(assetB, 201) }
    if (url.endsWith('/media') && options.method === 'GET' && uploaded) return response({ error: 'Media list unavailable' }, 500)
    if (url.endsWith('/media-fixture-a') && options.method === 'PATCH') return response({ ok: true })
  }); mount(`/products/${productA.public_id}`); await screen.findByLabelText('上传商品图片'); change('英文标题', 'Unsaved title'); change('图片说明：media-fixture-a', '  New alt  ')
  fireEvent.change(screen.getByLabelText('上传商品图片'), { target: { files: [new File(['fixture'], 'image.png', { type: 'image/png' })] } })
  await screen.findByLabelText('图片说明：media-fixture-b'); expect(screen.getByText(/列表刷新失败/)).toBeTruthy(); expect((screen.getByRole('textbox', { name: '英文标题' }) as HTMLInputElement).value).toBe('Unsaved title'); expect((screen.getByLabelText('图片说明：media-fixture-a') as HTMLInputElement).value).toBe('  New alt  ')
  fireEvent.submit(screen.getByLabelText('图片说明：media-fixture-a').closest('form')!); await waitFor(() => expect(writes(fetch)).toHaveLength(2))
  expect(JSON.parse(writes(fetch)[1]![1].body as string)).toEqual({ alt_text: 'New alt' }); expect(writes(fetch)[0]![1].body).toBeInstanceOf(FormData)
})
it('图片读取失败不当空列表或允许上传，基本信息仍可查看；图片删除需要确认', async () => {
  let fail = true, removed = false
  const fetch = fixtureFetch((url, options) => {
    if (url.endsWith('/media') && options.method === 'GET') return fail ? response({ error: 'Media unavailable' }, 500) : response({ media: removed ? [] : [assetA] })
    if (options.method === 'DELETE') { removed = true; return response({ ok: true }) }
  }); mount(`/products/${productA.public_id}`)
  await screen.findByText('Media unavailable'); expect(screen.queryByLabelText('上传商品图片')).toBeNull(); expect(screen.getByRole('textbox', { name: '英文标题' })).toBeTruthy()
  fail = false; await userEvent.click(screen.getByRole('button', { name: /重\s*试/ })); await userEvent.click(await screen.findByRole('button', { name: '删除图片：media-fixture-a' }))
  await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: /取\s*消/ })); expect(removed).toBe(false)
  await userEvent.click(screen.getByRole('button', { name: '删除图片：media-fixture-a' })); await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: /删\s*除/ }))
  await screen.findByText('还没有图片，选择一张上传。'); expect(writes(fetch)).toHaveLength(1)
})
it('演示保护示例商品字段、变体和图片只读，不能利用表单事件发送写入', async () => {
  const fetch = fixtureFetch(); mount(`/products/${productA.public_id}`, account('demo'))
  await screen.findByText('这是受保护的示例商品。公开演示可以查看，不能修改或删除。'); await screen.findByLabelText('图片说明：media-fixture-a')
  expect((screen.getByRole('textbox', { name: `价格：${variantID}` }) as HTMLInputElement).disabled).toBe(true); expect(screen.queryByLabelText('上传商品图片')).toBeNull()
  submitBasic(); submitVariant(); fireEvent.submit(screen.getByLabelText('图片说明：media-fixture-a').closest('form')!); await act(async () => {}); expect(writes(fetch)).toHaveLength(0)
})
it('演示自己的临时商品可编辑价量和图片说明，状态固定草稿且图片额度受限', async () => {
  const fetch = fixtureFetch((url, options) => {
    if (url.endsWith('/product-fixture-a') && options.method === 'GET') return response({ ...productA, demo_owned: true, status: 'draft' })
    if (url.endsWith('/product-fixture-a') && options.method === 'PATCH') return response({ ok: true })
  }); mount(`/products/${productA.public_id}`, account('demo'))
  await screen.findByLabelText('上传商品图片'); expect((screen.getByLabelText('上传商品图片') as HTMLInputElement).disabled).toBe(true); expect((screen.getByRole('textbox', { name: `库存：${variantID}` }) as HTMLInputElement).disabled).toBe(false)
  submitBasic(); await waitFor(() => expect(writes(fetch)).toHaveLength(1)); expect(JSON.parse(writes(fetch)[0]![1].body as string).status).toBe('draft'); expect(screen.queryByRole('button', { name: '翻译为英文标题' })).toBeNull()
})
it('演示新建入口和列表按会话额度计数，不能绕过满额直接打开创建页', async () => {
  fixtureFetch((url, options) => url.endsWith('/products') && options.method === 'GET' ? response({ products: Array.from({ length: 3 }, (_, i) => ({ ...summary(productA), public_id: `product-demo-${i}`, demo_owned: true, status: 'draft' })) }) : undefined)
  mount('/products/new', account('demo')); await screen.findByText(/临时商品额度已满/); expect(screen.queryByRole('textbox', { name: '英文标题' })).toBeNull()
})
it('翻译只在店主点击时请求，未配置只提示；成功只更新目标内容并生成英文链接', async () => {
  let configured = false
  const fetch = fixtureFetch((url, options) => {
    if (url.endsWith('/settings/translation')) return response({ configured })
    if (url.endsWith('/translation/text') && options.method === 'POST') return response({ text: 'Translated Shirt' })
  }); mount('/products/new'); await screen.findByRole('textbox', { name: '英文标题' }); change('中文标题（可选）', '跑步上衣'); expect(writes(fetch)).toHaveLength(0)
  await userEvent.click(screen.getByRole('button', { name: '翻译为英文标题' })); await screen.findByText('尚未设置翻译 API，无法生成英文内容。'); expect(writes(fetch)).toHaveLength(0)
  configured = true; await userEvent.click(screen.getByRole('button', { name: '翻译为英文标题' })); await waitFor(() => expect((screen.getByRole('textbox', { name: '英文标题' }) as HTMLInputElement).value).toBe('Translated Shirt'))
  expect((screen.getByRole('textbox', { name: /^英文 slug/ }) as HTMLInputElement).value).toBe('translated-shirt'); expect(writes(fetch)).toHaveLength(1)
})
it('分类读取失败不显示可保存默认分类，重试不丢已有选择', async () => {
  let fail = true
  fixtureFetch(url => url.endsWith('/categories') && fail ? response({ error: 'Categories unavailable' }, 500) : undefined)
  mount(`/products/${productA.public_id}`); await screen.findByText('Categories unavailable'); expect(screen.queryByRole('button', { name: '保存基本信息' })).toBeNull()
  fail = false; await userEvent.click(screen.getByRole('button', { name: /重\s*试/ })); await screen.findByRole('textbox', { name: '英文标题' }); expect(screen.getByText('Tops', { selector: '.ant-select-selection-item-content' })).toBeTruthy()
})
it('切换商品 B 后忽略迟到的 A 详情，保存 401 清除整页内容与会话', async () => {
  const pending = deferred<Response>()
  const fetch = fixtureFetch((url, options) => {
    if (url.endsWith('/product-fixture-a') && options.method === 'GET') return pending.promise
    if (options.method === 'PATCH') return response({ error: 'Session expired' }, 401)
  }); const controller = mount(`/products/${productA.public_id}`)
  await userEvent.click(await screen.findByRole('button', { name: '测试切到商品 B' })); expect((await screen.findByRole('textbox', { name: '英文标题' }) as HTMLInputElement).value).toBe(productB.title)
  await act(async () => pending.resolve(response(productA))); expect((screen.getByRole('textbox', { name: '英文标题' }) as HTMLInputElement).value).toBe(productB.title)
  submitBasic(); await waitFor(() => expect(controller.getSnapshot().phase).toBe('login')); expect(screen.queryByRole('textbox', { name: '英文标题' })).toBeNull(); expect(writes(fetch)).toHaveLength(1)
})
