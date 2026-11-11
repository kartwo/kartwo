// 会话测试样本 / Session Test Fixtures
// 功能：仅用于隔离测试，模拟真实 API 结构与不同角色
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:13:54

import { vi } from 'vitest'
import type { Session, AccountRole } from './contracts'
import type { SessionAPI } from './session-api'
export function account(role: AccountRole = 'owner', permissions: string[] = []): Session {
  return { username: 'fixture', public_id: 'fixture-public-id', role, permissions,
    expires_at: new Date(Date.now() + 3600000).toISOString(), demo_max_products: 3, demo_max_image_bytes: 2097152 }
}
export function fakeAPI(session = account()): SessionAPI {
  return {
    status: vi.fn(async () => ({ initialized: true, demo_mode: false })),
    me: vi.fn(async () => session), setup: vi.fn(async () => ({})), login: vi.fn(async () => ({})),
    logout: vi.fn(async () => ({})), demo: vi.fn(async () => ({})), resetDemo: vi.fn(async () => ({})),
    acceptInvite: vi.fn(async () => ({})), wizard: vi.fn(async () => ({ needed: false, configured: true })),
    skipWizard: vi.fn(async () => ({})),
  }
}
export function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
