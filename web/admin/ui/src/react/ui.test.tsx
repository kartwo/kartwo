// 后台交互验证 / Admin Interaction Tests
// 功能：真实 React 表单与路由权限、错误反馈、邀请验证
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:14:20

import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { expect, it, vi } from 'vitest'
import { AdminTheme } from './theme'
import { AdminRoot } from './AdminRoot'
import { SessionProvider } from './SessionProvider'
import { SessionController } from './session'
import { APIError } from './transport'
import { account, fakeAPI } from './test-fixtures'

function mount(controller: SessionController, path = '/dashboard') {
  const page = vi.fn(route => <div>已加载：{route.title}</div>)
  render(<MemoryRouter initialEntries={[path]}><AdminTheme><SessionProvider controller={controller}>
    <AdminRoot renderPage={page} renderWizard={step => <div>设置页：{step}</div>} />
  </SessionProvider></AdminTheme></MemoryRouter>)
  return page
}
it('输入用户名与密码可登录，429 提示可见且不创建会话', async () => {
  const api = fakeAPI(account('staff', ['dashboard.view']))
  vi.mocked(api.me).mockRejectedValueOnce(new APIError(401, '请登录'))
  vi.mocked(api.login).mockRejectedValueOnce(new APIError(429, '登录尝试过多，请稍后再试'))
  const controller = new SessionController(api)
  mount(controller)
  const user = userEvent.setup()
  await user.type(await screen.findByLabelText('用户名'), 'fixture')
  await user.type(screen.getByLabelText('口令'), 'fixture-password')
  await user.click(screen.getByRole('button', { name: /登\s*录\s*后\s*台/ }))
  expect(await screen.findByText('登录尝试过多，请稍后再试')).toBeTruthy()
  expect(controller.getSnapshot().session).toBeNull()
  expect((screen.getByLabelText('口令') as HTMLInputElement).value).toBe('')
  await user.type(screen.getByLabelText('口令'), 'fixture-password')
  await user.click(screen.getByRole('button', { name: /登\s*录\s*后\s*台/ }))
  expect(await screen.findByText('已加载：概览')).toBeTruthy()
  expect(api.login).toHaveBeenCalledWith('fixture', 'fixture-password')
})
it('无权限页面不渲染也不发起页面请求，失效后卸载页面', async () => {
  const controller = new SessionController(fakeAPI(account('staff', ['blog.view'])))
  const page = mount(controller, '/orders')
  expect(await screen.findByText('当前账号没有访问此页面的权限')).toBeTruthy()
  expect(page).not.toHaveBeenCalled()
  expect(screen.queryByRole('menuitem', { name: '用户与角色' })).toBeNull()
  expect(screen.getByRole('menuitem', { name: '博客文章' })).toBeTruthy()
  await act(async () => { controller.unauthorized() })
  expect(await screen.findByLabelText('用户名')).toBeTruthy()
  expect(screen.queryByRole('navigation', { name: '后台导航' })).toBeNull()
})
it('邀请确认不一致时不调用服务，成功清空口令', async () => {
  const api = fakeAPI()
  const controller = new SessionController(api)
  mount(controller, '/accept-invite?token=fixture-invite')
  const password = await screen.findByLabelText('新密码')
  fireEvent.change(password, { target: { value: 'fixture-password' } })
  fireEvent.change(screen.getByLabelText('确认密码'), { target: { value: 'different-password' } })
  fireEvent.submit(password.closest('form')!)
  expect(await screen.findByText('两次输入的密码不一致')).toBeTruthy()
  expect(api.acceptInvite).not.toHaveBeenCalled()
  fireEvent.change(screen.getByLabelText('确认密码'), { target: { value: 'fixture-password' } })
  fireEvent.submit(password.closest('form')!)
  expect(await screen.findByText('密码已设置')).toBeTruthy()
  expect(api.acceptInvite).toHaveBeenCalledWith('fixture-invite', 'fixture-password')
  expect(screen.queryByLabelText('新密码')).toBeNull()
})
it('店主登出失败保留页面并提示，成功后回到登录', async () => {
  const api = fakeAPI()
  vi.mocked(api.logout).mockRejectedValueOnce(new Error('无法登出，请重试'))
  const controller = new SessionController(api)
  mount(controller, '/blog')
  await screen.findByText('已加载：博客文章')
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: /登\s*出/ }))
  expect(await screen.findByText('无法登出，请重试')).toBeTruthy()
  expect(screen.getByText('已加载：博客文章')).toBeTruthy()
  await user.click(screen.getByRole('button', { name: /登\s*出/ }))
  await waitFor(() => expect(controller.getSnapshot().phase).toBe('login'))
  expect(await screen.findByLabelText('用户名')).toBeTruthy()
})

it('缺少邀请令牌不能提交口令', async () => {
  const api = fakeAPI()
  mount(new SessionController(api), '/accept-invite')
  expect(await screen.findByText('邀请链接不完整，请联系店主重新发送。')).toBeTruthy()
  expect(screen.queryByLabelText('新密码')).toBeNull()
  expect(api.acceptInvite).not.toHaveBeenCalled()
})

it('手机菜单选择当前页面也收起抽屉，不依赖地址变化', async () => {
  const controller = new SessionController(fakeAPI())
  render(<MemoryRouter initialEntries={['/orders']}><AdminTheme><ConfigProvider theme={{ token: { motion: false } }}><SessionProvider controller={controller}>
    <AdminRoot renderPage={() => <div>当前订单页</div>} />
  </SessionProvider></ConfigProvider></AdminTheme></MemoryRouter>)
  await screen.findByText('当前订单页')
  await userEvent.click(screen.getByRole('button', { name: '打开菜单' }))
  const dialog = await screen.findByRole('dialog', { name: 'Kartwo 菜单' })
  await userEvent.click(within(dialog).getByRole('link', { name: '订单' }))
  await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Kartwo 菜单' })).toBeNull())
  expect(screen.getByText('当前订单页')).toBeTruthy()
  expect(screen.getByRole('button', { name: '打开菜单' }).getAttribute('aria-expanded')).toBe('false')
})
