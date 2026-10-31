// 后台正式入口 / Admin Production Entry
// 功能：装配唯一 HashRouter、主题和会话，严格模式检查生命周期，业务页面按路由加载
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:56:12

import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { HashRouter } from 'react-router-dom'
import { AdminTheme } from './theme'
import { SessionProvider } from './SessionProvider'
import { PageBoundary } from './SettingsPages'
import { PageRouter } from './PageRouter'
const root = document.getElementById('app')
if (!root) throw new Error('后台入口节点不存在')
createRoot(root).render(<StrictMode><HashRouter><AdminTheme><PageBoundary><SessionProvider><PageRouter /></SessionProvider></PageBoundary></AdminTheme></HashRouter></StrictMode>)
