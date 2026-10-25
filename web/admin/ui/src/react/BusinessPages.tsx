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
  '/dashboard': lazy(() => import('./pages/Dashboard')),
  '/about': lazy(() => import('./pages/About')),
  '/export': lazy(() => import('./pages/ExportData')),
  '/imports/csv': lazy(() => import('./pages/ImportCSV')),
  '/categories': lazy(() => import('./pages/CategoryManagement')),
  '/merchandising': lazy(() => import('./pages/Merchandising')),
  '/blog': lazy(() => import('./pages/BlogPosts')),
  '/blog/new': lazy(() => import('./pages/BlogPostEdit')),
  '/blog/:id': lazy(() => import('./pages/BlogPostEdit')),
}
