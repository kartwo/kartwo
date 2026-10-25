// 订单列表 / Order List
// 功能：服务端分页、私密搜索与独立待处理视图，保留细分权限及店主独立 CSV 导出
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:46:12

import { ClearOutlined, DownloadOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import { useCallback, useState } from 'react'
import { Alert, Button, Card, Form, Input, Select, Space, Table, Tag, Typography } from 'antd'
import { Link } from 'react-router-dom'
import { orderStatuses, orderExportURL, type ExportFilter } from '../order-api'
import { emptyOrderFilters, initialOrderSearch, orderSearchPayload, orderViews, searchOrders, type OrderFilters, type OrderView, type WorkbenchOrder } from '../order-workbench-api'
import { money } from '../order-model'
import { can } from '../access'
import { useSession } from '../SessionProvider'
import { useResource } from '../useResource'
import { PageState } from '../PageState'
export default function OrderList() {
  const { state } = useSession(), customer = can(state.session, 'orders.customer_data'), owner = state.session?.role === 'owner'
  const [draft, setDraft] = useState<OrderFilters>(emptyOrderFilters)
  const [query, setQuery] = useState(initialOrderSearch)
  const [filterError, setFilterError] = useState('')
  const resource = useResource(useCallback(async (signal: AbortSignal) => ({ query, result: await searchOrders(query, customer, signal) }), [query, customer]))
  const data = resource.data?.query === query ? resource.data.result : null
  const loading = resource.loading || !resource.error && !data
  function apply() {
    try { const next = orderSearchPayload({ ...draft, view: query.view, page: 1, page_size: query.page_size }); setFilterError(''); setQuery(next) }
    catch (error) { setFilterError(error instanceof Error ? error.message : '查询条件无效') }
  }
  const modified = Object.keys(draft).some(key => draft[key as keyof OrderFilters] !== query[key as keyof OrderFilters])
  return <div className="read-page">
    <Card title="订单工作台">
      <Form layout="vertical" onFinish={apply}>
        <div className="order-search-fields">
          <Form.Item label="搜索订单" htmlFor="order-search-term"><Input id="order-search-term" aria-label="搜索订单" maxLength={160} allowClear placeholder={customer ? '订单号或客户邮箱' : '订单号'} value={draft.term} onChange={event => setDraft(current => ({ ...current, term: event.target.value }))} /></Form.Item>
          <Form.Item label="订单状态" htmlFor="order-search-status"><Select id="order-search-status" aria-label="订单状态筛选" value={draft.status} onChange={status => setDraft(current => ({ ...current, status }))} options={[{ value: '', label: '全部状态' }, ...Object.entries(orderStatuses).map(([value, label]) => ({ value, label }))]} /></Form.Item>
          <Form.Item label="支付方式" htmlFor="order-search-provider"><Select id="order-search-provider" aria-label="支付方式筛选" value={draft.provider} onChange={provider => setDraft(current => ({ ...current, provider }))} options={[{ value: '', label: '全部方式' }, { value: 'stripe', label: 'Stripe' }, { value: 'paypal', label: 'PayPal' }]} /></Form.Item>
          <Form.Item label="开始日期（UTC）" htmlFor="order-search-from"><Input id="order-search-from" aria-label="订单开始日期" type="date" value={draft.from} onChange={event => setDraft(current => ({ ...current, from: event.target.value }))} /></Form.Item>
          <Form.Item label="结束日期（含当天）" htmlFor="order-search-to"><Input id="order-search-to" aria-label="订单结束日期" type="date" value={draft.to} onChange={event => setDraft(current => ({ ...current, to: event.target.value }))} /></Form.Item>
        </div>
        <Space wrap><Button icon={<SearchOutlined aria-hidden />} type="primary" htmlType="submit">查询订单</Button><Button icon={<ClearOutlined aria-hidden />} onClick={() => { setDraft(emptyOrderFilters()); setQuery(initialOrderSearch()); setFilterError('') }}>清空筛选</Button><Button icon={<ReloadOutlined aria-hidden />} disabled={loading} onClick={() => void resource.refresh()}>刷新订单列表</Button></Space>
      </Form>
      {filterError && <Alert type="error" showIcon title={filterError} />}
      {modified && <Typography.Paragraph type="secondary">筛选条件已修改，点击“查询订单”后应用；下方当前仍是上次查询结果。</Typography.Paragraph>}
      <Typography.Paragraph type="secondary">日期按 UTC 整日筛选，包含结束当天。搜索仅在当前页面内存中保留；导出条件单独设置。</Typography.Paragraph>
      <div className="order-work-views" aria-label="订单待处理视图">{(Object.entries(orderViews) as [OrderView, string][]).map(([view, label]) => <Button key={view} aria-label={`查看${label}`} aria-pressed={query.view === view} type={query.view === view ? 'primary' : 'default'} onClick={() => setQuery(current => ({ ...current, view, page: 1 }))}>{label}<span className="order-view-count">{data?.counts[view] ?? '…'}</span></Button>)}</div>
      <Typography.Paragraph type="secondary">数量随已应用的搜索、状态、支付方式与日期变化；资金、退款和发货视图可重叠。可发货仅是清单提示，执行前仍须在详情核对。</Typography.Paragraph>
    </Card>
    {!customer && <Alert type="info" title="客户邮箱与收货资料已隐藏；查看订单权限不包含客户资料权限。" />}
    <PageState loading={loading} error={resource.error} retry={resource.refresh}>{data && <Card title={`${orderViews[data.view]} · ${data.total} 条`}><Table<WorkbenchOrder> className="order-workbench-table" rowKey="public_id" scroll={{ x: 'max-content' }} pagination={{ current: data.page, pageSize: data.page_size, total: data.total, showSizeChanger: true, pageSizeOptions: [20, 50, 100], showTotal: total => `共 ${total} 条`, onChange: (page, size) => setQuery(current => ({ ...current, page: size === current.page_size ? page : 1, page_size: size })) }} dataSource={data.orders} locale={{ emptyText: '没有符合条件的订单' }} columns={[
      { title: '订单号', key: 'public_id', render: (_, order) => <Link to={`/orders/${order.public_id}`}>{order.public_id}</Link> },
      { title: '订单状态', key: 'status', render: (_, order) => <Tag>{orderStatuses[order.status]}</Tag> },
      { title: '待处理', key: 'work', render: (_, order) => <Space wrap>{order.needs_review && <Tag color="warning">资金待核对</Tag>}{order.refund_pending && <Tag color="processing">退款待处理</Tag>}{order.ready_to_fulfill && <Tag color="success">可发货</Tag>}{!order.needs_review && !order.refund_pending && !order.ready_to_fulfill && '—'}</Space> },
      { title: '履约', key: 'shipment', render: (_, order) => order.shipped ? '已发货' : '未发货' }, { title: '邮箱', dataIndex: 'email' },
      { title: '金额', key: 'total', render: (_, order) => money(order.total_cents, order.currency) },
      { title: '支付方式', key: 'provider', render: (_, order) => ({ stripe: 'Stripe', paypal: 'PayPal' })[order.payment_provider] ?? (order.payment_provider || '未选择') }, { title: '下单时间（UTC）', dataIndex: 'created_at' },
    ]} /></Card>}</PageState>
    {owner && <OrderExport />}
  </div>
}
function OrderExport() {
  const [filter, setFilter] = useState<ExportFilter>({ status: '', from: '', to: '' })
  let href = '', error = ''
  try { href = orderExportURL(filter) } catch (failure) { error = failure instanceof Error ? failure.message : '导出条件无效' }
  return <Card title="导出订单 CSV（独立条件）"><div className="order-export-controls"><label htmlFor="order-export-status">导出订单状态</label><Select id="order-export-status" aria-label="导出订单状态" value={filter.status} onChange={status => setFilter(current => ({ ...current, status }))} options={[{ value: '', label: '全部状态' }, ...Object.entries(orderStatuses).map(([value, label]) => ({ value, label }))]} />
      <label htmlFor="order-export-from">从</label><Input id="order-export-from" aria-label="导出开始日期" type="date" value={filter.from} onChange={event => setFilter(current => ({ ...current, from: event.target.value }))} />
      <label htmlFor="order-export-to">至</label><Input id="order-export-to" aria-label="导出结束日期" type="date" value={filter.to} onChange={event => setFilter(current => ({ ...current, to: event.target.value }))} />
      <Button icon={<DownloadOutlined aria-hidden />} type="primary" disabled={!!error} href={href || undefined} target={href ? '_blank' : undefined} rel="noopener noreferrer">导出订单 CSV</Button></div>
      {error && <Alert type="error" title={error} />}<Typography.Paragraph type="secondary">仅使用这里的订单状态及 UTC 起止日期，包含结束当天；上方工作台搜索与待处理视图不会改变导出范围。CSV 在新标签页下载，包含客户履约资料；请在浏览器下载列表确认结果，失败时新标签页会显示服务器提示。</Typography.Paragraph>
    </Card>
}
