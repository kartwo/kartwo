// 会话安全验证 / Session Safety Tests
// 功能：覆盖退出竞态、重复提交、向导失败与初始化后的登录重试
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:13:54

import { expect, it, vi } from 'vitest'
import { SessionController } from './session'
import { APIError } from './transport'
import { account, deferred, fakeAPI } from './test-fixtures'

it('只有 401 显示登录，服务失败不能误判未初始化或跳过向导', async () => {
  const api = fakeAPI()
  vi.mocked(api.me).mockRejectedValueOnce(new APIError(401, '请登录'))
  const controller = new SessionController(api)
  await controller.refresh()
  expect(controller.getSnapshot().phase).toBe('login')
  vi.mocked(api.status).mockRejectedValueOnce(new Error('网络故障'))
  await controller.refresh()
  expect(controller.getSnapshot()).toMatchObject({ phase: 'error', session: null, error: '网络故障' })
  vi.mocked(api.wizard).mockRejectedValueOnce(new Error('向导失败'))
  await controller.refresh()
  expect(controller.getSnapshot()).toMatchObject({ phase: 'error', session: null, error: '向导失败' })
})
it('员工和演示不查询店主向导，权限来自当前会话', async () => {
  for (const role of ['staff', 'demo'] as const) {
    const api = fakeAPI(account(role))
    const controller = new SessionController(api)
    await controller.refresh()
    expect(controller.getSnapshot()).toMatchObject({ phase: 'ready', wizard: null, session: { role } })
    expect(api.wizard).not.toHaveBeenCalled()
  }
})
it('旧刷新结果不能覆盖退出或更新后的员工身份', async () => {
  const api = fakeAPI()
  const pending = deferred<ReturnType<typeof account>>()
  vi.mocked(api.me).mockReturnValueOnce(pending.promise)
  const controller = new SessionController(api)
  const old = controller.refresh()
  await vi.waitFor(() => expect(api.me).toHaveBeenCalledTimes(1))
  controller.unauthorized()
  vi.mocked(api.me).mockResolvedValue(account('staff', ['blog.view']))
  await controller.refresh()
  pending.resolve(account('owner'))
  await old
  expect(controller.getSnapshot().session?.role).toBe('staff')
})
it('登出取消未完成的刷新，不伪装服务端登出失败为成功', async () => {
  const api = fakeAPI()
  const controller = new SessionController(api)
  await controller.refresh()
  vi.mocked(api.logout).mockRejectedValueOnce(new Error('暂时无法登出'))
  await controller.logout()
  expect(controller.getSnapshot()).toMatchObject({ phase: 'ready', error: '暂时无法登出' })
  const pending = deferred<ReturnType<typeof account>>()
  vi.mocked(api.me).mockReturnValueOnce(pending.promise)
  const refresh = controller.refresh()
  await vi.waitFor(() => expect(api.me).toHaveBeenCalledTimes(2))
  await controller.logout()
  pending.resolve(account())
  await refresh
  expect(controller.getSnapshot()).toMatchObject({ phase: 'login', session: null, wizard: null })
})
it('重复登录只发送一次；setup 已完成但 login 失败后转为登录重试', async () => {
  const api = fakeAPI()
  const pending = deferred<unknown>()
  vi.mocked(api.login).mockReturnValueOnce(pending.promise)
  const controller = new SessionController(api)
  const first = controller.authenticate('login', 'fixture', 'fixture-password')
  await controller.authenticate('login', 'fixture', 'fixture-password')
  expect(api.login).toHaveBeenCalledTimes(1)
  pending.resolve({})
  await first
  vi.mocked(api.login).mockRejectedValueOnce(new APIError(429, '稍后再试'))
  await controller.authenticate('setup', 'fixture', 'fixture-password')
  expect(api.setup).toHaveBeenCalledTimes(1)
  expect(controller.getSnapshot()).toMatchObject({ phase: 'login', error: '稍后再试', busy: false })
})
it('三步向导保持顺序、支持返回；后续查询失败留在当前步骤', async () => {
  const api = fakeAPI()
  vi.mocked(api.wizard).mockResolvedValue({ needed: true, configured: false })
  const controller = new SessionController(api)
  await controller.refresh()
  expect(controller.getSnapshot().wizard).toBe('payment')
  await controller.advanceWizard(true)
  expect(api.skipWizard).toHaveBeenCalledWith('payment')
  expect(controller.getSnapshot().wizard).toBe('domain')
  controller.previousWizard()
  expect(controller.getSnapshot().wizard).toBe('payment')
  vi.mocked(api.wizard).mockRejectedValueOnce(new Error('无法连接'))
  await controller.advanceWizard(false)
  expect(controller.getSnapshot()).toMatchObject({ wizard: 'payment', error: '无法连接' })
  await controller.advanceWizard(false)
  await controller.advanceWizard(false)
  expect(controller.getSnapshot().wizard).toBe('smtp')
  await controller.advanceWizard(true)
  expect(controller.getSnapshot().wizard).toBeNull()
})
