// 订单共享操作 / Shared Order Operations
// 功能：订单各区域共用操作锁、刷新屏障与店主退款权限，不丢失未确认的原请求
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:46:12

import { createContext, useContext } from 'react'
import type { RefundDraft, RefundInput, RefundResult, RefundRequest, Receipt, ShipmentInput } from './order-api'
import type { ShipmentIntent, TrackingUpdate, NoticeActionInput } from './shipment-api'
export interface OrderOperationsValue {
  busy: boolean; stale: boolean; owner: boolean; customer: boolean; fulfillAllowed: boolean; intent: RefundInput | null
  submit: (draft: RefundDraft) => void; resume: () => void; check: (receipt: Receipt) => void
  refreshRefund: (request: Pick<RefundRequest, 'public_id' | 'provider' | 'amount_cents' | 'currency'>, gateway: string) => void
  fulfill: (input: ShipmentInput) => void; revoke: () => void; ack: RefundResult | null
  shipmentAllowed: boolean; shipmentIntent: ShipmentIntent | null; shipmentError: string
  updateShipment: (input: TrackingUpdate) => Promise<boolean>; manageNotice: (input: NoticeActionInput) => Promise<boolean>; resumeShipment: () => void
}
export const OrderOperations = createContext<OrderOperationsValue | null>(null)
export function useOrderOperations() { const value = useContext(OrderOperations); if (!value) throw new Error('订单操作上下文未提供'); return value }
