// 订单售后面板 / Order After Sale Panels
// 功能：Ant审核、退回检查与成功退款关联，固定说明/版本，恢复原操作并保留只读权限
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 22:41:59

import { AuditOutlined, CheckCircleOutlined, CloseCircleOutlined, InboxOutlined, ReloadOutlined, RollbackOutlined, SyncOutlined } from '@ant-design/icons'
import { useEffect, useState } from 'react'
import { Alert, Button, Card, Checkbox, Collapse, Descriptions, Empty, Form, Input, Modal, Select, Space, Tag, Typography } from 'antd'
import type { Order } from './order-api'
import { afterSaleActions, afterSaleStates, allowedAfterSaleActions, activeAfterSale, matchingRefunds, type AfterSaleAction, type AfterSaleCase, type AfterSaleHistory, type AfterSaleIntent } from './after-sale-api'
import { money, refundableReceipts } from './order-model'
import { useOrderOperations } from './OrderOperations'
import { useSession } from './SessionProvider'

const kindLabel = (c: AfterSaleCase) => c.kind === 'refund' ? '仅退款' : '退货退款'
const tone = (c: AfterSaleCase) => c.refund_needs_review ? 'error' : c.state === 'completed' ? 'success' : ['withdrawn', 'rejected'].includes(c.state) ? 'default' : 'processing'
const buttonLabel = (c: AfterSaleCase, action: AfterSaleAction) => action === 'approve' ? c.kind === 'return_refund' && c.state === 'requested' ? '同意退货并给出指引' : '批准退款申请' : action === 'reject' ? '拒绝申请' : action === 'receive_return' ? '确认实际收到退货' : '关联成功退款并办结'
const actionIcon = (action: AfterSaleAction) => action === 'reject' ? <CloseCircleOutlined aria-hidden /> : action === 'receive_return' ? <InboxOutlined aria-hidden /> : action === 'complete' ? <CheckCircleOutlined aria-hidden /> : <AuditOutlined aria-hidden />

export function AfterSaleRecovery({ intent, acknowledgement }: { intent: AfterSaleIntent | null; acknowledgement: AfterSaleCase | null }) {
  const actions = useOrderOperations()
  if (!actions.afterSaleAllowed) return null
  if (intent) return <Card title="原售后操作尚未确认"><Alert type="warning" showIcon title="原操作的申请、版本和说明已保留。请继续原请求核对，页内刷新不会清除或自动重发。" description="核对完成前暂停新的售后审核、退款、发货和运单通知操作。离开页面前请先核对结果。" />
    <Typography.Paragraph>{afterSaleActions[intent.action]} · {money(intent.amount_cents, intent.currency)}</Typography.Paragraph>
    <Typography.Paragraph className="order-reference">申请：{intent.case_id}</Typography.Paragraph><Typography.Paragraph className="order-address-text">{intent.message || '本次没有附加说明'}</Typography.Paragraph>
    <Button icon={<SyncOutlined aria-hidden />} disabled={actions.busy} onClick={actions.resumeAfterSale}>继续原售后操作</Button>
  </Card>
  return acknowledgement ? <Card title="已收到售后操作确认"><Alert type={acknowledgement.refund_needs_review ? "warning" : "info"} showIcon title={acknowledgement.refund_needs_review ? '原办结事实保留，但退款目前待核对，请联系店主核对平台记录。详情尚待重新读取。' : `${afterSaleStates[acknowledgement.state]}。操作已记录，详情尚待重新读取，请勿再建相同操作。`} /><Typography.Paragraph className="order-reference">申请：{acknowledgement.public_id} · {money(acknowledgement.amount_cents, acknowledgement.currency)}</Typography.Paragraph></Card> : null
}

function CaseRecord({ item, order, history }: { item: AfterSaleCase; order: Order; history: AfterSaleHistory }) {
  const actions = useOrderOperations(), disabled = actions.busy || actions.stale || !!actions.intent || !!actions.afterSaleIntent || !!actions.afterSaleEditor?.open
  const matches = matchingRefunds(order, item, history)
  return <Card type="inner" title={<Space wrap><Typography.Text strong>{kindLabel(item)} · {money(item.amount_cents, item.currency)}</Typography.Text><Tag color={tone(item)}>{afterSaleStates[item.state]}</Tag></Space>}>
    <Descriptions bordered size="small" column={1} items={[
      { key: 'id', label: '申请编号', children: <span className="order-reference">{item.public_id}</span> }, { key: 'reason', label: '顾客理由及涉及商品', children: <span className="order-address-text">{item.reason}</span> },
      { key: 'time', label: '提交时间（UTC）', children: item.created_at },
      { key: 'terms', label: '申请时政策资料', children: item.terms.configured ? `签收后 ${item.terms.return_window_days} 天；退回运费由${item.terms.return_shipping_payer === 'buyer' ? '买家' : '商家'}承担${item.terms.support_email ? `；客服 ${item.terms.support_email}` : ''}` : '当时尚未配置政策资料，请按店铺已公布政策和具体情况审核。' },
      ...(item.merchant_message && item.merchant_message !== item.return_instructions ? [{ key: 'message', label: '最近商家说明（顾客可见）', children: <span className="order-address-text">{item.merchant_message}</span> }] : []),
      ...(item.return_instructions ? [{ key: 'instructions', label: '退回指引（顾客可见）', children: <span className="order-address-text">{item.return_instructions}</span> }] : []),
      ...(item.return_tracking_number ? [{ key: 'tracking', label: '顾客退回运单', children: <span className="order-address-text">{item.return_carrier} · {item.return_tracking_number}</span> }] : []),
      ...(item.refund_request_id ? [{ key: 'refund', label: '关联成功退款', children: <span className="order-reference">{item.refund_request_id}</span> }] : []),
    ]} />
    {item.state === 'approved' && <Alert type="info" showIcon title="申请获批，实际退款尚未办结。" description={actions.owner ? '请先在下方“按付款退款”中明确选择付款并退款。出现匹配的成功退款记录后，再回此关联办结；不会自动退款或回补库存。' : '请联系店主明确处理实际退款。获得匹配的成功记录后才可关联办结；审核权限不包含资金操作。'} />}
    {item.state === 'approved' && actions.owner && refundableReceipts(order).length > 0 && <Button icon={<RollbackOutlined aria-hidden />} disabled={disabled} onClick={() => { const target = document.getElementById('order-refund-form'); if (target && typeof target.scrollIntoView === 'function') target.scrollIntoView({ block: 'start' }) }}>前往按付款退款</Button>}
    {item.state === 'awaiting_return' && <Typography.Paragraph type="secondary">退回运单由顾客填写；物流信息不代表实际收到包裹，收到后请明确确认并检查商品。</Typography.Paragraph>}
    {item.refund_needs_review && <Alert type="error" showIcon title="已办结退款出现新的待核对情况。" description="请联系店主核对支付平台记录；原售后办结、退款和发货历史会继续保留。" />}
    {actions.afterSaleAllowed && <Space wrap>{allowedAfterSaleActions(item).map(action => <Button key={action} icon={actionIcon(action)} danger={action === 'reject'} disabled={disabled || action === 'complete' && !matches.length} onClick={() => actions.openAfterSale(item, action)}>{buttonLabel(item, action)}</Button>)}</Space>}
    {item.state === 'approved' && !matches.length && <Typography.Paragraph type="secondary">目前没有可关联的成功退款：需属于本订单、申请之后创建、金额和币种完全匹配，且无未解决的资金核对问题。系统会在办结时再次核对。</Typography.Paragraph>}
    <Collapse items={[{ key: 'events', label: `处理历史（${item.events.length} 条）`, children: item.events.map(event => <div className="order-record" key={event.version}><Typography.Text strong>{afterSaleActions[event.action]} · {event.actor === 'customer' ? '顾客' : '商家'}</Typography.Text><Typography.Paragraph type="secondary">{event.created_at}（UTC） · {afterSaleStates[event.state]}</Typography.Paragraph>{event.message && <Typography.Paragraph className="order-address-text">{event.message}</Typography.Paragraph>}{event.tracking_number && <Typography.Paragraph className="order-address-text">{event.carrier} · {event.tracking_number}</Typography.Paragraph>}</div>) }]} />
  </Card>
}

export function OrderAfterSales({ order, history }: { order: Order; history: AfterSaleHistory | null }) {
  const actions = useOrderOperations(), { state } = useSession()
  if (!actions.customer || state.session?.role === 'demo') return <Card title="售后申请"><Typography.Paragraph type="secondary">{state.session?.role === 'demo' ? '公开演示不读取真实售后申请。' : '查看售后理由、退回指引和运单需要客户信息权限。'}</Typography.Paragraph></Card>
  return <Card title="售后申请" extra={<Button icon={<ReloadOutlined aria-hidden />} disabled={actions.busy || !!actions.afterSaleEditor?.open} onClick={() => void actions.refreshAfterSale()}>刷新售后与订单</Button>}>
    <Typography.Paragraph type="secondary">审核、实际退款和库存分别处理。换货请通过客服沟通；退货期限不能从发货日自动推断。</Typography.Paragraph>
    {!actions.afterSaleAllowed && <Alert type="info" title="当前账号可查看，处理申请还需要“处理售后”权限。" />}
    {actions.stale ? <Alert type="warning" showIcon title="操作后的售后记录尚待重新读取。请以上方已确认结果或原请求为准，刷新订单核对后再处理。" /> : !history ? <Alert type="error" showIcon title="售后记录尚未确认，暂停新的订单操作，请刷新核对。" /> : <div className="read-page">
      {history.cases.some(activeAfterSale) && <Alert type="warning" showIcon title="此订单有处理中售后，首次发货暂停；已有运单仍会保留。" />}
      {!history.cases.length ? <Empty description="暂无售后申请；顾客可从已验证的私密订单页申请。" /> : history.cases.map(item => <CaseRecord key={item.public_id} item={item} order={order} history={history} />)}
    </div>}
  </Card>
}

export function AfterSaleDialog({ order, history }: { order: Order | null; history: AfterSaleHistory | null }) {
  const actions = useOrderOperations(), editor = actions.afterSaleEditor
  const [form] = Form.useForm<Pick<AfterSaleIntent, 'message' | 'refund_request_id'>>()
  const [attempted, setAttempted] = useState(false)
  useEffect(() => {
    if (editor?.open) { form.setFieldsValue({ message: editor.intent.message, refund_request_id: editor.intent.refund_request_id }); setAttempted(false) }
  }, [editor?.open, editor?.intent.request_key, form])
  if (!editor) return null
  const { intent, case: item } = editor, current = history?.cases.find(c => c.public_id === item.public_id), changed = !!current && current.version !== intent.expected_version
  const canRebase = !!current && allowedAfterSaleActions(current).includes(intent.action), matches = order && current && history ? matchingRefunds(order, current, history) : []
  const unknown = !!actions.afterSaleIntent, disabled = actions.busy || actions.stale || unknown || !current || changed || intent.action === 'receive_return' && !editor.receipt_confirmed || intent.action === 'complete' && !matches.length
  const instructions = intent.action === 'approve' && item.kind === 'return_refund' && item.state === 'requested'
  return <Modal destroyOnHidden open={editor.open} title={buttonLabel(item, intent.action)} okText={buttonLabel(item, intent.action)} cancelText="关闭并保留说明" confirmLoading={actions.busy} okButtonProps={{ disabled, danger: intent.action === 'reject' }} cancelButtonProps={{ disabled: actions.busy }} onCancel={actions.closeAfterSale} onOk={() => form.submit()}>
    <Descriptions column={1} size="small" items={[{ key: 'kind', label: '申请类型', children: kindLabel(item) }, { key: 'amount', label: '申请金额', children: money(item.amount_cents, item.currency) }, { key: 'state', label: '打开时状态', children: afterSaleStates[item.state] }, { key: 'reason', label: '顾客理由', children: <span className="order-address-text">{item.reason}</span> }, ...(item.return_tracking_number ? [{ key: 'return', label: '打开时退回运单', children: <span className="order-address-text">{item.return_carrier} · {item.return_tracking_number}</span> }] : []) ]} />
    <Alert type="info" showIcon title={intent.action === 'complete' ? '仅关联已成功退款并办结，不会再次退款或回补库存。' : instructions ? '请输入明确的退回指引，顾客会在私密订单页看到。不要直接沿用发货地作为退回地址。' : '说明会展示给顾客；此次审核不会自动退款、发邮件或回补库存。'} />
    {attempted && actions.afterSaleError && <Alert type="error" showIcon title={actions.afterSaleError} />}
    {(actions.stale || !current) && <Alert type="warning" showIcon title={unknown ? '原操作结果未知，请使用页面上的“继续原售后操作”核对。' : '当前记录尚待重新读取，已填写说明会保留。'} action={!unknown && <Button icon={<ReloadOutlined aria-hidden />} disabled={actions.busy} onClick={() => void actions.refreshAfterSale()}>刷新并保留说明</Button>} />}
    {changed && <Alert type="warning" showIcon title={`申请已变化，当前为“${afterSaleStates[current.state]}”。未自动覆盖打开时的审核内容。`} action={canRebase && <Button icon={<ReloadOutlined aria-hidden />} disabled={actions.busy || actions.stale || unknown} onClick={actions.rebaseAfterSale}>采用最新记录重新核对</Button>} description={!canRebase ? '当前状态已不能执行原操作，请关闭后核对并选择适用的下一步。说明仍会保留。' : undefined} />}
    <Form form={form} layout="vertical" disabled={actions.busy || unknown} onValuesChange={(_, values) => actions.changeAfterSale({ message: values.message ?? '', refund_request_id: values.refund_request_id ?? '' })} onFinish={values => { if (!disabled) { setAttempted(true); void actions.confirmAfterSale({ message: values.message ?? '', refund_request_id: values.refund_request_id ?? '' }) } }}>
      <Form.Item name="message" label={instructions ? '退回指引（顾客可见）' : '处理说明（顾客可见）'} rules={[{ required: intent.action !== 'complete', whitespace: true, message: '请填写给顾客的说明' }, { validator: (_, value: unknown) => typeof value !== 'string' || new TextEncoder().encode(value).length <= 4000 ? Promise.resolve() : Promise.reject(new Error('说明过长，请缩短后再提交')) }]}><Input.TextArea rows={5} maxLength={4000} placeholder={instructions ? '退回收件人、地址、标签使用方式和具体要求；请按本店政策填写' : '请明确说明审核结论或实际收货、检查结果'} /></Form.Item>
      {intent.action === 'complete' && <Form.Item name="refund_request_id" label="匹配的成功退款" rules={[{ required: true, message: '请选择与申请匹配的成功退款记录' }]}><Select options={matches.map(r => ({ value: r.public_id, label: `${money(r.amount_cents, r.currency)} · ${r.provider} · ${r.public_id} · ${r.created_at}（UTC）` }))} /></Form.Item>}
    </Form>
    {intent.action === 'receive_return' && <Checkbox checked={editor.receipt_confirmed} disabled={actions.busy || unknown} onChange={event => actions.confirmReturnReceived(event.target.checked)}>我已核对，确认实际收到退回包裹，后续仍需检查商品</Checkbox>}
  </Modal>
}
