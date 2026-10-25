// 订单列表 / Order List
// 功能：保留订单明细入口、按权限隐藏客户邮箱，店主原生下载带状态和 UTC 日期筛选的 CSV
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:46:12

import { useCallback, useState } from 'react'
import { Alert, Button, Card, Input, Select, Table, Tag, Typography } from 'antd'
import { Link } from 'react-router-dom'
import { orderAPI, orderStatuses, orderExportURL, type ExportFilter, type OrderSummary } from '../order-api'
import { money } from '../order-model'
import { can } from '../access'
import { useSession } from '../SessionProvider'
import { useResource } from '../useResource'
import { PageState } from '../PageState'
export default function OrderList() {
  const { state } = useSession(), customer = can(state.session, 'orders.customer_data'), owner = state.session?.role === 'owner'
  const resource = useResource(useCallback((signal: AbortSignal) => orderAPI.list(customer, signal), [customer]))
  const [filter, setFilter] = useState<ExportFilter>({ status: '', from: '', to: '' })
  let href = '', error = ''
  if (owner) { try { href = orderExportURL(filter) } catch (failure) { error = failure instanceof Error ? failure.message : '导出条件无效' } }
  return <div className="read-page">
    {owner && <Card title="导出订单 CSV"><div className="order-export-controls"><label htmlFor="order-export-status">导出订单状态</label><Select id="order-export-status" aria-label="导出订单状态" value={filter.status} onChange={status => setFilter(current => ({ ...current, status }))} options={[{ value: '', label: '全部状态' }, ...Object.entries(orderStatuses).map(([value, label]) => ({ value, label }))]} />
      <label htmlFor="order-export-from">从</label><Input id="order-export-from" aria-label="导出开始日期" type="date" value={filter.from} onChange={event => setFilter(current => ({ ...current, from: event.target.value }))} />
      <label htmlFor="order-export-to">至</label><Input id="order-export-to" aria-label="导出结束日期" type="date" value={filter.to} onChange={event => setFilter(current => ({ ...current, to: event.target.value }))} />
      <Button type="primary" disabled={!!error} href={href || undefined} target={href ? '_blank' : undefined} rel="noopener noreferrer">导出订单 CSV</Button></div>
      {error && <Alert type="error" title={error} />}<Typography.Paragraph type="secondary">日期按 UTC 整日筛选，包含结束当天。CSV 在新标签页下载，包含客户履约资料；请在浏览器下载列表确认结果，失败时新标签页会显示服务器提示。</Typography.Paragraph>
    </Card>}
    {!customer && <Alert type="info" title="客户邮箱与收货资料已隐藏；查看订单权限不包含客户资料权限。" />}
    <Button className="order-refresh-button" disabled={resource.loading} onClick={() => void resource.refresh()}>刷新订单列表</Button>
    <PageState {...resource} retry={resource.refresh}>{resource.data && <Card><Table<OrderSummary> rowKey="public_id" scroll={{ x: true }} pagination={{ pageSize: 20, showSizeChanger: false }} dataSource={resource.data} locale={{ emptyText: '暂无订单' }} columns={[
      { title: '订单号', key: 'public_id', render: (_, order) => <Link to={`/orders/${order.public_id}`}>{order.public_id}</Link> },
      { title: '状态', key: 'status', render: (_, order) => <Tag>{orderStatuses[order.status]}</Tag> }, { title: '邮箱', dataIndex: 'email' },
      { title: '金额', key: 'total', render: (_, order) => money(order.total_cents, order.currency) }, { title: '时间', dataIndex: 'created_at' },
    ]} /></Card>}</PageState>
  </div>
}
