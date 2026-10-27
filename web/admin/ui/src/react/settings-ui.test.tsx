// 配置表单交互测试 / Settings Form Interaction Tests
// 功能：通过实际请求客户端验证保存、演示只读、权限、测试连接与向导
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:25:50

import { Suspense } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { expect, it, vi } from 'vitest'
import { AdminTheme } from './theme'
import { SessionProvider } from './SessionProvider'
import { AdminRoot } from './AdminRoot'
import { SessionController } from './session'
import { settingsPages, PageLoading } from './SettingsPages'
import { account, deferred, fakeAPI } from './test-fixtures'
import { paymentFixture, domainFixture, smtpFixture } from './settings-fixtures'
import type { Session } from './contracts'

function mount(path: keyof typeof settingsPages, session: Session = account(), controller = new SessionController(fakeAPI(session))) {
  const page = vi.fn(route => {
    const Settings = settingsPages[route.path as keyof typeof settingsPages]
    return <Suspense fallback={<PageLoading />}><Settings /></Suspense>
  })
  render(<MemoryRouter initialEntries={[path]}><AdminTheme><SessionProvider controller={controller}><AdminRoot renderPage={page} /></SessionProvider></AdminTheme></MemoryRouter>)
  return { controller, page }
}
function response(body: unknown, status = 200) { return new Response(JSON.stringify(body), { status }) }
function submit(label: string | RegExp) { fireEvent.submit(screen.getByLabelText(label).closest('form')!) }
function field(label: string | RegExp, value: string) { fireEvent.change(screen.getByLabelText(label), { target: { value } }) }

it.each(['stripe', 'paypal'] as const)('保存 %s 只清空本通道密钥，保留另一个通道未保存的草稿', async channel => {
  const writes: RequestInit[] = []
  vi.stubGlobal('fetch', vi.fn(async (_url: string, options: RequestInit) => {
    if (options.method !== 'GET') writes.push(options)
    return response(paymentFixture())
  }))
  mount('/payment')
  await screen.findByDisplayValue('fixture-public')
  await screen.findByDisplayValue('fixture-client')
  const user = userEvent.setup()
  await user.type(screen.getByLabelText(/Secret key（/), 'stripe-draft-value')
  await user.type(screen.getByLabelText('PayPal Secret'), 'paypal-draft-value')
  if (channel === 'stripe') {
    await user.clear(screen.getByLabelText('Client ID')); await user.type(screen.getByLabelText('Client ID'), 'Unsaved PayPal client')
  } else {
    await user.clear(screen.getByLabelText('Publishable key')); await user.type(screen.getByLabelText('Publishable key'), 'Unsaved Stripe public')
  }
  await user.click(screen.getByRole('button', { name: channel === 'stripe' ? '保存 Stripe' : '保存 PayPal' }))
  await screen.findByText(channel === 'stripe' ? 'Stripe 已保存，收款配置已即时生效。' : 'PayPal 已保存，收款配置已即时生效。')
  expect(writes).toHaveLength(1)
  expect(Object.keys(JSON.parse(writes[0]!.body as string))).toEqual([channel])
  expect((screen.getByLabelText(channel === 'stripe' ? /Secret key（/ : 'PayPal Secret') as HTMLInputElement).value).toBe('')
  expect((screen.getByLabelText(channel === 'stripe' ? 'PayPal Secret' : /Secret key（/) as HTMLInputElement).value).toBe(channel === 'stripe' ? 'paypal-draft-value' : 'stripe-draft-value')
  expect((screen.getByLabelText(channel === 'stripe' ? 'Client ID' : 'Publishable key') as HTMLInputElement).value).toBe(channel === 'stripe' ? 'Unsaved PayPal client' : 'Unsaved Stripe public')
})

it('收款分开保存，成功清空密钥；测试连接不提交未保存内容', async () => {
  const data = paymentFixture()
  const writes: RequestInit[] = []
  const fetch = vi.fn(async (_url: string, options: RequestInit) => {
    if (options.method !== 'GET') writes.push(options)
    return response(data)
  })
  vi.stubGlobal('fetch', fetch)
  mount('/payment')
  await screen.findByLabelText('Publishable key')
  field(/Secret key（/, 'fixture-stripe-secret')
  field(/Webhook 签名密钥/, 'fixture-signing-secret')
  submit('Publishable key')
  await screen.findByText('Stripe 已保存，收款配置已即时生效。')
  expect(JSON.parse(writes[0]?.body as string)).toEqual({ stripe: { mode: 'test', publishable: 'fixture-public', secret: 'fixture-stripe-secret', webhook_secret: 'fixture-signing-secret' } })
  expect((screen.getByLabelText(/Secret key（/) as HTMLInputElement).value).toBe('')
  field('PayPal Secret', 'fixture-paypal-secret')
  submit('Client ID')
  await screen.findByText('PayPal 已保存，收款配置已即时生效。')
  expect(JSON.parse(writes[1]?.body as string)).toEqual({ paypal: { mode: 'sandbox', client_id: 'fixture-client', secret: 'fixture-paypal-secret', webhook_id: 'fixture-webhook' } })
  field(/Secret key（/, 'not-saved')
  await userEvent.setup().click(screen.getByRole('button', { name: '测试 Stripe 连接' }))
  await screen.findByText('Stripe 连接成功，已保存的密钥与模式一致。')
  expect(writes).toHaveLength(3)
  expect(writes[2]?.body).toBeUndefined()
  expect(fetch).toHaveBeenLastCalledWith('/admin/api/settings/payment/stripe/test', expect.objectContaining({ method: 'POST' }))
})
it('环境变量通道只读，但允许店主测试已保存的连接', async () => {
  const data = paymentFixture(); data.stripe.source = 'env'; data.stripe.readonly = true
  const fetch = vi.fn(async () => response(data)); vi.stubGlobal('fetch', fetch)
  mount('/payment')
  const input = await screen.findByLabelText('Publishable key') as HTMLInputElement
  expect(input.disabled).toBe(true)
  expect(screen.queryByRole('button', { name: '保存 Stripe' })).toBeNull()
  expect(screen.getByRole('button', { name: '保存 PayPal' })).toBeTruthy()
  submit('Publishable key')
  await act(async () => {})
  expect(fetch).toHaveBeenCalledTimes(1)
  await userEvent.setup().click(screen.getByRole('button', { name: '测试 Stripe 连接' }))
  expect(fetch).toHaveBeenCalledTimes(2)
})
it('演示即使收到错误的可写标识也不能保存或测试支付', async () => {
  const fetch = vi.fn(async () => response(paymentFixture())); vi.stubGlobal('fetch', fetch)
  mount('/payment', account('demo'))
  const input = await screen.findByLabelText('Publishable key') as HTMLInputElement
  expect(input.disabled).toBe(true)
  expect((screen.getByRole('button', { name: '测试 Stripe 连接' }) as HTMLButtonElement).disabled).toBe(true)
  submit('Publishable key'); submit('Client ID')
  await act(async () => {})
  expect(fetch).toHaveBeenCalledTimes(1)
})
it('配置读取失败显示重试，不提供空白可写表单', async () => {
  const fetch = vi.fn().mockResolvedValueOnce(response({ error: '配置读取失败' }, 500)).mockResolvedValueOnce(response(paymentFixture()))
  vi.stubGlobal('fetch', fetch)
  mount('/payment')
  expect(await screen.findByText('配置读取失败')).toBeTruthy()
  expect(screen.queryByLabelText('Publishable key')).toBeNull()
  await userEvent.setup().click(screen.getByRole('button', { name: /重\s*试/ }))
  expect(await screen.findByLabelText('Publishable key')).toBeTruthy()
})
it('员工无法挂载收款页；域名权限员工可以保存，并提示需重启', async () => {
  const fetch = vi.fn(async (_url: string, options: RequestInit) => response({ ...domainFixture(), domain: options.method === 'PUT' ? 'new.example.com' : 'shop.example.com' }))
  vi.stubGlobal('fetch', fetch)
  mount('/domain', account('staff', ['settings.domain']))
  await screen.findByLabelText('你的店铺域名')
  field('你的店铺域名', 'https://new.example.com/path')
  submit('你的店铺域名')
  expect(await screen.findByText('请输入完整域名，不要带协议、路径或空格')).toBeTruthy()
  expect(fetch).toHaveBeenCalledTimes(1)
  field('你的店铺域名', 'new.example.com'); submit('你的店铺域名')
  expect(await screen.findByText(/请重启一次 Kartwo/)).toBeTruthy()
  expect(fetch).toHaveBeenLastCalledWith('/admin/api/settings/domain', expect.objectContaining({ method: 'PUT', body: '{"domain":"new.example.com"}' }))
})
it('没有收款权限的员工不发起配置请求', async () => {
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch)
  const { page } = mount('/payment', account('staff', ['settings.domain']))
  expect(await screen.findByText('当前账号没有访问此页面的权限')).toBeTruthy()
  expect(page).not.toHaveBeenCalled(); expect(fetch).not.toHaveBeenCalled()
})
it('本地域名提示不签发 HTTPS，保存后不声称证书已生效', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => response({ ...domainFixture(), https_capable: false })))
  mount('/domain')
  await screen.findByLabelText('你的店铺域名')
  expect(screen.getByText('当前为本地开发模式')).toBeTruthy()
  submit('你的店铺域名')
  expect(await screen.findByText('当前为本地开发模式，不会签发 HTTPS；正式部署并重启后才会生效。')).toBeTruthy()
  expect(screen.queryByText(/之后才会为.*申请并启用 HTTPS/)).toBeNull()
})
it('SMTP 保存保留密码空格并清空输入，测试只发送收件地址', async () => {
  const writes: RequestInit[] = []
  const fetch = vi.fn(async (_url: string, options: RequestInit) => { if (options.method !== 'GET') writes.push(options); return response(smtpFixture()) })
  vi.stubGlobal('fetch', fetch)
  mount('/smtp')
  await screen.findByLabelText('SMTP 主机')
  field('密码 / 授权码', ' fixture-password ')
  field('端口', '65536'); submit('SMTP 主机')
  expect(await screen.findByText('端口需为 1–65535 的整数')).toBeTruthy()
  expect(writes).toHaveLength(0)
  field('端口', '587'); submit('SMTP 主机')
  await screen.findByText('SMTP 配置已保存')
  expect(JSON.parse(writes[0]?.body as string)).toMatchObject({ password: ' fixture-password ', port: '587', encryption: 'starttls' })
  expect((screen.getByLabelText('密码 / 授权码') as HTMLInputElement).value).toBe('')
  field('密码 / 授权码', 'unsaved')
  field('发送测试邮件到', 'test@example.com'); submit('发送测试邮件到')
  await screen.findByText('测试邮件已发送，请查收（含垃圾箱）。')
  expect(JSON.parse(writes[1]?.body as string)).toEqual({ to: 'test@example.com' })
})
it('SMTP 演示配置只读且不能发信', async () => {
  const data = { ...smtpFixture(), source: 'env', readonly: true }
  const fetch = vi.fn(async () => response(data)); vi.stubGlobal('fetch', fetch)
  mount('/smtp', account('demo'))
  expect((await screen.findByLabelText('SMTP 主机') as HTMLInputElement).disabled).toBe(true)
  expect(screen.queryByRole('button', { name: '保存 SMTP' })).toBeNull()
  expect((screen.getByRole('button', { name: '发送测试邮件' }) as HTMLButtonElement).disabled).toBe(true)
  field('发送测试邮件到', 'test@example.com'); submit('发送测试邮件到')
  await act(async () => {})
  expect(fetch).toHaveBeenCalledTimes(1)
})
it('保存返回 401 清空页面并回到登录，避免保留凭证输入', async () => {
  const fetch = vi.fn().mockResolvedValueOnce(response(paymentFixture())).mockResolvedValueOnce(response({ error: '会话已过期' }, 401))
  vi.stubGlobal('fetch', fetch)
  const { controller } = mount('/payment')
  await screen.findByLabelText('Publishable key'); field(/Secret key（/, 'fixture-secret'); submit('Publishable key')
  // Check private DOM removal before waiting for the real lazy authentication chunk.
  await waitFor(() => {
    expect(controller.getSnapshot().session).toBeNull()
    expect(screen.queryByLabelText(/Secret key（/)).toBeNull()
  })
  await act(async () => { await import('./AuthScreen') })
  expect(await screen.findByLabelText('用户名')).toBeTruthy()
  expect(controller.getSnapshot().session).toBeNull()
  expect(screen.queryByLabelText(/Secret key（/)).toBeNull()
})
it('向导复用实际表单，配置写入未完成时不能提前进入下一步', async () => {
  const pending = deferred<Response>()
  const fetch = vi.fn(async (_url: string, options: RequestInit) => options.method === 'PUT' ? pending.promise : response(paymentFixture()))
  vi.stubGlobal('fetch', fetch)
  const api = fakeAPI()
  vi.mocked(api.wizard).mockResolvedValue({ needed: true, configured: false })
  const controller = new SessionController(api)
  mount('/payment', account(), controller)
  await screen.findByLabelText('Publishable key')
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: /配好了，下一步$/ }))
  expect(await screen.findByText('请先保存当前配置，或选择“稍后再配”。')).toBeTruthy()
  submit('Publishable key')
  await waitFor(() => expect(fetch).toHaveBeenCalledWith('/admin/api/settings/payment', expect.objectContaining({ method: 'PUT', body: JSON.stringify({ stripe: { mode: 'test', publishable: 'fixture-public', secret: '', webhook_secret: '' } }) })))
  await waitFor(() => expect((screen.getByRole('button', { name: /配好了，下一步$/ }) as HTMLButtonElement).disabled).toBe(true))
  const reads = vi.mocked(api.wizard).mock.calls.length
  await user.click(screen.getByRole('button', { name: /配好了，下一步$/ }))
  await user.click(screen.getByRole('button', { name: /稍后再配$/ }))
  expect(api.wizard).toHaveBeenCalledTimes(reads)
  expect(api.skipWizard).not.toHaveBeenCalled()
  expect(controller.getSnapshot().wizard).toBe('payment')
  await act(async () => pending.resolve(response(paymentFixture())))
  expect(await screen.findByText('Stripe 已保存，收款配置已即时生效。')).toBeTruthy()
  await waitFor(() => expect((screen.getByRole('button', { name: /配好了，下一步$/ }) as HTMLButtonElement).disabled).toBe(false))
  vi.mocked(api.wizard).mockImplementation(async step => ({ needed: step !== 'payment', configured: step === 'payment' }))
  fetch.mockImplementation(async () => response(domainFixture()))
  await user.click(screen.getByRole('button', { name: /配好了，下一步$/ }))
  expect(await screen.findByLabelText('你的店铺域名')).toBeTruthy()
  expect(controller.getSnapshot().wizard).toBe('domain')
})

it('店主可测试环境变量 SMTP，无法写入环境配置', async () => {
  const data = { ...smtpFixture(), source: 'env', readonly: true }
  const fetch = vi.fn(async () => response(data)); vi.stubGlobal('fetch', fetch)
  mount('/smtp')
  expect((await screen.findByLabelText('SMTP 主机') as HTMLInputElement).disabled).toBe(true)
  expect(screen.queryByRole('button', { name: '保存 SMTP' })).toBeNull()
  field('发送测试邮件到', 'test@example.com'); submit('发送测试邮件到')
  expect(await screen.findByText('测试邮件已发送，请查收（含垃圾箱）。')).toBeTruthy()
  expect(fetch).toHaveBeenLastCalledWith('/admin/api/smtp/test', expect.objectContaining({ method: 'POST', body: '{"to":"test@example.com"}' }))
})

it('环境域名不能通过表单提交绕过只读状态', async () => {
  const fetch = vi.fn(async () => response({ ...domainFixture(), source: 'env', readonly: true })); vi.stubGlobal('fetch', fetch)
  mount('/domain')
  expect((await screen.findByLabelText('你的店铺域名') as HTMLInputElement).disabled).toBe(true)
  expect(screen.queryByRole('button', { name: '保存域名' })).toBeNull()
  submit('你的店铺域名')
  await act(async () => {})
  expect(fetch).toHaveBeenCalledTimes(1)
})
