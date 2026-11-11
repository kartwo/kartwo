// 工作台测试样本 / Workbench Test Fixtures
// 功能：提供分页/视图的隔离接口响应，默认只有虚构订单，无真实客户资料
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 13:42:16

import { orderA, orderSummary } from './order-fixtures'
import { initialOrderSearch, type OrderSearch, type WorkbenchOrder, type WorkbenchResult } from './order-workbench-api'
export function workbenchOrder(overrides: Partial<WorkbenchOrder> = {}): WorkbenchOrder { return { ...orderSummary(orderA), shipped: false, refund_pending: false, needs_review: false, ready_to_fulfill: true, ...overrides } }
export function workbenchFixture(input: OrderSearch = initialOrderSearch(), orders = [workbenchOrder()], counts = { all: 1, awaiting_payment: 0, ready_to_fulfill: 1, needs_review: 0, refunds_pending: 0, shipped: 0 }): WorkbenchResult {
  return { orders, counts, total: counts[input.view], page: input.page, page_size: input.page_size, view: input.view }
}
