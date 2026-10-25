// 订单资金与履约记录 / Order Financial and Fulfillment Records
// 功能：分别展示客户、商品、运费、运单、收款和退款历史，保留核对屏障与只读隐私
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:46:12

import { Alert, Button, Card, Descriptions, Empty, Table, Typography } from 'antd'
import { orderStatuses, refundStates, type Order } from './order-api'
import { money, reviewText, paymentTexts } from './order-model'
import { useOrderOperations } from './OrderOperations'
export function OrderRecords({ order }: { order: Order }) {
  const actions = useOrderOperations()
  return <>
    <Card title={`订单 ${order.public_id}`}><Descriptions bordered size="small" column={1} items={[
      { key: 'status', label: '订单状态', children: orderStatuses[order.status] }, { key: 'payment', label: '资金状态', children: paymentTexts[order.payment_status] },
      { key: 'fulfillment', label: '履约', children: order.fulfillment_status === 'fulfilled' ? '已发货（退款保留运单记录）' : '未发货' },
      { key: 'total', label: '订单金额', children: money(order.total_cents, order.currency) }, { key: 'subtotal', label: '商品小计', children: money(order.subtotal_cents, order.currency) },
      { key: 'shipping', label: '运费', children: `${money(order.shipping_cents, order.currency)} · ${order.shipping_rule_name || '无规则名称快照'}` },
      { key: 'email', label: '邮箱', children: order.email }, { key: 'address', label: '收货信息', children: `${order.ship_name} · ${order.ship_phone} · ${order.ship_address} · ${order.ship_country}` },
      { key: 'provider', label: '支付通道', children: order.payment_provider || '—' }, { key: 'time', label: '下单时间', children: order.created_at },
    ]} />{!actions.customer && <Alert type="info" title="客户资料和支付参考号已隐藏。" />}</Card>
    {(order.payment_review || order.status === 'payment_review' || order.payment_status === 'review') && <Alert type="error" showIcon title="收款需要核对，请勿继续发货。" description="超时到账、重复付款或历史退款需逐笔核对。对重复付款退款时，请核对选中的付款，避免退错正常收款。" />}
    {order.fulfillment_status === 'fulfilled' && <Card title="发货与追踪记录"><Descriptions column={1} items={[
      { key: 'carrier', label: '承运商', children: order.tracking_carrier || '无记录' }, { key: 'number', label: '运单号', children: order.tracking_number || '无记录' },
      { key: 'link', label: '追踪链接', children: order.tracking_url ? <a href={order.tracking_url} target="_blank" rel="noopener noreferrer" referrerPolicy="no-referrer">查看物流追踪</a> : '无有效 HTTPS 追踪链接' },
    ]} /></Card>}
    <Card title="收款与退款"><Typography.Paragraph>累计收款 {money(order.paid_cents, order.currency)} · 已退 {money(order.refunded_cents, order.currency)}</Typography.Paragraph>
      {!order.receipts.length && <Empty description="尚无收款账本记录，不能仅凭订单状态发起退款。" />}
      {order.receipts.map((receipt, index) => <div className="order-record" key={index}><strong>{receipt.provider} · {money(receipt.amount_cents, receipt.currency)} · 已退 {money(receipt.refunded_cents, receipt.currency)}</strong>
        <Typography.Paragraph>{reviewText(receipt.review_reason)}{receipt.source === 'legacy' ? '（历史记录）' : ''}</Typography.Paragraph><Typography.Paragraph className="order-reference" type="secondary">{receipt.reference}</Typography.Paragraph>
        {actions.owner && <Button disabled={actions.busy} aria-label={`核对平台退款记录：${index + 1}`} onClick={() => actions.check(receipt)}>核对平台退款记录</Button>}
        <Typography.Paragraph type="secondary">{receipt.refund_checked_at ? `最近核对：${receipt.refund_checked_at}。` : ''}核对只查询平台记录并校正本地记账，不新增退款。</Typography.Paragraph>
        {receipt.refund_reconciliation_required && <Alert type="warning" title="平台退款尚待确认，或与原入账状态不一致。核对完成前暂停此付款的新退款和订单发货。" />}
      </div>)}
    </Card>
    <Card title="商品"><Table<Order['lines'][number] & { index: number }> rowKey="index" size="small" scroll={{ x: true }} pagination={false} dataSource={order.lines.map((line, index) => ({ ...line, index }))} columns={[
      { title: '商品', dataIndex: 'title' }, { title: '规格', dataIndex: 'spec' }, { title: 'SKU', dataIndex: 'sku' }, { title: '单价', key: 'unit', render: (_, line) => money(line.unit_cents, order.currency) }, { title: '数量', dataIndex: 'quantity' }, { title: '小计', key: 'total', render: (_, line) => money(line.line_cents, order.currency) },
    ]} /></Card>
    {order.refunds.length > 0 && <Card title="历史退款入账记录"><Typography.Paragraph type="secondary">保留原入账历史。平台后来失败的退款不计入上方当前已退金额，以核对后的资金状态为准。</Typography.Paragraph>{order.refunds.map((refund, index) => <div className="order-record" key={index}>{money(refund.amount_cents, order.currency)} · {refund.provider}<Typography.Paragraph className="order-reference" type="secondary">{refund.provider_refund_id} · {refund.observed_state ? refundStates[refund.observed_state] : '待核对当前状态'} · {refund.created_at}</Typography.Paragraph></div>)}</Card>}
  </>
}
