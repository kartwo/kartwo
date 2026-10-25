// 订单详情与持久退款 / Order Details and Durable Refunds
// 功能：组合隐私、收款、履约及核对页面；共享操作锁与刷新屏障，未知退款保留原键
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:46:12

import { useCallback, useRef, useState } from 'react'
import { Alert, App, Button, Space } from 'antd'
import { useLocation, useNavigate } from 'react-router-dom'
import { orderAPI, refundStates, shipmentPayload, type Order, type RefundInput, type RefundResult } from '../order-api'
import { canShip, refundDraft, receiptKey } from '../order-model'
import { centsToMoney } from '../money'
import { writeAndReload } from '../catalog-api'
import { can } from '../access'
import { useSession } from '../SessionProvider'
import { useResource, useTask } from '../useResource'
import { PageState } from '../PageState'
import { OrderOperations, type OrderOperationsValue } from '../OrderOperations'
import { OrderRecords } from '../OrderRecords'
import { NewRefund, RefundRecovery, RefundRequests } from '../OrderRefunds'
import { OrderFulfillment } from '../OrderFulfillment'
function Detail({ id }: { id: string }) {
  const { state } = useSession(), customer = can(state.session, 'orders.customer_data'), owner = state.session?.role === 'owner', navigate = useNavigate(), { message } = App.useApp()
  const resource = useResource(useCallback((signal: AbortSignal) => orderAPI.get(id, customer, signal), [id, customer])), task = useTask(), lock = useRef(false)
  const [blockedSnapshot, setBlockedSnapshot] = useState<Order | null>(null), [intent, setIntent] = useState<RefundInput | null>(null), [ack, setAck] = useState<RefundResult | null>(null), [warning, setWarning] = useState('')
  const [ackSnapshot, setAckSnapshot] = useState<Order | null>(null)
  // A new validated GET object removes the barrier. A failed GET or old snapshot cannot do so.
  const stale = blockedSnapshot !== null && (!resource.data || resource.data === blockedSnapshot), busy = task.busy || resource.loading
  async function refresh() {
    if (lock.current) return
    lock.current = true
    try { task.clearError(); setWarning(''); await resource.refresh() } finally { lock.current = false }
  }
  async function financial<T>(write: () => Promise<T>, confirmed: (result: T) => void, validate?: () => void) {
    if (lock.current) return
    lock.current = true
    try {
      await task.run(async () => {
        setWarning('')
        validate?.()
        if (resource.data) setBlockedSnapshot(resource.data)
        return writeAndReload(write, () => orderAPI.get(id, customer))
      }, result => { confirmed(result.value); if (result.data) resource.setData(result.data); setWarning(result.warning) })
    } finally { lock.current = false }
  }
  const showRefund = (result: RefundResult) => { setAck(result); setAckSnapshot(resource.data ?? blockedSnapshot); void message.info(`退款请求：${refundStates[result.state]}。请核对处理记录。`) }
  const operations: OrderOperationsValue = {
    busy, stale, owner, customer, intent, ack: ackSnapshot === blockedSnapshot ? ack : null,
    fulfillAllowed: !!resource.data && can(state.session, 'orders.fulfill') && customer && canShip(resource.data),
    submit: draft => {
      if (!owner || stale || intent || !resource.data) return
      const order = resource.data
      void financial(async () => {
        const receipt = order.receipts.find(receipt => receipt.provider === draft.provider && receipt.reference === draft.payment_ref)
        const current = receipt && refundDraft(order, receiptKey(receipt), centsToMoney(draft.amount_cents))
        if (!current || current.expected_refunded_cents !== draft.expected_refunded_cents || current.currency !== draft.currency) throw new Error('付款余额已变化，请刷新核对后重新确认')
        const input: RefundInput = { ...draft, request_key: crypto.randomUUID() }; setIntent(input)
        return orderAPI.refund(id, input)
      }, result => { setIntent(null); showRefund(result) })
    },
    resume: () => { if (owner && intent) void financial(() => orderAPI.refund(id, intent), result => { setIntent(null); showRefund(result) }) },
    check: receipt => { if (owner) void financial(() => orderAPI.reconcile(id, receipt), result => { void message.info(result.pending ? '已核对金额，平台仍有处理中退款，请稍后继续核对。' : '已按支付平台记录校正退款金额。') }) },
    refreshRefund: (request, gateway) => { if (owner) void financial(() => orderAPI.refreshRefund(id, request, gateway), showRefund) },
    fulfill: values => {
      if (!resource.data || !operations.fulfillAllowed || stale || intent) return
      void financial(() => orderAPI.fulfill(id, values), () => { void message.success('订单已标记为发货') }, () => { shipmentPayload(values) })
    },
    revoke: () => {
      if (!owner || lock.current) return
      lock.current = true
      void task.run(() => orderAPI.revoke(id), () => { void message.success('已撤销此订单的顾客访问凭证。') }).finally(() => { lock.current = false })
    },
  }
  return <OrderOperations.Provider value={operations}><div className="read-page">
    <Space wrap><Button disabled={busy} onClick={() => void navigate('/orders')}>返回订单列表</Button><Button disabled={busy} onClick={() => void refresh()}>刷新订单</Button></Space>
    {task.busy && <div role="status">正在处理订单操作，请稍候…</div>}
    {task.error && <Alert type="error" showIcon title={task.error} />}
    {stale && <Alert type="warning" showIcon title="操作后的订单状态尚未重新确认，请刷新核对。暂停发起新退款和发货，原退款请求仍可继续核对。" />}
    {warning && <Alert type="warning" title={warning} />}
    <RefundRecovery />
    <PageState {...resource} retry={refresh}>{resource.data && <><OrderRecords order={resource.data} /><OrderFulfillment /><NewRefund order={resource.data} /><RefundRequests order={resource.data} /></>}</PageState>
  </div></OrderOperations.Provider>
}
export default function OrderDetail() { const { pathname } = useLocation(), id = pathname.slice('/orders/'.length); return <Detail key={id} id={id} /> }
