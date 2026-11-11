// 配置接口契约 / Settings API Contracts
// 功能：明确解析收款、域名和 SMTP 状态，只提交接口允许的配置字段
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:21:49

import { request } from './transport'
import { record, string, bool, choice } from './api-values'
export type Source = 'db' | 'env' | 'demo' | 'none'
interface SettingsMeta { source: Source; readonly: boolean }
export type StripeFields = { mode: 'test' | 'live'; publishable: string; secret: string; webhook_secret: string }
export type PayPalFields = { mode: 'sandbox' | 'live'; client_id: string; secret: string; webhook_id: string }
export type StripeStatus = SettingsMeta & Omit<StripeFields, 'secret' | 'webhook_secret'> & { has_secret: boolean; has_webhook: boolean }
export type PayPalStatus = SettingsMeta & Omit<PayPalFields, 'secret'> & { has_secret: boolean }
export interface PaymentStatus { stripe: StripeStatus; paypal: PayPalStatus }
export type PaymentUpdate = { stripe: StripeFields } | { paypal: PayPalFields }
export type DomainStatus = SettingsMeta & { domain: string; https_capable: boolean }
export type SmtpFields = { host: string; port: string; username: string; password: string; from_address: string; from_name: string; encryption: 'starttls' | 'tls' | 'none' }
export type SmtpStatus = SettingsMeta & Omit<SmtpFields, 'password'> & { configured: boolean; has_password: boolean }

function meta(data: Record<string, unknown>): SettingsMeta {
  const source = choice(data, 'source', ['db', 'env', 'demo', 'none'] as const)
  return { source, readonly: bool(data, 'readonly') || source === 'env' || source === 'demo' }
}
export function parsePayment(value: unknown): PaymentStatus {
  const data = record(value), stripe = record(data.stripe), paypal = record(data.paypal)
  return {
    stripe: { ...meta(stripe), mode: choice(stripe, 'mode', ['test', 'live'] as const), publishable: string(stripe, 'publishable'), has_secret: bool(stripe, 'has_secret'), has_webhook: bool(stripe, 'has_webhook') },
    paypal: { ...meta(paypal), mode: choice(paypal, 'mode', ['sandbox', 'live'] as const), client_id: string(paypal, 'client_id'), webhook_id: string(paypal, 'webhook_id'), has_secret: bool(paypal, 'has_secret') },
  }
}
export function parseDomain(value: unknown): DomainStatus {
  const data = record(value)
  return { ...meta(data), domain: string(data, 'domain'), https_capable: bool(data, 'https_capable') }
}
export function parseSmtp(value: unknown): SmtpStatus {
  const data = record(value)
  return { ...meta(data), host: string(data, 'host'), port: string(data, 'port'), username: string(data, 'username'), from_address: string(data, 'from_address'), from_name: string(data, 'from_name'),
    encryption: choice(data, 'encryption', ['none', 'starttls', 'tls'] as const), configured: bool(data, 'configured'), has_password: bool(data, 'has_password') }
}
export const settingsAPI = {
  payment: async (signal?: AbortSignal) => parsePayment(await request('GET', '/settings/payment', undefined, signal)),
  savePayment: async (input: PaymentUpdate) => {
    const payload = 'stripe' in input ? { stripe: {
      mode: input.stripe.mode, publishable: input.stripe.publishable.trim(), secret: input.stripe.secret.trim(), webhook_secret: input.stripe.webhook_secret.trim(),
    } } : { paypal: {
      mode: input.paypal.mode, client_id: input.paypal.client_id.trim(), secret: input.paypal.secret.trim(), webhook_id: input.paypal.webhook_id.trim(),
    } }
    return parsePayment(await request('PUT', '/settings/payment', payload))
  },
  testStripe: () => request('POST', '/settings/payment/stripe/test'),
  domain: async (signal?: AbortSignal) => parseDomain(await request('GET', '/settings/domain', undefined, signal)),
  saveDomain: async (domain: string) => parseDomain(await request('PUT', '/settings/domain', { domain: domain.trim() })),
  smtp: async (signal?: AbortSignal) => parseSmtp(await request('GET', '/settings/smtp', undefined, signal)),
  saveSmtp: async (input: SmtpFields) => parseSmtp(await request('PUT', '/settings/smtp', {
    host: input.host.trim(), port: input.port.trim(), username: input.username.trim(),
    // SMTP passwords may contain leading/trailing spaces; never normalize them.
    password: input.password, from_address: input.from_address.trim(), from_name: input.from_name.trim(), encryption: input.encryption,
  })),
  testSmtp: (to: string) => request('POST', '/smtp/test', { to: to.trim() }),
}
