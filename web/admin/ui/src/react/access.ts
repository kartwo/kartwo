// 路由与权限 / Routes and Access
// 功能：集中保留旧地址、菜单层级、演示可见性及员工权限边界
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:10:05

import type { Session } from './contracts'
export type Section = 'business' | 'catalog' | 'data' | 'system' | 'settings'
export interface AdminRoute {
  path: string
  title: string
  source: string
  section?: Section
  permission?: string
  ownerOnly?: boolean
  staffHidden?: boolean
  demoReadonly?: boolean
  hidden?: boolean
}
// Source records make the migration checklist auditable; components are connected as each page is ported.
export const adminRoutes: readonly AdminRoute[] = [
  { path: '/dashboard', title: '概览', source: 'Dashboard', permission: 'dashboard.view' },
  { path: '/orders', title: '订单', source: 'OrderList', section: 'business', permission: 'orders.view', demoReadonly: true },
  { path: '/orders/:id', title: '订单详情', source: 'OrderDetail', section: 'business', permission: 'orders.view', demoReadonly: true, hidden: true },
  { path: '/contact', title: '客服留言', source: 'ContactInbox', section: 'business', permission: 'contact.view', demoReadonly: true },
  { path: '/products', title: '商品', source: 'ProductList', section: 'catalog', permission: 'products.view' },
  { path: '/products/new', title: '新建商品', source: 'ProductEdit', section: 'catalog', permission: 'products.create', hidden: true },
  { path: '/products/:id', title: '编辑商品', source: 'ProductEdit', section: 'catalog', permission: 'products.view', hidden: true },
  { path: '/categories', title: '分类', source: 'CategoryManagement', section: 'catalog', permission: 'products.view' },
  { path: '/merchandising', title: '内容', source: 'Merchandising', section: 'catalog', permission: 'content.manage' },
  { path: '/blog', title: '博客文章', source: 'BlogPosts', permission: 'blog.view', demoReadonly: true },
  { path: '/blog/new', title: '新建文章', source: 'BlogPostEdit', permission: 'blog.manage', demoReadonly: true, hidden: true },
  { path: '/blog/:id', title: '编辑文章', source: 'BlogPostEdit', permission: 'blog.view', demoReadonly: true, hidden: true },
  { path: '/imports/csv', title: '导入', source: 'ImportCSV', section: 'data', permission: 'imports.manage', demoReadonly: true },
  { path: '/export', title: '导出', source: 'ExportData', section: 'data', staffHidden: true, demoReadonly: true },
  { path: '/diagnostics', title: '诊断', source: 'Diagnostics', section: 'system', permission: 'diagnostics.view', demoReadonly: true },
  { path: '/audit', title: '日志', source: 'AuditEvents', section: 'system', permission: 'audit.view', demoReadonly: true },
  { path: '/about', title: '关于', source: 'About', section: 'system' },
  { path: '/shop', title: '店铺', source: 'ShopSettings', section: 'settings', permission: 'settings.shop', demoReadonly: true },
  { path: '/payment', title: '收款', source: 'PaymentSettings', section: 'settings', staffHidden: true, demoReadonly: true },
  { path: '/domain', title: '域名', source: 'DomainSettings', section: 'settings', permission: 'settings.domain', demoReadonly: true },
  { path: '/smtp', title: '邮件', source: 'SmtpSettings', section: 'settings', staffHidden: true, demoReadonly: true },
  { path: '/translation', title: '翻译', source: 'TranslationSettings', section: 'settings', staffHidden: true, demoReadonly: true },
  { path: '/backup', title: '备份', source: 'BackupSettings', section: 'settings', staffHidden: true, demoReadonly: true },
  { path: '/shipping', title: '配送', source: 'ShippingSettings', section: 'settings', permission: 'settings.shipping', demoReadonly: true },
  { path: '/policies', title: '店铺政策', source: 'PolicySettings', section: 'settings', permission: 'settings.policy', demoReadonly: true },
  { path: '/contact-settings', title: '客服留言设置', source: 'ContactSettings', section: 'settings', permission: 'contact.view', demoReadonly: true },
  { path: '/blog-settings', title: '博客设置', source: 'BlogSettings', section: 'settings', permission: 'blog.view', demoReadonly: true },
  { path: '/account', title: '账号与安全', source: 'AccountSettings', section: 'settings', demoReadonly: true },
  { path: '/users', title: '用户与角色', source: 'UsersRoles', section: 'settings', ownerOnly: true },
]
export const sections: readonly { key: Section; title: string }[] = [
  { key: 'business', title: '经营' }, { key: 'catalog', title: '商品与内容' },
  { key: 'data', title: '数据' }, { key: 'system', title: '系统' }, { key: 'settings', title: '设置' },
]
const ownerPermissions = new Set(['users.manage', 'roles.manage', 'sessions.manage', 'orders.refund', 'orders.export', 'translation.manage'])
export function can(session: Session | null, permission: string): boolean {
  if (!session) return false
  if (session.role === 'owner') return true
  return session.role === 'staff' && !ownerPermissions.has(permission) && session.permissions.includes(permission)
}
export function canVisit(session: Session | null, route: AdminRoute | undefined): boolean {
  if (!session || !route) return false
  if (route.ownerOnly) return session.role === 'owner'
  if (session.role === 'owner' || session.role === 'demo') return true
  if (session.role !== 'staff' || route.staffHidden) return false
  return route.permission ? can(session, route.permission) : ['/about', '/account'].includes(route.path)
}
export function matchRoute(path: string): AdminRoute | undefined {
  const exact = adminRoutes.find(route => route.path === path)
  if (exact) return exact
  return adminRoutes.find(route => route.path.endsWith('/:id') &&
    path.startsWith(route.path.slice(0, -3)) && /^[^/]+$/.test(path.slice(route.path.length - 3)))
}
export function navigation(session: Session): AdminRoute[] {
  return adminRoutes.filter(route => !route.hidden && canVisit(session, route))
}
export function homePath(session: Session): string {
  return navigation(session)[0]?.path ?? '/account'
}
export function selectedPath(path: string): string {
  const route = matchRoute(path)
  return route?.hidden ? route.path.split('/').slice(0, 2).join('/') : route?.path ?? ''
}
