// 配置路由与向导 / Settings Routes and Wizard
// 功能：按需加载实际配置页面，同一表单复用于正式路由与向导
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:24:05

import { ReloadOutlined } from '@ant-design/icons'
import { Component, lazy, Suspense, type ReactNode } from 'react'
import { Button, Result, Spin } from 'antd'
import type { WizardStep } from './contracts'
import { useWizardActivity } from './WizardActivity'

export const settingsPages = {
  '/payment': lazy(() => import('./pages/PaymentSettings')),
  '/domain': lazy(() => import('./pages/DomainSettings')),
  '/smtp': lazy(() => import('./pages/SmtpSettings')),
  '/shop': lazy(() => import('./pages/ShopSettings')),
  '/blog-settings': lazy(() => import('./pages/BlogSettings')),
  '/contact-settings': lazy(() => import('./pages/ContactSettings')),
  '/account': lazy(() => import('./pages/AccountSettings')),
  '/translation': lazy(() => import('./pages/TranslationSettings')),
  '/backup': lazy(() => import('./pages/BackupSettings')),
  '/shipping': lazy(() => import('./pages/ShippingSettings')),
  '/policies': lazy(() => import('./pages/PolicySettings')),
  '/users': lazy(() => import('./pages/UsersRoles')),
}
export class PageBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false }
  static getDerivedStateFromError() { return { failed: true } }
  render() {
    return this.state.failed ? <Result status="error" title="页面加载失败" subTitle="请刷新页面后重试。" extra={<Button icon={<ReloadOutlined aria-hidden />} onClick={() => window.location.reload()}>刷新页面</Button>} /> : this.props.children
  }
}
export function PageLoading() {
  return <div className="page-loading" role="status"><Spin /><span>正在加载页面…</span></div>
}
function WizardLoading() {
  useWizardActivity(true)
  return <PageLoading />
}
export function WizardSettings({ step }: { step: WizardStep }) {
  const Settings = settingsPages[`/${step}`]
  return <PageBoundary key={step}><Suspense fallback={<WizardLoading />}><Settings /></Suspense></PageBoundary>
}
