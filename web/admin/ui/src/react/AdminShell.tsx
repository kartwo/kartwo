// 后台布局 / Admin Shell
// 功能：Ant Design 响应式导航、独立博客菜单及演示倒计时与重置
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:12:07

import { useEffect, useState, type ReactNode } from 'react'
import { Alert, App, Avatar, Button, Drawer, Layout, Menu, Space, Typography, type MenuProps } from 'antd'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { matchRoute, navigation, sections, selectedPath } from './access'
import { useSession } from './SessionProvider'
import { APIError, errorMessage } from './transport'

export function AdminShell({ children }: { children: ReactNode }) {
  const { state, controller } = useSession()
  const { session } = state
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const { modal, message } = App.useApp()
  const [mobileOpen, setMobileOpen] = useState(false)
  const [expanded, setExpanded] = useState<string[]>(['business'])
  const [now, setNow] = useState(Date.now())
  const [resetting, setResetting] = useState(false)
  const current = matchRoute(pathname)
  useEffect(() => {
    setMobileOpen(false)
    if (current?.section) setExpanded(keys => [...new Set([...keys, current.section!])])
  }, [pathname, current?.section])
  useEffect(() => {
    if (session?.role !== 'demo') return
    const clock = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(clock)
  }, [session?.role])
  useEffect(() => {
    if (session?.role === 'demo' && Date.parse(session.expires_at) <= now) controller.unauthorized()
  }, [session, now, controller])
  if (!session) return null
  const allowed = navigation(session)
  const item = (path: string, title: string) => ({ key: path, label: <Link to={path}>{title}</Link> })
  const menuItems: MenuProps['items'] = []
  if (allowed.some(route => route.path === '/dashboard')) menuItems.push(item('/dashboard', '概览'))
  for (const section of sections) {
    const children = allowed.filter(route => route.section === section.key).map(route => item(route.path, route.title))
    if (children.length) menuItems.push({ key: section.key, label: section.title, children })
    if (section.key === 'catalog' && allowed.some(route => route.path === '/blog')) menuItems.push(item('/blog', '博客文章'))
  }
  const sidebar = <>
    <div className="sidebar-brand"><span className="brand-mark" aria-hidden="true">K</span><div><strong>Kartwo</strong><small>店铺运营后台</small></div></div>
    <nav aria-label="后台导航"><Menu mode="inline" theme="dark" items={menuItems} selectedKeys={[selectedPath(pathname)]} openKeys={expanded} onOpenChange={setExpanded} onClick={() => setMobileOpen(false)} /></nav>
  </>
  const remaining = Math.max(0, Math.ceil((Date.parse(session.expires_at) - now) / 1000))
  const remainingText = `${Math.floor(remaining / 60).toString().padStart(2, '0')}:${(remaining % 60).toString().padStart(2, '0')}`
  const resetDemo = () => modal.confirm({
    title: '重置本次演示？', content: '清空本次演示创建的临时商品与图片，示例数据不会受影响。',
    okText: '重置', cancelText: '取消',
    onOk: async () => {
      setResetting(true)
      try {
        await controller.api.resetDemo()
        // Session refresh unmounts the page and discards stale product/media state.
        await controller.refresh()
        navigate('/products', { replace: true })
      } catch (error) {
        if (error instanceof APIError && error.status === 401) controller.unauthorized()
        else { void message.error(errorMessage(error)); throw error }
      } finally { setResetting(false) }
    },
  })
  return <>
    {session.role === 'demo' && <Alert type="warning" title="公开演示模式" description={`可新建 ${session.demo_max_products} 个临时草稿商品；示例商品和关键配置受保护。剩余 ${remainingText}`} action={<Button loading={resetting} onClick={resetDemo}>重置本次演示</Button>} />}
    <Layout className="admin-layout">
      <Layout.Sider width={244} className="admin-sidebar">{sidebar}</Layout.Sider>
      <Drawer title="Kartwo 菜单" placement="left" open={mobileOpen} onClose={() => setMobileOpen(false)} className="admin-drawer">{sidebar}</Drawer>
      <Layout>
        <Layout.Header className="admin-header">
          <Space><Button className="mobile-menu-button" aria-label="打开菜单" aria-expanded={mobileOpen} onClick={() => setMobileOpen(true)}>菜单</Button>
            <div><Typography.Text type="secondary">{sections.find(section => section.key === current?.section)?.title ?? '工作台'}</Typography.Text><Typography.Title level={2}>{current?.title ?? '后台'}</Typography.Title></div>
          </Space>
          <Space><Avatar>{session.username.slice(0, 1).toUpperCase()}</Avatar><span className="account-name">{session.username}</span><Button loading={state.busy} onClick={() => void controller.logout()}>登出</Button></Space>
        </Layout.Header>
        <Layout.Content className="admin-content">
          {state.error && <Alert type="error" showIcon title={state.error} />}
          {session.role === 'demo' && current?.demoReadonly && <Alert type="info" showIcon title="公开演示只读预览" description="敏感信息已隐藏；此页面的修改、导入、导出与订单操作不可执行。" />}
          {children}
        </Layout.Content>
      </Layout>
    </Layout>
  </>
}
