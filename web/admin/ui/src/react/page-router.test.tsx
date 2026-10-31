// 唯一入口集成测试 / Single Entry Integration Tests
// 功能：验证实际页面映射、HashRouter、严格模式生命周期与默认拒绝，不依赖旧后台运行时
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 13:02:07

import { lazy, StrictMode } from 'react'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { ConfigProvider } from 'antd'
import userEvent from '@testing-library/user-event'
import { HashRouter, MemoryRouter, useLocation, useNavigate } from 'react-router-dom'
import { expect, it, vi } from 'vitest'
import { AdminTheme } from './theme'
import { SessionProvider } from './SessionProvider'
import { SessionController } from './session'
import { PageRouter, pageComponents } from './PageRouter'
import { account, fakeAPI } from './test-fixtures'
function CurrentPath() { const { pathname } = useLocation(), navigate = useNavigate(); return <><output aria-label="当前路由">{pathname}</output><button onClick={() => void navigate('/about')}>测试打开关于</button><button onClick={() => void navigate('/missing-page')}>测试打开未知地址</button></> }
function Application({ controller }: { controller: SessionController }) { return <AdminTheme><ConfigProvider theme={{ token: { motion: false } }}><SessionProvider controller={controller}><CurrentPath /><PageRouter /></SessionProvider></ConfigProvider></AdminTheme> }
it('正式页面映射在严格模式的重复初始化后仍进入真实页面，不恢复过期会话', async () => {
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch)
  const controller = new SessionController(fakeAPI(account('staff')))
  render(<StrictMode><MemoryRouter initialEntries={['/about']}><Application controller={controller} /></MemoryRouter></StrictMode>)
  await screen.findByRole('link', { name: '查看 MIT 许可证' })
  expect(controller.getSnapshot().phase).toBe('ready'); expect(controller.getSnapshot().session?.role).toBe('staff'); expect(fetch).not.toHaveBeenCalled()
})
it('保留 /admin/#/about 地址并渲染真实页面，不依赖服务器处理 hash 业务路径', async () => {
  const original = window.location.href
  try {
    window.history.replaceState(null, '', '/admin/#/about')
    render(<HashRouter><Application controller={new SessionController(fakeAPI())} /></HashRouter>)
    await screen.findByRole('link', { name: '查看 MIT 许可证' }); expect(screen.getByLabelText('当前路由').textContent).toBe('/about')
  } finally { cleanup(); window.history.replaceState(null, '', original) }
})
it('根入口选择员工实际可访问的页面，旧 market 地址仍转向收款并校验权限', async () => {
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch)
  const app = render(<MemoryRouter initialEntries={['/']}><Application controller={new SessionController(fakeAPI(account('staff')))} /></MemoryRouter>)
  await screen.findByRole('link', { name: '查看 MIT 许可证' }); expect(screen.getByLabelText('当前路由').textContent).toBe('/about'); app.unmount()
  render(<MemoryRouter initialEntries={['/market']}><Application controller={new SessionController(fakeAPI(account('staff')))} /></MemoryRouter>)
  await screen.findByText('当前账号没有访问此页面的权限'); expect(screen.getByLabelText('当前路由').textContent).toBe('/payment'); expect(fetch).not.toHaveBeenCalled()
})
it('实际惰性页面映射仍在权限判断后运行，未知路径可恢复到其他页面', async () => {
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch)
  render(<MemoryRouter initialEntries={['/users']}><Application controller={new SessionController(fakeAPI(account('staff', ['users.manage'])))} /></MemoryRouter>)
  await waitFor(() => expect(screen.getByText('当前账号没有访问此页面的权限')).toBeTruthy()); expect(fetch).not.toHaveBeenCalled()
  await userEvent.click(screen.getByRole('button', { name: '测试打开未知地址' })); await screen.findByText('页面不存在')
  await userEvent.click(screen.getByRole('button', { name: '测试打开关于' })); await screen.findByRole('link', { name: '查看 MIT 许可证' }); expect(fetch).not.toHaveBeenCalled()
})
it('实际页面加载拒绝时显示通用重试提示，不泄漏错误细节，切换地址后可恢复', async () => {
  const original = pageComponents['/about']
  const log = vi.spyOn(console, 'error').mockImplementation(() => {})
  Object.defineProperty(pageComponents, '/about', { value: lazy(async () => { throw new Error('fixture-private-loader-detail') }), configurable: true })
  try {
    render(<MemoryRouter initialEntries={['/about']}><Application controller={new SessionController(fakeAPI(account('staff')))} /></MemoryRouter>)
    await screen.findByText('页面加载失败'); expect(screen.getByRole('button', { name: '刷新页面' })).toBeTruthy(); expect(document.body.textContent).not.toContain('fixture-private-loader-detail')
    await userEvent.click(screen.getByRole('button', { name: '测试打开未知地址' })); await screen.findByText('页面不存在')
    Object.defineProperty(pageComponents, '/about', { value: original, configurable: true })
    await userEvent.click(screen.getByRole('button', { name: '测试打开关于' })); await screen.findByRole('link', { name: '查看 MIT 许可证' })
    expect(log).toHaveBeenCalled()
  } finally { Object.defineProperty(pageComponents, '/about', { value: original, configurable: true }) }
})
