// 配置测试样本 / Settings Test Fixtures
// 功能：模拟服务端实际配置响应，仅使用无效样本凭证
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:25:50

import type { PaymentStatus, DomainStatus, SmtpStatus } from './settings-api'
export function paymentFixture(): PaymentStatus {
  return {
    stripe: { source: 'db', readonly: false, mode: 'test', publishable: 'fixture-public', has_secret: true, has_webhook: true },
    paypal: { source: 'db', readonly: false, mode: 'sandbox', client_id: 'fixture-client', webhook_id: 'fixture-webhook', has_secret: true },
  }
}
export function domainFixture(): DomainStatus { return { source: 'db', readonly: false, domain: 'shop.example.com', https_capable: true } }
export function smtpFixture(): SmtpStatus {
  return { source: 'db', readonly: false, host: 'smtp.example.com', port: '587', username: 'fixture@example.com', from_address: 'shop@example.com', from_name: 'Fixture Shop', encryption: 'starttls', configured: true, has_password: true }
}
