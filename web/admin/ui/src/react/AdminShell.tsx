// 后台布局 / Admin Shell
// 功能：Ant Design 响应式导航、独立博客菜单及演示倒计时与重置
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:12:07

import { useEffect, useState, type ReactNode } from 'react'
import { Alert, App, Avatar, Breadcrumb, Button, Drawer, Layout, Menu, Space, Tag, Tooltip, Typography, type MenuProps } from 'antd'
import { HomeOutlined, LogoutOutlined, MenuFoldOutlined, MenuOutlined, MenuUnfoldOutlined, ReloadOutlined, ShopOutlined, UserOutlined } from '@ant-design/icons'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { homePath, matchRoute, navigation, sections, selectedPath } from './access'
import { routeIcon, sectionIcon } from './navigation-icons'
import { useSession } from './SessionProvider'
import { APIError, errorMessage } from './transport'

export function AdminShell({ children }: { children: ReactNode }) {
  const { state, controller } = useSession()
  const { session } = state
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const { modal, message } = App.useApp()
  const [mobileOpen, setMobileOpen] = useState(false)
  const [collapsed, setCollapsed] = useState(false)
  const [expanded, setExpanded] = useState<string[]>(['business'])
  const [popupOpen, setPopupOpen] = useState<string[]>([])
  const [now, setNow] = useState(Date.now())
  const [resetting, setResetting] = useState(false)
  const current = matchRoute(pathname)
  useEffect(() => {
    setMobileOpen(false)
    setPopupOpen([])
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
  const item = (path: string, title: string) => ({ key: path, icon: routeIcon(matchRoute(path)), label: <Link to={path}>{title}</Link> })
  const menuItems: MenuProps['items'] = []
  if (allowed.some(route => route.path === '/dashboard')) menuItems.push(item('/dashboard', '概览'))
  for (const section of sections) {
    const children = allowed.filter(route => route.section === section.key).map(route => item(route.path, route.title))
    if (children.length) menuItems.push({ key: section.key, icon: sectionIcon(section.key), label: section.title, children })
    if (section.key === 'catalog' && allowed.some(route => route.path === '/blog')) menuItems.push(item('/blog', '博客文章'))
  }
  const sidebar = (mobile = false) => <>
    <Link to={homePath(session)} aria-label="Kartwo 后台首页" className={`sidebar-brand${collapsed && !mobile ? ' sidebar-brand-compact' : ''}`}><span className="brand-mark"><ShopOutlined aria-hidden /></span>{(!collapsed || mobile) && <div><strong>Kartwo</strong><small>店铺运营后台</small></div>}</Link>
    <nav aria-label="后台导航"><Menu mode="inline" theme="dark" inlineCollapsed={!mobile && collapsed} items={menuItems} selectedKeys={[selectedPath(pathname)]} openKeys={!mobile && collapsed ? popupOpen : expanded} onOpenChange={keys => { if (!mobile && collapsed) setPopupOpen(keys); else setExpanded(keys) }} onClick={() => setMobileOpen(false)} /></nav>
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
    {session.role === 'demo' && <Alert type="warning" title="公开演示模式" description={`可新建 ${session.demo_max_products} 个临时草稿商品；示例商品和关键配置受保护。剩余 ${remainingText}`} action={<Button icon={<ReloadOutlined aria-hidden />} loading={resetting} onClick={resetDemo}>重置本次演示</Button>} />}
    <Layout className="admin-layout">
      <Layout.Sider width={244} collapsed={collapsed} collapsible trigger={null} className="admin-sidebar">{sidebar()}</Layout.Sider>
      <Drawer title="Kartwo 菜单" placement="left" open={mobileOpen} onClose={() => setMobileOpen(false)} className="admin-drawer">{sidebar(true)}</Drawer>
      <Layout>
        <Layout.Header className="admin-header">
          <Space><Tooltip title={collapsed ? '展开导航' : '收起导航'}><Button type="text" className="desktop-menu-button" aria-label={collapsed ? '展开导航' : '收起导航'} aria-expanded={!collapsed} icon={collapsed ? <MenuUnfoldOutlined aria-hidden /> : <MenuFoldOutlined aria-hidden />} onClick={() => { setPopupOpen([]); setCollapsed(value => !value) }} /></Tooltip>
            <Button type="text" className="mobile-menu-button" aria-label="打开菜单" aria-expanded={mobileOpen} icon={<MenuOutlined aria-hidden />} onClick={() => setMobileOpen(true)}>菜单</Button>
            <div><Breadcrumb items={[{ title: <Link to={homePath(session)} aria-label="后台首页"><HomeOutlined aria-hidden /></Link> }, ...(current?.section ? [{ title: sections.find(section => section.key === current.section)?.title }] : []), { title: current?.title ?? '后台' }]} /><Typography.Title level={2}>{current?.title ?? '后台'}</Typography.Title></div>
          </Space>
          <Space><Avatar className="admin-avatar" icon={<UserOutlined aria-hidden />} /><span className="account-name">{session.username}</span><Tag className="account-role" color="blue">{{ owner: '店主', staff: '员工', demo: '演示' }[session.role]}</Tag><Button icon={<LogoutOutlined aria-hidden />} loading={state.busy} onClick={() => void controller.logout()}>登出</Button></Space>
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
