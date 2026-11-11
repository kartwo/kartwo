// 会话客户端 / Session API Client
// 功能：类型化现有认证、邀请、演示与开店向导接口
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:10:05

import { request } from './transport'
import { parseSession, parseStatus, parseWizard, type WizardStep } from './contracts'

export const sessionAPI = {
  status: async (signal?: AbortSignal) => parseStatus(await request('GET', '/status', undefined, signal)),
  me: async (signal?: AbortSignal) => parseSession(await request('GET', '/me', undefined, signal)),
  setup: (username: string, password: string) => request('POST', '/setup', { username, password }),
  login: (username: string, password: string) => request('POST', '/login', { username, password }),
  logout: () => request('POST', '/logout'),
  demo: () => request('POST', '/demo-session'),
  resetDemo: () => request('POST', '/demo/reset'),
  acceptInvite: (token: string, password: string) => request('POST', '/invites/accept', { token, password }),
  wizard: async (step: WizardStep, signal?: AbortSignal) => parseWizard(await request('GET', `/wizard/${step}`, undefined, signal)),
  skipWizard: (step: WizardStep) => request('POST', `/wizard/${step}/skip`),
}
export type SessionAPI = typeof sessionAPI
