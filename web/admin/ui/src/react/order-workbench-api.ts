// 订单工作台接口 / Order Workbench API
// 功能：从请求体提交有界搜索，严格核对分页/计数/视图与隐私字段，不持久化客户查询
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 13:42:16

import { bool, choice, integer, record } from './api-values'
import { orderExportURL, parseOrders, type OrderStatus, type OrderSummary } from './order-api'
import { request } from './transport'
export const orderViews = { all: '全部订单', awaiting_payment: '待付款', ready_to_fulfill: '可发货', needs_review: '资金待核对', refunds_pending: '退款待处理', shipped: '已发货' } as const
export type OrderView = keyof typeof orderViews
export interface OrderFilters { term: string; status: OrderStatus | ''; provider: '' | 'stripe' | 'paypal'; from: string; to: string }
export interface OrderSearch extends OrderFilters { view: OrderView; page: number; page_size: number }
export interface WorkbenchOrder extends OrderSummary { shipped: boolean; refund_pending: boolean; needs_review: boolean; ready_to_fulfill: boolean }
export interface WorkbenchResult { orders: WorkbenchOrder[]; counts: Record<OrderView, number>; total: number; page: number; page_size: number; view: OrderView }
export function emptyOrderFilters(): OrderFilters { return { term: '', status: '', provider: '', from: '', to: '' } }
export function initialOrderSearch(): OrderSearch { return { ...emptyOrderFilters(), view: 'all', page: 1, page_size: 20 } }
export function orderSearchPayload(input: OrderSearch): OrderSearch {
  orderExportURL({ status: input.status, from: input.from, to: input.to })
  const term = input.term.trim()
  if (Array.from(term).length > 160 || /\p{Cc}/u.test(term)) throw new Error('搜索内容最多 160 字，不能包含控制字符')
  if (!Object.hasOwn(orderViews, input.view) || !['', 'stripe', 'paypal'].includes(input.provider)) throw new Error('订单查询视图或支付方式无效')
  if (!Number.isSafeInteger(input.page) || input.page < 1 || !Number.isSafeInteger(input.page_size) || input.page_size < 1 || input.page_size > 100) throw new Error('页码须为正数，每页最多 100 条')
  return { term, status: input.status, provider: input.provider, from: input.from, to: input.to, view: input.view, page: input.page, page_size: input.page_size }
}
export function parseWorkbench(value: unknown, input: OrderSearch, customer: boolean): WorkbenchResult {
  const data = record(value), orders = parseOrders(data, customer), rawOrders = data.orders as unknown[]
  const countData = record(data.counts), counts = {} as Record<OrderView, number>
  for (const view of Object.keys(orderViews) as OrderView[]) counts[view] = integer(countData, view, 0, Number.MAX_SAFE_INTEGER)
  const total = integer(data, 'total', 0, Number.MAX_SAFE_INTEGER), page = integer(data, 'page', 1, Number.MAX_SAFE_INTEGER), pageSize = integer(data, 'page_size', 1, 100), view = choice(data, 'view', Object.keys(orderViews) as OrderView[])
  const maxPage = total === 0 ? 1 : Math.floor((total - 1) / pageSize) + 1
  if (view !== input.view || total !== counts[view] || Object.values(counts).some(count => count > counts.all) || pageSize !== input.page_size || page !== Math.min(input.page, maxPage) || orders.length !== Math.min(pageSize, total - (page - 1) * pageSize)) throw new Error('订单分页或待处理数量不一致，请重新查询')
  const resultOrders = orders.map((order, index) => {
    const row = record(rawOrders[index]), result = { ...order, shipped: bool(row, 'shipped'), refund_pending: bool(row, 'refund_pending'), needs_review: bool(row, 'needs_review'), ready_to_fulfill: bool(row, 'ready_to_fulfill') }
    if (result.ready_to_fulfill && (result.status !== 'paid' || result.shipped || result.refund_pending || result.needs_review)) throw new Error('订单发货条件不一致，请打开详情核对')
    if (!matchesView(result, view) || input.status && result.status !== input.status || input.provider && result.payment_provider !== input.provider) throw new Error('订单与所选待处理视图或条件不一致，请重新查询')
    return result
  })
  if ((Object.keys(orderViews) as OrderView[]).some(view => resultOrders.filter(order => matchesView(order, view)).length > counts[view])) throw new Error('待处理数量少于当前清单，请重新查询')
  return { total, page, page_size: pageSize, view, counts, orders: resultOrders }
}
function matchesView(order: WorkbenchOrder, view: OrderView): boolean { return view === 'all' || view === 'awaiting_payment' && order.status === 'pending' || view === 'ready_to_fulfill' && order.ready_to_fulfill || view === 'needs_review' && order.needs_review || view === 'refunds_pending' && order.refund_pending || view === 'shipped' && order.shipped }
export async function searchOrders(input: OrderSearch, customer: boolean, signal?: AbortSignal): Promise<WorkbenchResult> {
  const payload = orderSearchPayload(input)
  return parseWorkbench(await request('POST', '/orders/search', { ...payload }, signal), payload, customer)
}
