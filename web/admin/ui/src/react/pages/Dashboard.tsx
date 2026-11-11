// 经营概览 / Store Dashboard
// 功能：真实订单净额、商品库存和开店配置引导，入口遵循当前账号权限
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:27:44

import { CalendarOutlined, ClockCircleOutlined, ReloadOutlined, ShopOutlined, ShoppingCartOutlined, WarningOutlined } from '@ant-design/icons'
import type { ReactNode } from 'react'
import { Alert, Button, Card, Statistic, Tag, Typography } from 'antd'
import { Link } from 'react-router-dom'
import { dashboardAPI, dashboardMoney } from '../dashboard-api'
import { useSession } from '../SessionProvider'
import { canVisit, matchRoute } from '../access'
import { useResource } from '../useResource'
import { PageState } from '../PageState'
export default function Dashboard() {
  const { state } = useSession(), resource = useResource(dashboardAPI.load), data = resource.data
  const mayVisit = (path: string) => canVisit(state.session, matchRoute(path))
  const linked = (path: string, children: ReactNode, className = '') => mayVisit(path) ? <Link to={path} className={`dashboard-card-link ${className}`}>{children}</Link> : <div className={className}>{children}</div>
  return <div className="read-page"><PageState {...resource} retry={resource.refresh}>
    {data && <>
      <Card title="经营概览" extra={<Tag color={data.setup.ready ? 'success' : 'default'}>{data.setup.ready ? '基础设置已就绪' : '继续完成开店设置'}</Tag>}>
        <Typography.Paragraph>订单、商品与开店进度，在这里一眼掌握。</Typography.Paragraph>
        {data.setup.ready ? <Alert type="success" showIcon title="商品、收款和域名已配置" description="请继续确认真实交易及 HTTPS 访问，完成开店验收。" /> : <div className="dashboard-guides">
          {[
            { done: data.setup.has_products, path: '/products/new', title: '发布首个商品', description: '先上架一个能卖的商品。' },
            { done: data.setup.payment_configured, path: '/payment', title: '配置收款', description: '接入 Stripe 或 PayPal。' },
            { done: data.setup.domain_configured, path: '/domain', title: '配置域名与 HTTPS', description: '配置店面域名后启用自动 HTTPS。' },
          ].filter(step => !step.done).map(step => <Card key={step.path} size="small" title={step.title}><Typography.Paragraph type="secondary">{step.description}</Typography.Paragraph>{mayVisit(step.path) ? <Link to={step.path}>前往{step.title}</Link> : <Typography.Text type="secondary">请联系有权限的同事完成。</Typography.Text>}</Card>)}
        </div>}
      </Card>
      <Typography.Paragraph type="secondary">销售额按当前市场货币显示，为订单创建时间范围内已确认正常收款扣除退款后的金额。</Typography.Paragraph>
      <div className="dashboard-stats">
        <Card><Statistic title="今日订单" prefix={<ShoppingCartOutlined aria-hidden className="dashboard-stat-icon" />} value={data.orders.today.count} /><Typography.Text type="secondary">销售额 {dashboardMoney(data.orders.today.sales_cents, data.currency)}</Typography.Text></Card>
        <Card><Statistic title="近 7 日订单" prefix={<CalendarOutlined aria-hidden className="dashboard-stat-icon" />} value={data.orders.week.count} /><Typography.Text type="secondary">销售额 {dashboardMoney(data.orders.week.sales_cents, data.currency)}</Typography.Text></Card>
        {linked('/orders', <Card><Statistic title="待处理订单" prefix={<ClockCircleOutlined aria-hidden className="dashboard-stat-icon" />} value={data.orders.pending_fulfillment} /><Typography.Text type="secondary">已付待发货</Typography.Text></Card>)}
        {linked('/products', <Card><Statistic title="商品总数" prefix={<ShopOutlined aria-hidden className="dashboard-stat-icon" />} value={data.products.count} /><Typography.Text type="secondary">全部未删除商品（含草稿）</Typography.Text></Card>)}
        {linked('/products', <Card><Statistic title="库存告警（零 / 低）" prefix={<WarningOutlined aria-hidden className="dashboard-stat-icon" />} value={data.products.zero_stock} suffix={` / ${data.products.low_stock}`} /><Typography.Text type="secondary">按可售库存统计，低库存为 1–5</Typography.Text></Card>, 'dashboard-stock-card')}
      </div>
      {!data.orders.week.count && <Alert type="info" title="近 7 日暂无订单。可以分享店面链接，邀请顾客访问。" />}
    </>}
  </PageState><div><Button icon={<ReloadOutlined aria-hidden />} disabled={resource.loading} onClick={() => void resource.refresh()}>刷新概览</Button></div></div>
}
