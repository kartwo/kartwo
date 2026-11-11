// 配置契约测试 / Settings Contract Tests
// 功能：验证精确写入字段、凭证空值与空白语义，拒绝未识别来源
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:25:50

import { expect, it, vi } from 'vitest'
import { settingsAPI, parsePayment, parseDomain, parseSmtp } from './settings-api'
import { paymentFixture, domainFixture, smtpFixture } from './settings-fixtures'

it('分通道提交，绝不透传来源/只读/密钥状态字段', async () => {
  const fetch = vi.fn(async () => new Response(JSON.stringify(paymentFixture())))
  vi.stubGlobal('fetch', fetch)
  await settingsAPI.savePayment({ stripe: { ...paymentFixture().stripe, secret: '', webhook_secret: '', publishable: ' fixture-public ' } })
  await settingsAPI.savePayment({ paypal: { ...paymentFixture().paypal, secret: ' fixture-secret ', webhook_id: ' fixture-webhook ' } })
  expect(fetch).toHaveBeenNthCalledWith(1, '/admin/api/settings/payment', expect.objectContaining({ method: 'PUT', body: JSON.stringify({ stripe: { mode: 'test', publishable: 'fixture-public', secret: '', webhook_secret: '' } }) }))
  expect(fetch).toHaveBeenNthCalledWith(2, '/admin/api/settings/payment', expect.objectContaining({ method: 'PUT', body: JSON.stringify({ paypal: { mode: 'sandbox', client_id: 'fixture-client', secret: 'fixture-secret', webhook_id: 'fixture-webhook' } }) }))
})
it('SMTP 端口保持字符串、密码空白原样保留，不发送只读状态', async () => {
  const fetch = vi.fn(async () => new Response(JSON.stringify(smtpFixture())))
  vi.stubGlobal('fetch', fetch)
  await settingsAPI.saveSmtp({ ...smtpFixture(), password: ' fixture-password ', host: ' smtp.example.com ' })
  expect(fetch).toHaveBeenCalledWith('/admin/api/settings/smtp', expect.objectContaining({ method: 'PUT', body: JSON.stringify({ host: 'smtp.example.com', port: '587', username: 'fixture@example.com', password: ' fixture-password ', from_address: 'shop@example.com', from_name: 'Fixture Shop', encryption: 'starttls' }) }))
})
it('连接测试只提交测试动作，不携带未保存凭证；域名只提交域名', async () => {
  const fetch = vi.fn(async () => new Response(JSON.stringify(domainFixture())))
  vi.stubGlobal('fetch', fetch)
  await settingsAPI.testStripe()
  await settingsAPI.testSmtp(' test@example.com ')
  await settingsAPI.saveDomain(' shop.example.com ')
  expect(fetch).toHaveBeenNthCalledWith(1, '/admin/api/settings/payment/stripe/test', expect.not.objectContaining({ body: expect.anything() }))
  expect(fetch).toHaveBeenNthCalledWith(2, '/admin/api/smtp/test', expect.objectContaining({ method: 'POST', body: '{"to":"test@example.com"}' }))
  expect(fetch).toHaveBeenNthCalledWith(3, '/admin/api/settings/domain', expect.objectContaining({ method: 'PUT', body: '{"domain":"shop.example.com"}' }))
})
it('响应白名单不引入意外回传的秘密，未知来源或缺状态时拒绝编辑', () => {
  const payment = paymentFixture()
  expect(parsePayment({ ...payment, stripe: { ...payment.stripe, secret: 'must-not-store', source: 'env', readonly: false } }).stripe).toEqual({ ...payment.stripe, source: 'env', readonly: true })
  expect(parseSmtp({ ...smtpFixture(), password: 'must-not-store' })).not.toHaveProperty('password')
  expect(() => parsePayment({ ...payment, stripe: { ...payment.stripe, source: 'unknown' } })).toThrow()
  expect(() => parseDomain({ ...domainFixture(), readonly: undefined })).toThrow()
  expect(() => parseSmtp({ ...smtpFixture(), encryption: 'unknown' })).toThrow()
})
