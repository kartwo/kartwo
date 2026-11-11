// 完整后台页面路由 / Complete Admin Page Router
// 功能：合并所有已迁移业务与设置页面，逐地址隔离惰性加载错误，保留鉴权前置
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:56:12

import { Suspense, type ComponentType, type LazyExoticComponent } from 'react'
import { useLocation } from 'react-router-dom'
import { AdminRoot } from './AdminRoot'
import { businessPages } from './BusinessPages'
import { settingsPages, PageBoundary, PageLoading } from './SettingsPages'
export const pageComponents: Readonly<Record<string, LazyExoticComponent<ComponentType>>> = { ...businessPages, ...settingsPages }
export function PageRouter() {
  const { pathname } = useLocation()
  return <AdminRoot renderPage={route => {
    const Page = pageComponents[route.path]
    if (!Page) throw new Error('已授权页面缺少实现')
    return <PageBoundary key={pathname}><Suspense fallback={<PageLoading />}><Page /></Suspense></PageBoundary>
  }} />
}
