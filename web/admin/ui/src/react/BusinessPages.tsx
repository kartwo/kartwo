// 业务页面路由 / Business Page Routes
// 功能：逐页迁移经营及系统页面，以动态导入接入最终唯一后台入口
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:17:21

import { lazy } from 'react'
export const businessPages = {
  '/contact': lazy(() => import('./pages/ContactInbox')),
  '/audit': lazy(() => import('./pages/AuditEvents')),
  '/diagnostics': lazy(() => import('./pages/Diagnostics')),
}
