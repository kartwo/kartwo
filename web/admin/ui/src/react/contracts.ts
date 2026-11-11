// 会话接口契约 / Session API Contracts
// 功能：校验服务端身份和向导结构，不把未知身份默认为店主
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:10:05

export type AccountRole = 'owner' | 'staff' | 'demo'
export interface Session {
  username: string
  public_id: string
  role: AccountRole
  permissions: string[]
  expires_at: string
  demo_max_products: number
  demo_max_image_bytes: number
}
export interface Status { initialized: boolean; demo_mode: boolean }
export type WizardStep = 'payment' | 'domain' | 'smtp'
export interface WizardStatus { needed: boolean; configured: boolean }

function object(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('服务器响应格式不正确，请重试。')
  return value as Record<string, unknown>
}
export function parseStatus(value: unknown): Status {
  const data = object(value)
  if (typeof data.initialized !== 'boolean' || typeof data.demo_mode !== 'boolean') throw new Error('无法确认后台初始化状态，请重试。')
  return { initialized: data.initialized, demo_mode: data.demo_mode }
}
export function parseSession(value: unknown): Session {
  const data = object(value)
  if (!['owner', 'staff', 'demo'].includes(String(data.role)) ||
      typeof data.username !== 'string' || typeof data.public_id !== 'string' ||
      !Array.isArray(data.permissions) || !data.permissions.every((p: unknown) => typeof p === 'string') ||
      typeof data.expires_at !== 'string' || !Number.isFinite(Date.parse(data.expires_at))) {
    throw new Error('无法确认账号身份，请重新登录。')
  }
  return {
    username: data.username, public_id: data.public_id, role: data.role as AccountRole,
    permissions: data.permissions as string[], expires_at: data.expires_at,
    demo_max_products: typeof data.demo_max_products === 'number' ? data.demo_max_products : 0,
    demo_max_image_bytes: typeof data.demo_max_image_bytes === 'number' ? data.demo_max_image_bytes : 0,
  }
}
export function parseWizard(value: unknown): WizardStatus {
  const data = object(value)
  if (typeof data.needed !== 'boolean' || typeof data.configured !== 'boolean') throw new Error('无法确认开店进度，请重试。')
  return { needed: data.needed, configured: data.configured }
}
