// 后台入口组合 / Admin Root Composition
// 功能：组合鉴权、路由权限和向导，按会话状态惰性加载认证或导航外壳
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:12:07

import { lazy, Suspense, useState, type ReactNode } from 'react'
import { Alert, Button, Result, Space, Steps } from 'antd'
import { Navigate, useLocation } from 'react-router-dom'
import { canVisit, homePath, matchRoute, type AdminRoute } from './access'
import type { WizardStep } from './contracts'
import { InsecureNotice } from './InsecureNotice'
import { useSession } from './SessionProvider'
import { WizardSettings, PageLoading } from './SettingsPages'
import { WizardActivity } from './WizardActivity'
const AuthScreen = lazy(() => import('./AuthScreen').then(module => ({ default: module.AuthScreen })))
const AdminShell = lazy(() => import('./AdminShell').then(module => ({ default: module.AdminShell })))

interface Props {
  renderPage: (route: AdminRoute) => ReactNode
  renderWizard?: (step: WizardStep) => ReactNode
}
export function AdminRoot({ renderPage, renderWizard }: Props) {
  const { state, controller } = useSession()
  const { pathname } = useLocation()
  const session = state.session
  const [wizardBusy, setWizardBusy] = useState(false)
  let content: ReactNode
  if (state.phase === 'loading' && pathname !== '/accept-invite') content = <div className="auth-screen"><PageLoading /></div>
  else if (pathname === '/accept-invite' || state.phase !== 'ready' || !session) content = <AuthScreen />
  else if (state.wizard) {
    const step = state.wizard
    const stepNumber = ['payment', 'domain', 'smtp'].indexOf(step)
    content = <main className="wizard-content">
      <div className="wizard-header"><h1>Kartwo · 开店向导</h1><Button onClick={() => void controller.logout()} loading={state.busy}>登出</Button></div>
      <Steps current={stepNumber} items={[{ title: '收款' }, { title: '域名' }, { title: '邮件' }]} />
      {state.error && <Alert type="error" showIcon title={state.error} />}
      {step === 'smtp' && <Alert type="info" title="配置邮件后可发送订单确认信，并帮助顾客通过邮箱在其他设备查看订单。" />}
      <WizardActivity.Provider value={setWizardBusy}><div key={step}>{renderWizard ? renderWizard(step) : <WizardSettings step={step} />}</div></WizardActivity.Provider>
      <Space wrap>
        {stepNumber > 0 && <Button disabled={state.busy || wizardBusy} onClick={controller.previousWizard}>上一步</Button>}
        <Button disabled={state.busy || wizardBusy} onClick={() => void controller.advanceWizard(true)}>稍后再配</Button>
        <Button type="primary" loading={state.busy} disabled={wizardBusy} onClick={() => void controller.advanceWizard(false)}>{step === 'smtp' ? '完成，进入后台' : '配好了，下一步'}</Button>
      </Space>
    </main>
  } else if (pathname === '/' || pathname === '/market') content = <Navigate replace to={pathname === '/market' ? '/payment' : homePath(session)} />
  else {
    const route = matchRoute(pathname)
    content = <AdminShell>{!route ? <Result status="404" title="页面不存在" /> : !canVisit(session, route) ?
      <Result status="403" title="当前账号没有访问此页面的权限" /> : renderPage(route)}</AdminShell>
  }
  // Private branches must unmount on identity/state changes. The public one-time invitation is independent of bootstrap authentication.
  const branchKey = pathname === '/accept-invite' ? 'invitation' : `${state.phase}:${session?.public_id ?? ''}`
  return <><InsecureNotice /><Suspense key={branchKey} fallback={<PageLoading />}>{content}</Suspense></>
}
