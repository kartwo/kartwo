// 订单退款面板 / Order Refund Panels
// 功能：按付款确认金额、保留未知请求并继续原键、展示持久退款与平台核对入口
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:46:12

import { useState } from 'react'
import { Alert, Button, Card, Descriptions, Form, Input, Modal, Select, Typography } from 'antd'
import { refundStates, type Order, type RefundDraft, type RefundRequest } from './order-api'
import { activeRefund, money, receiptKey, refundableReceipts, refundDraft, refundReason, reviewText } from './order-model'
import { centsToMoney } from './money'
import { useOrderOperations } from './OrderOperations'
export function RefundRecovery() {
  const actions = useOrderOperations(), intent = actions.intent, ack = actions.ack
  if (!actions.owner) return null
  if (intent) return <Card title="原退款请求尚未确认"><Alert type="warning" showIcon title="当前付款、金额和请求键已锁定。请继续原请求或刷新订单核对，离开页面前先核对结果。" />
    <Typography.Paragraph className="order-reference">{intent.provider} · {intent.payment_ref} · {money(intent.amount_cents, intent.currency)}</Typography.Paragraph>
    <Button danger disabled={actions.busy} onClick={actions.resume}>继续原退款请求</Button>
  </Card>
  return ack && actions.stale ? <Card title="已收到退款请求结果"><Typography.Paragraph>请求编号：{ack.public_id} · {money(ack.amount_cents, ack.currency)} · {refundStates[ack.state]}</Typography.Paragraph>{activeRefund(ack.state) && <Button disabled={actions.busy} onClick={() => actions.refreshRefund(ack, '')}>核对此退款请求</Button>}</Card> : null
}
export function NewRefund({ order }: { order: Order }) {
  const actions = useOrderOperations(), [form] = Form.useForm<{ payment: string; amount: string }>()
  const [confirming, setConfirming] = useState<RefundDraft | null>(null), [error, setError] = useState('')
  const receipts = refundableReceipts(order), disabled = actions.busy || actions.stale || !!actions.intent
  if (!actions.owner || !receipts.length) return null
  function prepare(values: { payment: string; amount: string }) {
    if (disabled) return
    try { setConfirming(refundDraft(order, values.payment, values.amount)); setError('') }
    catch (failure) { setError(failure instanceof Error ? failure.message : '退款金额无效') }
  }
  return <Card title="按付款退款">{error && <Alert type="error" showIcon title={error} />}
    <Form form={form} layout="vertical" disabled={disabled} onFinish={prepare}>
      <Form.Item name="payment" label="选择付款" rules={[{ required: true, message: '请选择要退款的付款' }]}><Select options={receipts.map(receipt => ({ value: receiptKey(receipt), label: `${receipt.provider} · ${receipt.reference} · 可退 ${money(receipt.amount_cents - receipt.refunded_cents, receipt.currency)} · ${reviewText(receipt.review_reason)}` }))} onChange={key => { const receipt = receipts.find(item => receiptKey(item) === key); form.setFieldValue('amount', receipt ? centsToMoney(receipt.amount_cents - receipt.refunded_cents) : '') }} /></Form.Item>
      <Form.Item name="amount" label="退款金额" rules={[{ required: true, message: '请输入退款金额' }]}><Input inputMode="decimal" maxLength={24} placeholder="例如 10.00" /></Form.Item>
      <Typography.Paragraph type="secondary">退款不会自动补回库存；已发货订单的运单会保留。请逐笔核对付款与币种。</Typography.Paragraph>
      <Button danger htmlType="submit" disabled={disabled}>提交退款</Button>
    </Form>
    <Modal open={!!confirming} title="确认退款" okText="提交退款" cancelText="取消" okButtonProps={{ danger: true, disabled }} onCancel={() => setConfirming(null)} onOk={() => { if (confirming && !disabled) { const draft = confirming; setConfirming(null); actions.submit(draft) } }}>
      {confirming && <Descriptions column={1} items={[{ key: 'provider', label: '支付通道', children: confirming.provider }, { key: 'payment', label: '付款', children: <span className="order-reference">{confirming.payment_ref}</span> }, { key: 'amount', label: '退款金额', children: money(confirming.amount_cents, confirming.currency) }]} />}
      <Typography.Paragraph>确认向这笔付款退回上述金额？请核对付款与金额。</Typography.Paragraph>
    </Modal>
  </Card>
}
function RequestCard({ request }: { request: RefundRequest }) {
  const actions = useOrderOperations(), [gateway, setGateway] = useState('')
  return <div className="order-record"><strong>{money(request.amount_cents, request.currency)} · {request.provider} · {refundStates[request.state]}</strong>
    <Typography.Paragraph className="order-reference" type="secondary">{request.payment_ref} · {request.created_at}</Typography.Paragraph>
    {actions.owner && <Typography.Paragraph className="order-reference" type="secondary">请求编号：{request.public_id}{request.gateway_ref ? ` · 支付平台退款编号：${request.gateway_ref}` : ''}</Typography.Paragraph>}
    {request.reason && <Typography.Paragraph>{refundReason(request.reason)}</Typography.Paragraph>}
    {actions.owner && ['prepared', 'pending', 'review'].includes(request.state) && <div className="read-page">
      {request.state === 'review' && !request.gateway_ref && <><label htmlFor={`gateway-${request.public_id}`}>支付平台退款编号</label><Input id={`gateway-${request.public_id}`} aria-label={`支付平台退款编号：${request.public_id}`} value={gateway} maxLength={255} disabled={actions.busy} onChange={event => setGateway(event.target.value)} placeholder="从这笔退款的平台记录中复制" /></>}
      <Button disabled={actions.busy} aria-label={`继续处理退款：${request.public_id}`} onClick={() => actions.refreshRefund(request, gateway.trim())}>继续处理 / 核对结果</Button>
    </div>}
  </div>
}
export function RefundRequests({ order }: { order: Order }) {
  const actions = useOrderOperations()
  return <>{order.refund_requests.length > 0 && <Card title="退款请求"><Typography.Paragraph type="secondary">已取得平台退款编号的处理中请求由后台自动查询。继续处理会沿用原请求；结果未知时不会自动新建或重新提交退款。</Typography.Paragraph>{order.refund_requests.map(request => <RequestCard key={request.public_id} request={request} />)}</Card>}{!actions.owner && <Typography.Paragraph type="secondary">退款、平台核对及撤销顾客访问仅限店主操作。</Typography.Paragraph>}</>
}
