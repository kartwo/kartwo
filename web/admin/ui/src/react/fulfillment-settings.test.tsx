// 配送政策交互测试 / Shipping and Policy Interaction Tests
// 功能：覆盖金额精度、国家范围、规则变更与生成前保存及权限确认
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:01:04

import { Suspense } from 'react'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfigProvider } from 'antd'
import { MemoryRouter } from 'react-router-dom'
import { expect, it, vi } from 'vitest'
import { moneyToCents, centsToMoney } from './money'
import { shippingAPI, parseCountries, parseZones, ratePayload } from './shipping-api'
import { policyAPI, parsePolicy, policyRegions } from './policy-api'
import { AdminRoot } from './AdminRoot'
import { AdminTheme } from './theme'
import { SessionProvider } from './SessionProvider'
import { SessionController } from './session'
import { settingsPages, PageLoading } from './SettingsPages'
import { account, fakeAPI, deferred } from './test-fixtures'
import type { Session } from './contracts'

const countryData = { countries: [
  { code: 'CN', name: 'China', continent: 'Asia' }, { code: 'JP', name: 'Japan', continent: 'Asia' }, { code: 'US', name: 'United States', continent: 'North America' },
], enabled: ['CN', 'US'] }
const defaultZone = { public_id: 'default-fixture-id', name: 'Default', countries: '', rate_cents: 500, free_over_cents: 0, active: true, is_default: true }
const specialZone = { public_id: 'special-fixture-id', name: 'Fixture USA', countries: 'US', rate_cents: 899, free_over_cents: 10000, active: true, is_default: false }
function profileFixture() { return { configured: true, profile: { support_email: 'support@example.com', ship_from: 'China', processing_hours: 48, return_window_days: 7, return_shipping_payer: 'buyer', business_name: 'Fixture Store', delivery_estimates: policyRegions.map(region => ({ region, min_business_days: 5, max_business_days: 10 })) } } }
function response(value: unknown, status = 200) { return new Response(status === 204 ? null : JSON.stringify(value), { status }) }
function mount(path: keyof typeof settingsPages, session: Session = account()) {
  const controller = new SessionController(fakeAPI(session))
  // jsdom does not run CSS transitions; business assertions do not depend on motion.
  render(<MemoryRouter initialEntries={[path]}><AdminTheme><ConfigProvider theme={{ token: { motion: false } }}><SessionProvider controller={controller}><AdminRoot renderPage={route => {
    const Page = settingsPages[route.path as keyof typeof settingsPages]
    return <Suspense fallback={<PageLoading />}><Page /></Suspense>
  }} /></SessionProvider></ConfigProvider></AdminTheme></MemoryRouter>)
  return controller
}
function field(label: string, value: string) { fireEvent.change(screen.getByLabelText(label), { target: { value } }) }
function submit(label: string) { fireEvent.submit(screen.getByLabelText(label).closest('form')!) }
function shippingFetch(zones = [defaultZone, specialZone]) {
  return vi.fn(async (url: string, options: RequestInit) => {
    if (options.method !== 'GET') return response({ ok: true })
    return response(url.endsWith('/countries') ? countryData : { zones })
  })
}

it('金额按十进制精确转分，零运费有效，拒绝隐式舍入、指数和溢出', () => {
  for (const [value, cents] of [['0', 0], ['0.29', 29], ['1.1', 110], [' 10.01 ', 1001], ['90071992547409.91', Number.MAX_SAFE_INTEGER]] as const) {
    expect(moneyToCents(value)).toBe(cents)
    expect(moneyToCents(centsToMoney(cents))).toBe(cents)
  }
  for (const value of ['', '-1', '1.001', '1e2', '+1', '1.', '.2', '1,000', '90071992547409.92', '0'.repeat(25)]) expect(() => moneyToCents(value)).toThrow()
  expect(() => centsToMoney(1.1)).toThrow()
  expect(ratePayload({ mode: 'free', rate: 'not-used', freeOver: 'not-used' })).toEqual({ rate_cents: 0, free_over_cents: 0 })
  expect(() => ratePayload({ mode: 'threshold', rate: '5.00', freeOver: '0.00' })).toThrow('包邮门槛')
})
it('国家和规则契约拒绝未知国家、重复规则、不安全金额和默认状态矛盾', () => {
  expect(() => parseCountries({ ...countryData, enabled: ['XX'] })).toThrow()
  expect(() => parseCountries({ ...countryData, countries: [countryData.countries[0], countryData.countries[0]] })).toThrow()
  expect(() => parseZones({ zones: [defaultZone, defaultZone] })).toThrow()
  expect(() => parseZones({ zones: [{ ...specialZone, is_default: true }] })).toThrow()
  expect(() => parseZones({ zones: [{ ...specialZone, rate_cents: Number.MAX_SAFE_INTEGER + 1 }] })).toThrow()
})
it('运费写入只提交规则字段，公开标识路径受限', async () => {
  const fetch = shippingFetch(); vi.stubGlobal('fetch', fetch)
  await shippingAPI.saveDefault({ mode: 'threshold', rate: '0.29', freeOver: '20.01' })
  expect(fetch).toHaveBeenNthCalledWith(1, '/admin/api/settings/shipping/default', expect.objectContaining({ method: 'PUT', body: JSON.stringify({ name: 'Default', countries: '', rate_cents: 29, free_over_cents: 2001 }) }))
  await shippingAPI.saveRule({ name: ' West ', countries: ['US', 'US'], mode: 'flat', rate: '8.99', freeOver: 'unused' }, specialZone.public_id)
  expect(fetch).toHaveBeenNthCalledWith(3, '/admin/api/settings/shipping/special-fixture-id', expect.objectContaining({ method: 'PATCH', body: JSON.stringify({ name: 'West', countries: 'US', rate_cents: 899, free_over_cents: 0 }) }))
  await shippingAPI.saveRule({ name: ' New ', countries: ['CN'], mode: 'free', rate: '', freeOver: '' })
  expect(fetch).toHaveBeenNthCalledWith(5, '/admin/api/settings/shipping', expect.objectContaining({ method: 'POST', body: '{"name":"New","countries":"CN","rate_cents":0,"free_over_cents":0}' }))
  await expect(shippingAPI.removeRule('../private')).rejects.toThrow()
  expect(fetch).toHaveBeenCalledTimes(6)
})

it('国家搜索保留其他选择，清空可提交空数组；授权员工可管理', async () => {
  const fetch = shippingFetch(); vi.stubGlobal('fetch', fetch)
  mount('/shipping', account('staff', ['settings.shipping']))
  await screen.findByLabelText('搜索配送国家')
  field('搜索配送国家', 'Japan')
  await userEvent.setup().click(screen.getByRole('checkbox', { name: /Japan/ }))
  await userEvent.setup().click(screen.getByRole('button', { name: '保存配送范围' }))
  await screen.findByText('可配送范围已保存')
  const write = fetch.mock.calls.find(([, options]) => options.method === 'PUT')
  expect(JSON.parse(write?.[1].body as string)).toEqual({ countries: ['CN', 'US', 'JP'] })
  await userEvent.setup().click(screen.getByRole('button', { name: /清\s*空/ }))
  await userEvent.setup().click(screen.getByRole('button', { name: '保存配送范围' }))
  await waitFor(() => expect(fetch.mock.calls.filter(([, options]) => options.method === 'PUT')).toHaveLength(2))
  const writes = fetch.mock.calls.filter(([, options]) => options.method === 'PUT')
  expect(JSON.parse(writes[1]?.[1].body as string)).toEqual({ countries: [] })
})
it('默认运费拒绝小数精度丢失，免费模式提交整数零', async () => {
  const fetch = shippingFetch(); vi.stubGlobal('fetch', fetch)
  mount('/shipping')
  await screen.findByLabelText('搜索配送国家')
  await userEvent.setup().click(screen.getByRole('tab', { name: '默认配送费' }))
  await screen.findByLabelText('默认固定运费（US$）')
  field('默认固定运费（US$）', '5.001'); submit('默认固定运费（US$）')
  await screen.findByText('金额须为非负数，最多两位小数')
  expect(fetch.mock.calls.every(([, options]) => options.method === 'GET')).toBe(true)
  await userEvent.setup().click(screen.getByRole('radio', { name: '免费配送' }))
  await userEvent.setup().click(screen.getByRole('button', { name: '保存默认运费' }))
  await screen.findByText('默认运费已保存')
  expect(fetch.mock.calls.find(([, options]) => options.method === 'PUT')?.[1].body).toBe('{"name":"Default","countries":"","rate_cents":0,"free_over_cents":0}')
})
it('特殊规则可查看编辑并通过确认删除；取消确认不发请求', async () => {
  let deleted = false
  const fetch = vi.fn(async (url: string, options: RequestInit) => {
    if (options.method === 'DELETE') { deleted = true; return response(null, 204) }
    return response(url.endsWith('/countries') ? countryData : { zones: deleted ? [defaultZone] : [defaultZone, specialZone] })
  }); vi.stubGlobal('fetch', fetch)
  mount('/shipping'); await screen.findByLabelText('搜索配送国家')
  await userEvent.setup().click(screen.getByRole('tab', { name: /特殊地区规则/ }))
  await screen.findByText('Fixture USA')
  await userEvent.setup().click(screen.getByRole('button', { name: /编\s*辑/ }))
  expect((screen.getByLabelText('特殊规则固定运费（US$）') as HTMLInputElement).value).toBe('8.99')
  await userEvent.setup().click(screen.getByRole('button', { name: /删\s*除/ }))
  let dialog = await screen.findByRole('dialog', { name: '删除配送规则' })
  await userEvent.setup().click(within(dialog).getByRole('button', { name: /取\s*消/ }))
  await waitFor(() => expect(screen.queryByRole('dialog', { name: '删除配送规则' })).toBeNull())
  expect(fetch.mock.calls.every(([, options]) => options.method === 'GET')).toBe(true)
  await userEvent.setup().click(screen.getByRole('button', { name: /删\s*除/ }))
  dialog = await screen.findByRole('dialog', { name: '删除配送规则' })
  await userEvent.setup().click(within(dialog).getByRole('button', { name: /删\s*除/ }))
  await screen.findByText('配送规则已删除')
  await screen.findByRole('button', { name: '新增特殊规则' })
  expect((screen.getByLabelText('规则名称') as HTMLInputElement).value).toBe('')
  expect(fetch.mock.calls.some(([url, options]) => url.endsWith('/special-fixture-id') && options.method === 'DELETE')).toBe(true)
})
it('配送演示可搜索切页但禁止写入', async () => {
  const fetch = shippingFetch(); vi.stubGlobal('fetch', fetch); mount('/shipping', account('demo'))
  await screen.findByLabelText('搜索配送国家'); field('搜索配送国家', 'JP')
  expect((screen.getByRole('checkbox', { name: /Japan/ }) as HTMLInputElement).disabled).toBe(true)
  await userEvent.setup().click(screen.getByRole('tab', { name: '默认配送费' }))
  submit('默认固定运费（US$）'); await act(async () => {})
  expect(fetch).toHaveBeenCalledTimes(2)
})

it('政策契约拒绝时效缺项、地区重复或反向范围，保存过滤元数据', async () => {
  const fixture = profileFixture()
  expect(() => parsePolicy({ ...fixture, profile: { ...fixture.profile, delivery_estimates: [] } })).toThrow()
  expect(() => parsePolicy({ ...fixture, profile: { ...fixture.profile, delivery_estimates: fixture.profile.delivery_estimates.map(() => fixture.profile.delivery_estimates[0]) } })).toThrow()
  expect(() => parsePolicy({ ...fixture, profile: { ...fixture.profile, delivery_estimates: fixture.profile.delivery_estimates.map(row => ({ ...row, max_business_days: 1 })) } })).toThrow()
  const fetch = vi.fn(async () => response({ ok: true })); vi.stubGlobal('fetch', fetch)
  await policyAPI.save(parsePolicy(fixture).profile)
  expect(fetch).toHaveBeenCalledWith('/admin/api/settings/policy-profile', expect.objectContaining({ method: 'PUT', body: JSON.stringify(fixture.profile) }))
})
it('政策时效最大值不能小于最小值；未配置也可验证并保存后生成', async () => {
  const pending = deferred<Response>(), calls: string[] = []
  const fetch = vi.fn(async (url: string, options: RequestInit) => {
    calls.push(`${options.method} ${url}`)
    if (options.method === 'GET') return response({ ...profileFixture(), configured: false })
    if (options.method === 'PUT') return pending.promise
    return response({ created: 7, updated: 0, skipped: 0 })
  }); vi.stubGlobal('fetch', fetch); mount('/policies')
  await screen.findByLabelText('客服邮箱')
  field('亚洲 / Asia 最多天数', '1'); submit('客服邮箱')
  await screen.findByText('最多天数不能小于最少天数')
  expect(fetch).toHaveBeenCalledTimes(1)
  field('亚洲 / Asia 最多天数', '10')
  await userEvent.setup().click(screen.getByRole('checkbox', { name: '新建页面后直接发布到店面页脚' }))
  await userEvent.setup().click(screen.getByRole('button', { name: '生成缺少页面' }))
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2))
  expect(calls).toEqual(['GET /admin/api/settings/policy-profile', 'PUT /admin/api/settings/policy-profile'])
  await act(async () => pending.resolve(response({ ok: true })))
  await screen.findByText('完成：新建 7，更新 0，跳过 0')
  expect(fetch).toHaveBeenLastCalledWith('/admin/api/content-pages/generate-footer', expect.objectContaining({ method: 'POST', body: '{"publish":false,"overwrite":false}' }))
})
it('覆盖政策正文须确认，取消不保存或生成；确认保留 overwrite 语义', async () => {
  const fetch = vi.fn(async (_url: string, options: RequestInit) => response(options.method === 'GET' ? profileFixture() : options.method === 'PUT' ? { ok: true } : { created: 0, updated: 7, skipped: 0 }))
  vi.stubGlobal('fetch', fetch); mount('/policies'); await screen.findByLabelText('客服邮箱')
  const button = screen.getByRole('button', { name: '重新生成并覆盖正文' })
  await userEvent.setup().click(button)
  let dialog = await screen.findByRole('dialog', { name: '覆盖标准页面正文' })
  await userEvent.setup().click(within(dialog).getByRole('button', { name: /取\s*消/ }))
  expect(fetch).toHaveBeenCalledTimes(1)
  await userEvent.setup().click(button)
  dialog = await screen.findByRole('dialog', { name: '覆盖标准页面正文' })
  await userEvent.setup().click(within(dialog).getByRole('button', { name: '重新生成' }))
  await screen.findByText('完成：新建 0，更新 7，跳过 0')
  expect(fetch).toHaveBeenLastCalledWith('/admin/api/content-pages/generate-footer', expect.objectContaining({ body: '{"publish":true,"overwrite":true}' }))
})
it('政策资料权限不含页面生成权限', async () => {
  const fetch = vi.fn(async () => response(profileFixture())); vi.stubGlobal('fetch', fetch)
  mount('/policies', account('staff', ['settings.policy']))
  expect(await screen.findByRole('button', { name: '保存资料' })).toBeTruthy()
  expect(screen.queryByRole('button', { name: '生成缺少页面' })).toBeNull()
  expect(screen.queryByRole('button', { name: '重新生成并覆盖正文' })).toBeNull()
})
it('政策演示即使提交表单也不能写入', async () => {
  const fetch = vi.fn(async () => response(profileFixture())); vi.stubGlobal('fetch', fetch)
  mount('/policies', account('demo')); await screen.findByLabelText('客服邮箱')
  expect((screen.getByLabelText('客服邮箱') as HTMLInputElement).disabled).toBe(true)
  submit('客服邮箱'); await act(async () => {})
  expect(fetch).toHaveBeenCalledTimes(1)
})
it('政策保存失败时不生成页面', async () => {
  const fetch = vi.fn(async (_url: string, options: RequestInit) => options.method === 'GET' ? response(profileFixture()) : response({ error: '保存失败' }, 500))
  vi.stubGlobal('fetch', fetch); mount('/policies'); await screen.findByLabelText('客服邮箱')
  await userEvent.setup().click(screen.getByRole('button', { name: '生成缺少页面' }))
  await screen.findByText('保存失败')
  expect(fetch).toHaveBeenCalledTimes(2)
})

it.each(['/shipping', '/policies'] as const)('%s 读取失败时不提供默认可写表单', async path => {
  vi.stubGlobal('fetch', vi.fn(async () => response({ error: '读取失败' }, 500)))
  mount(path); expect(await screen.findByText('读取失败')).toBeTruthy()
  expect(screen.queryByLabelText('搜索配送国家')).toBeNull()
  expect(screen.queryByLabelText('客服邮箱')).toBeNull()
})
it('政策资料保存成功而生成失败时显示真实结果，不丢失已保存资料', async () => {
  const fetch = vi.fn(async (_url: string, options: RequestInit) => options.method === 'GET' ? response(profileFixture()) : options.method === 'PUT' ? response({ ok: true }) : response({ error: '生成失败' }, 500))
  vi.stubGlobal('fetch', fetch); mount('/policies'); await screen.findByLabelText('客服邮箱')
  field('经营主体名称', 'New Store')
  await userEvent.setup().click(screen.getByRole('button', { name: '生成缺少页面' }))
  await screen.findByText('资料已保存，页面生成失败：生成失败')
  expect((screen.getByLabelText('经营主体名称') as HTMLInputElement).value).toBe('New Store')
  expect(fetch).toHaveBeenCalledTimes(3)
})
