// 路由权限验证 / Route Access Tests
// 功能：与旧路由核对完整性，测试员工细分权限和演示边界
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:13:54

import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { adminRoutes, can, canVisit, homePath, matchRoute, navigation, selectedPath } from './access'
import { account } from './test-fixtures'
import { parseSession, parseStatus, parseWizard } from './contracts'

describe('路由迁移清单', () => {
  it('保留旧后台的每个业务路由和三个非业务入口', () => {
    const source = readFileSync('src/main.js', 'utf8')
    const original = [...source.matchAll(/path: '([^']+)'/g)].map(match => match[1])
    expect(new Set([...adminRoutes.map(route => route.path), '/', '/market', '/accept-invite'])).toEqual(new Set(original))
  })
  it('新建与编辑分别匹配，不吞掉多层未知路径', () => {
    expect(matchRoute('/products/new')?.permission).toBe('products.create')
    expect(matchRoute('/products/item-123')?.permission).toBe('products.view')
    expect(matchRoute('/blog/new')?.permission).toBe('blog.manage')
    expect(matchRoute('/blog/article')?.permission).toBe('blog.view')
    for (const path of ['/products/', '/products/a/b', '/not-found', '/blogging/a']) expect(matchRoute(path)).toBeUndefined()
    expect(selectedPath('/blog/article')).toBe('/blog')
    expect(selectedPath('/blog-settings')).toBe('/blog-settings')
  })
})
describe('权限', () => {
  it('无会话及未知页面拒绝访问，包括店主未知页面', () => {
    for (const route of adminRoutes) expect(canVisit(null, route)).toBe(false)
    expect(canVisit(account(), undefined)).toBe(false)
    expect(() => parseSession({ ...account(), role: '' })).toThrow()
    expect(() => parseSession({ ...account(), permissions: ['products.view', 1] })).toThrow()
    expect(() => parseStatus({ initialized: 'false' })).toThrow()
    expect(() => parseWizard({ needed: null })).toThrow()
  })
  it('员工独立的创建、库存和价格权限不会被编辑权限统吞', () => {
    const staff = account('staff', ['products.create', 'products.inventory', 'products.price'])
    expect(canVisit(staff, matchRoute('/products/new'))).toBe(true)
    expect(can(staff, 'products.inventory')).toBe(true)
    expect(can(staff, 'products.price')).toBe(true)
    expect(can(staff, 'products.edit')).toBe(false)
    expect(canVisit(staff, matchRoute('/products/item'))).toBe(false)
    expect(homePath(account('staff', ['blog.view']))).toBe('/blog')
  })
  it('错误注入的店主专属权限不能授权员工；demo 只能浏览', () => {
    const reserved = ['users.manage', 'roles.manage', 'sessions.manage', 'orders.refund', 'orders.export', 'translation.manage']
    const staff = account('staff', reserved)
    for (const permission of reserved) expect(can(staff, permission)).toBe(false)
    for (const path of ['/payment', '/smtp', '/backup', '/export', '/translation', '/users']) expect(canVisit(staff, matchRoute(path))).toBe(false)
    expect(navigation(account('demo')).some(route => route.path === '/users')).toBe(false)
    expect(canVisit(account('demo'), matchRoute('/payment'))).toBe(true)
    expect(can(account('demo'), 'orders.fulfill')).toBe(false)
    expect(canVisit(account(), matchRoute('/users'))).toBe(true)
  })
})
