// 发货通知界面 / Shipment Notice Panels
// 功能：按权限展示运单历史和真实投递状态，确认更正/核对重试，恢复原请求并有界刷新
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 16:21:58

import { CheckCircleOutlined, EditOutlined, ReloadOutlined, SendOutlined, SyncOutlined } from '@ant-design/icons'
import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { Alert, Button, Card, Checkbox, Collapse, Descriptions, Form, Input, Modal, Space, Tag, Typography } from 'antd'
import { Link } from 'react-router-dom'
import type { Order, ShipmentInput } from './order-api'
import { noticeOrigins, noticeReasons, noticeResolutions, noticeStates, shipmentAPI, type NoticeAction, type ShipmentHistory, type ShipmentNotice, type TrackingUpdate } from './shipment-api'
import { useOrderOperations } from './OrderOperations'
import { useResource } from './useResource'
import { useSession } from './SessionProvider'
import { PageState } from './PageState'

const statusTone = (status: ShipmentNotice['status']) => ({ pending: 'processing', sending: 'processing', sent: 'success', failed: 'error', skipped: 'warning', delivery_unknown: 'warning', superseded: 'default' })[status]
export function ShipmentRecovery() {
  const actions = useOrderOperations(), pending = actions.shipmentIntent
  if (!actions.shipmentAllowed || !pending) return null
  const label = pending.kind === 'fulfill' ? '发货操作' : pending.kind === 'update' ? '运单更正' : '通知操作'
  return <Card title={`原${label}尚未确认`}><Alert type="warning" showIcon title="原操作已保留，请继续原请求核对。“刷新订单”不会自动重发或清除原请求。" description="核对完成前暂停新退款、发货、运单更正和通知操作。离开页面前请先核对结果。" />
    {pending.kind !== 'notice' && <Typography.Paragraph className="order-reference">{pending.fields.carrier} · {pending.fields.tracking_number}</Typography.Paragraph>}
    <Button icon={<SyncOutlined aria-hidden />} disabled={actions.busy} onClick={actions.resumeShipment}>{`继续原${label}`}</Button>
  </Card>
}

function NoticeRecord({ notice, children }: { notice: ShipmentNotice; children?: ReactNode }) {
  return <div className="order-record">
    <Space wrap><Typography.Text strong>第 {notice.version} 版 · 第 {notice.sequence} 次投递</Typography.Text><Tag color={statusTone(notice.status)}>{noticeStates[notice.status]}</Tag>{notice.resolution && <Tag>{noticeResolutions[notice.resolution]}</Tag>}</Space>
    <Typography.Paragraph type="secondary">{noticeOrigins[notice.origin]} · 已尝试 {notice.attempts} 次 · 提交时间（UTC）：{notice.created_at}</Typography.Paragraph>
    {notice.reason && <Typography.Paragraph>{noticeReasons[notice.reason]}</Typography.Paragraph>}
    {notice.sent_at && <Typography.Paragraph>服务器接收确认（UTC）：{notice.sent_at}</Typography.Paragraph>}
    {notice.resolved_at && <Typography.Paragraph type="secondary">核对或替代时间（UTC）：{notice.resolved_at}</Typography.Paragraph>}
    {notice.status === 'pending' && notice.attempts > 0 && <Typography.Paragraph type="secondary">下次自动尝试（UTC）：{notice.next_attempt_at}</Typography.Paragraph>}
    {children}
  </div>
}

function ShipmentContent({ order }: { order: Order }) {
  const actions = useOrderOperations(), [form] = Form.useForm<TrackingUpdate>()
  // Bind the response to this exact parent snapshot. Background polls retain
  // visible records; a parent refresh never makes older records actionable.
  const resource = useResource(useCallback(async (signal: AbortSignal) => ({ order, history: await shipmentAPI.get(order.public_id, actions.customer, signal) }), [order, actions.customer]), true)
  const history = resource.data?.order === order ? resource.data.history : null
  const [editing, setEditing] = useState(false), [dialog, setDialog] = useState<{ notice: ShipmentNotice; action: NoticeAction } | null>(null), [checked, setChecked] = useState(false)
  const [editDraft, setEditDraft] = useState<TrackingUpdate | null>(null)
  const [attempted, setAttempted] = useState(false)
  const latest = history?.notices[0], revision = history?.revisions[0]
  const mismatch = !!history && (!history.has_shipment || !!revision && (revision.carrier !== order.tracking_carrier || actions.customer && (revision.tracking_number !== order.tracking_number || revision.tracking_url !== order.tracking_url)))
  const disabled = actions.busy || actions.stale || !!actions.intent || resource.loading || !history || mismatch
  const sending = !!history?.notices.some(notice => notice.status === 'sending')
  useEffect(() => { if (actions.shipmentIntent && !actions.busy) { setEditing(false); setDialog(null) } }, [actions.shipmentIntent, actions.busy])
  // The modal mounts its Form only when opened. Set the pinned draft after
  // that commit, so the form is connected and a poll cannot replace its fields.
  useEffect(() => { if (editing && editDraft) form.setFieldsValue(editDraft) }, [editing, editDraft, form])
  useEffect(() => {
    if (!history?.notices.some(notice => notice.status === 'pending' || notice.status === 'sending') || actions.busy || actions.shipmentIntent || editing || dialog) return
    const timer = window.setInterval(() => { if (document.visibilityState === 'visible') void resource.refresh() }, 20000)
    return () => window.clearInterval(timer)
  }, [history, resource.refresh, actions.busy, actions.shipmentIntent, editing, dialog])
  function edit() {
    if (disabled || !actions.shipmentAllowed || sending || !history) return
    setEditDraft({ expected_revision: history.current_revision, carrier: order.tracking_carrier, tracking_number: order.tracking_number, tracking_url: order.tracking_url })
    setAttempted(false); setEditing(true)
  }
  async function save(values: ShipmentInput) {
    if (disabled || !actions.shipmentAllowed) return
    const expected_revision = form.getFieldValue('expected_revision')
    setAttempted(true)
    const ok = await actions.updateShipment({ ...values, expected_revision })
    if (ok) { setEditing(false); void resource.refresh() }
  }
  function open(notice: ShipmentNotice, action: NoticeAction) { if (disabled) return; setAttempted(false); setChecked(false); setDialog({ notice, action }) }
  async function manage() {
    if (!dialog || disabled || !actions.shipmentAllowed || dialog.action !== 'retry' && !checked) return
    setAttempted(true)
    const ok = await actions.manageNotice({ notice_id: dialog.notice.public_id, action: dialog.action, request_key: crypto.randomUUID(), confirm_unknown: dialog.action !== 'retry' && checked })
    if (ok) { setDialog(null); void resource.refresh() }
  }
  const canAct = actions.shipmentAllowed && latest?.revision_id === history?.current_revision && latest?.resolution === ''
  return <Card title="发货通知" extra={<Button icon={<ReloadOutlined aria-hidden />} loading={resource.loading} disabled={actions.busy || editing || !!dialog} onClick={() => void resource.refresh()}>刷新通知</Button>}>
    <Typography.Paragraph type="secondary">发货记录与邮件投递分别保留。邮件服务器已接收不等于顾客已收到；投递结果未知时先核对，再决定是否重发。</Typography.Paragraph>
    <PageState loading={resource.loading && !history} error={resource.error} retry={resource.refresh}>
      {history && <div className="read-page">
        {mismatch && <Alert type="warning" showIcon title="运单已变化，请先使用页面上方的“刷新订单”核对，再继续操作。" />}
        {resource.loading && <Typography.Text type="secondary" role="status">正在刷新通知状态…</Typography.Text>}
        {history.has_shipment && <Typography.Paragraph>原发货时间（UTC）：{history.shipped_at} · {history.current_version ? `当前运单第 ${history.current_version} 版` : '历史运单'}</Typography.Paragraph>}
        {history.has_shipment && history.current_version === 0 && <Alert type="info" showIcon title="历史发货记录没有通知历史，升级不会自动补发。更正运单后会生成新的通知。" />}
        {actions.shipmentAllowed && <div><Button icon={<EditOutlined aria-hidden />} disabled={disabled || sending} onClick={edit}>更正运单</Button>{sending && <Typography.Paragraph type="secondary">通知正在投递，请稍后刷新，再更正运单。</Typography.Paragraph>}</div>}
        {latest && <NoticeRecord notice={latest}>{canAct && <Space wrap>
          {['failed', 'skipped'].includes(latest.status) && <Button icon={<SendOutlined aria-hidden />} disabled={disabled} onClick={() => open(latest, 'retry')}>重试通知</Button>}
          {latest.status === 'delivery_unknown' && <><Button icon={<CheckCircleOutlined aria-hidden />} disabled={disabled} onClick={() => open(latest, 'received')}>确认已收到</Button><Button danger icon={<SendOutlined aria-hidden />} disabled={disabled} onClick={() => open(latest, 'resend_after_check')}>核对后重发</Button></>}
        </Space>}</NoticeRecord>}
        {latest?.reason === 'smtp_unconfigured' && <Typography.Paragraph>{actions.owner ? <Link to="/smtp">前往邮件设置</Link> : '请联系店主配置邮件服务，再重试通知。'}</Typography.Paragraph>}
        <HistoryRecords history={history} />
      </div>}
    </PageState>
    <Modal destroyOnHidden open={editing} title="更正运单" okText="保存更正并通知" cancelText="取消" confirmLoading={actions.busy} okButtonProps={{ disabled }} cancelButtonProps={{ disabled: actions.busy }} onCancel={() => { if (!actions.busy) setEditing(false) }} onOk={() => form.submit()}>
      <Alert type="info" showIcon title="保存后会向顾客发送新的运单信息；原发货时间、付款、退款和库存不变。" />
      {attempted && actions.shipmentError && <Alert type="error" showIcon title={actions.shipmentError} />}
      {(resource.error || mismatch) && <Alert type="warning" showIcon title="通知或运单记录需要重新核对，请取消后刷新订单再操作。" />}
      <Form form={form} layout="vertical" disabled={actions.busy || !!actions.shipmentIntent} onFinish={save}>
        <Form.Item name="expected_revision" hidden><Input /></Form.Item>
        <Form.Item name="carrier" label="更正承运商" rules={[{ required: true, whitespace: true, message: '请填写承运商' }]}><Input maxLength={128} /></Form.Item>
        <Form.Item name="tracking_number" label="更正运单号" rules={[{ required: true, whitespace: true, message: '请填写运单号' }]}><Input maxLength={128} /></Form.Item>
        <Form.Item name="tracking_url" label="更正追踪链接（可选）"><Input maxLength={2048} placeholder="https://" /></Form.Item>
      </Form>
    </Modal>
    <Modal destroyOnHidden open={!!dialog} title={dialog?.action === 'retry' ? '重试发货通知' : dialog?.action === 'received' ? '确认收到发货通知' : '核对后重发通知'} okText={dialog?.action === 'received' ? '确认已收到' : dialog?.action === 'retry' ? '确认重试' : '确认重发'} cancelText="取消" confirmLoading={actions.busy} okButtonProps={{ disabled: disabled || dialog?.action !== 'retry' && !checked, danger: dialog?.action === 'resend_after_check' }} cancelButtonProps={{ disabled: actions.busy }} onCancel={() => { if (!actions.busy) setDialog(null) }} onOk={() => void manage()}>
      {attempted && actions.shipmentError && <Alert type="error" showIcon title={actions.shipmentError} />}
      {(resource.error || mismatch) && <Alert type="warning" showIcon title="通知或运单记录需要重新核对，请取消后刷新订单再操作。" />}
      <Typography.Paragraph>将处理第 {dialog?.notice.version} 版运单的这次通知。重试沿用原收件人，旧投递记录会保留。</Typography.Paragraph>
      {dialog?.action === 'received' ? <Typography.Paragraph>仅记录你已核对收到邮件的结果，不会发送邮件。</Typography.Paragraph> : dialog?.action === 'retry' ? <Typography.Paragraph>确认后加入发送队列，新通知至少间隔一分钟才能重试。</Typography.Paragraph> : <Alert type="warning" showIcon title="系统无法确认上次是否发出。请先核对收件箱或邮件服务商记录；重发可能产生重复邮件。" />}
      {dialog && dialog.action !== 'retry' && <Checkbox checked={checked} disabled={actions.busy} onChange={event => setChecked(event.target.checked)}>{dialog.action === 'received' ? '我已核对，确认顾客已收到此邮件' : '我已核对，确认未收到，接受重复邮件风险后重发'}</Checkbox>}
    </Modal>
  </Card>
}

function HistoryRecords({ history }: { history: ShipmentHistory }) {
  const items = []
  if (history.revisions.length) items.push({ key: 'revisions', label: `运单历史（共 ${history.revision_count} 版）`, children: <>
    {history.revision_count > history.revisions.length && <Typography.Paragraph type="secondary">显示最近 {history.revisions.length} 个版本，较早记录仍保留。</Typography.Paragraph>}
    {history.revisions.map(revision => <div className="order-record" key={revision.public_id}><Space wrap><Typography.Text strong>第 {revision.version} 版</Typography.Text>{revision.legacy && <Tag>原始历史快照</Tag>}</Space><Descriptions column={1} size="small" items={[{ key: 'carrier', label: '承运商', children: revision.carrier }, { key: 'number', label: '运单号', children: revision.tracking_number }, { key: 'url', label: '追踪链接', children: revision.tracking_url ? <a href={revision.tracking_url} target="_blank" rel="noopener noreferrer" referrerPolicy="no-referrer">查看该版物流追踪</a> : '无有效 HTTPS 追踪链接' }, { key: 'time', label: '记录时间（UTC）', children: revision.created_at }]} /></div>)}
  </> })
  if (history.notices.length > 1) items.push({ key: 'notices', label: `更多投递记录（共 ${history.notice_count} 次）`, children: <>
    {history.notice_count > history.notices.length && <Typography.Paragraph type="secondary">本页显示最近 {history.notices.length} 条投递记录，较早记录仍保留。</Typography.Paragraph>}
    {history.notices.slice(1).map(notice => <NoticeRecord key={notice.public_id} notice={notice} />)}
  </> })
  return items.length ? <Collapse items={items} /> : null
}

export function OrderShipment({ order }: { order: Order }) {
  const { state } = useSession()
  if (order.fulfillment_status !== 'fulfilled') return null
  if (state.session?.role === 'demo') return <Alert type="info" title="公开演示不读取真实发货通知。" />
  return <ShipmentContent order={order} />
}
