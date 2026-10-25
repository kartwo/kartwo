// 订单履约与访问管理 / Order Fulfillment and Access
// 功能：按独立资金/履约和员工资料权限开放发货，店主确认撤销顾客凭证
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:46:12

import { CarOutlined, LockOutlined } from '@ant-design/icons'
import { useState } from 'react'
import { Alert, Button, Card, Form, Input, Modal } from 'antd'
import { useOrderOperations } from './OrderOperations'
import type { ShipmentInput } from './order-api'
export function OrderFulfillment() {
  const actions = useOrderOperations(), [revoking, setRevoking] = useState(false)
  return <>
    {actions.fulfillAllowed && <Card title="发货与追踪"><Form<ShipmentInput> name="shipment" layout="vertical" disabled={actions.busy || actions.stale || !!actions.intent} initialValues={{ carrier: '', tracking_number: '', tracking_url: '' }} onFinish={values => { if (!actions.busy && !actions.stale && !actions.intent) actions.fulfill(values) }}>
      <Form.Item name="carrier" label="承运商" rules={[{ required: true, whitespace: true, message: '请填写承运商' }]}><Input placeholder="例如 DHL" /></Form.Item>
      <Form.Item name="tracking_number" label="运单号" rules={[{ required: true, whitespace: true, message: '请填写运单号' }]}><Input maxLength={128} /></Form.Item>
      <Form.Item name="tracking_url" label="追踪链接（可选）"><Input placeholder="https://" /></Form.Item>
      <Button icon={<CarOutlined aria-hidden />} type="primary" htmlType="submit" disabled={actions.busy || actions.stale || !!actions.intent}>确认发货</Button>
    </Form></Card>}
    {actions.owner && <Card title="顾客订单访问"><Button icon={<LockOutlined aria-hidden />} danger disabled={actions.busy} onClick={() => setRevoking(true)}>撤销顾客访问</Button><Modal open={revoking} title="撤销顾客访问" okText="撤销访问" cancelText="取消" okButtonProps={{ danger: true, disabled: actions.busy }} onCancel={() => setRevoking(false)} onOk={() => { if (!actions.busy) { setRevoking(false); actions.revoke() } }}><Alert type="warning" title="此订单所有设备上的查看凭证和未使用验证码会立即失效。" description="顾客可通过订单邮箱重新验证；订单与付款状态不变。" /></Modal></Card>}
  </>
}
