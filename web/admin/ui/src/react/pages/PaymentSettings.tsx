// 收款设置 / Payment Settings
// 功能：迁移 Stripe 与 PayPal 分通道配置，保留凭证状态、只读和测试连接
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:23:15

import { ApiOutlined, SaveOutlined } from '@ant-design/icons'
import { useEffect, useState } from 'react'
import { Alert, App, Button, Card, Form, Input, Select, Space, Tag, Typography } from 'antd'
import { settingsAPI, type StripeFields, type PayPalFields } from '../settings-api'
import { useResource, useTask } from '../useResource'
import { PageState, SourceNotice } from '../PageState'
import { useSession } from '../SessionProvider'
import { useWizardActivity } from '../WizardActivity'

export default function PaymentSettings() {
  const { state } = useSession()
  const resource = useResource(settingsAPI.payment)
  const task = useTask()
  const { message } = App.useApp()
  const [stripeForm] = Form.useForm<StripeFields>()
  const [paypalForm] = Form.useForm<PayPalFields>()
  const [operation, setOperation] = useState('')
  const data = resource.data
  useWizardActivity(resource.loading || task.busy)
  useEffect(() => {
    if (!data) return
    stripeForm.setFieldsValue({ mode: data.stripe.mode, publishable: data.stripe.publishable, secret: '', webhook_secret: '' })
    paypalForm.setFieldsValue({ mode: data.paypal.mode, client_id: data.paypal.client_id, webhook_id: data.paypal.webhook_id, secret: '' })
  }, [data, stripeForm, paypalForm])
  const owner = state.session?.role === 'owner'
  const stripeWritable = owner && data?.stripe.source === 'db' && !data.stripe.readonly
  const paypalWritable = owner && data?.paypal.source === 'db' && !data.paypal.readonly
  const secretPlaceholder = (readonly: boolean, present: boolean) => readonly ? '凭证不会回传到浏览器' : present ? '已保存，留空不改' : '粘贴密钥'
  async function saveStripe(values: StripeFields) {
    if (!stripeWritable) return
    await task.run(async () => { setOperation('stripe'); return settingsAPI.savePayment({ stripe: values }) }, next => {
      resource.setData(next)
      void message.success('Stripe 已保存，收款配置已即时生效。')
    })
  }
  async function savePayPal(values: PayPalFields) {
    if (!paypalWritable) return
    await task.run(async () => { setOperation('paypal'); return settingsAPI.savePayment({ paypal: values }) }, next => {
      resource.setData(next)
      void message.success('PayPal 已保存，收款配置已即时生效。')
    })
  }
  async function testStripe() {
    if (!owner || !data?.stripe.has_secret || data.stripe.source === 'demo') return
    await task.run(async () => { setOperation('test'); return settingsAPI.testStripe() }, () => {
      void message.success('Stripe 连接成功，已保存的密钥与模式一致。')
    })
  }
  return <div className="settings-page">
    <Typography.Paragraph>密钥加密保存，不会回传到浏览器。默认使用测试环境；先完成测试交易，再切换正式收款。</Typography.Paragraph>
    <PageState {...resource} retry={resource.refresh}>
      {data && <>
        {task.error && <Alert type="error" showIcon title={task.error} />}
        <Card title={<Space>Stripe（信用卡）<Tag color={data.stripe.has_secret ? 'success' : 'default'}>{data.stripe.has_secret ? '已配置' : '未配置'}</Tag></Space>}>
          <SourceNotice source={data.stripe.source} readonly={!stripeWritable} env="STRIPE_*" />
          <Form name="stripe" form={stripeForm} layout="vertical" disabled={!stripeWritable || task.busy} onFinish={saveStripe}>
            <Form.Item name="mode" label="Stripe 模式"><Select options={[{ value: 'test', label: '测试 / 沙箱' }, { value: 'live', label: '正式' }]} /></Form.Item>
            <Form.Item name="publishable" label="Publishable key"><Input autoComplete="off" placeholder="pk_…" /></Form.Item>
            <Form.Item name="secret" label="Secret key（推荐使用受限密钥 rk_）" extra="仅授予所需权限；留空保留已保存的密钥。"><Input.Password autoComplete="new-password" placeholder={secretPlaceholder(!stripeWritable, data.stripe.has_secret)} /></Form.Item>
            <Form.Item name="webhook_secret" label={<Space>Webhook 签名密钥<Tag color={data.stripe.has_webhook ? 'success' : 'default'}>{data.stripe.has_webhook ? '已配置' : '未配置'}</Tag></Space>}><Input.Password autoComplete="new-password" placeholder={secretPlaceholder(!stripeWritable, data.stripe.has_webhook)} /></Form.Item>
            <Typography.Paragraph type="secondary">回调地址为店面域名下的 <Typography.Text code>/webhooks/stripe</Typography.Text>。请订阅 checkout.session.completed、checkout.session.async_payment_succeeded、charge.refunded、refund.created、refund.updated 和 refund.failed，同步付款与退款结果。</Typography.Paragraph>
            {stripeWritable && <Button icon={<SaveOutlined aria-hidden />} type="primary" htmlType="submit" loading={task.busy && operation === 'stripe'}>保存 Stripe</Button>}
          </Form>
          <div className="settings-test">
            <Button icon={<ApiOutlined aria-hidden />} loading={task.busy && operation === 'test'} disabled={!owner || !data.stripe.has_secret || data.stripe.source === 'demo' || task.busy} onClick={() => void testStripe()}>测试 Stripe 连接</Button>
            <Typography.Paragraph type="secondary">使用已保存的配置测试，不保存输入框中尚未提交的内容。</Typography.Paragraph>
          </div>
        </Card>
        <Card title={<Space>PayPal<Tag color={data.paypal.has_secret ? 'success' : 'default'}>{data.paypal.has_secret ? '已配置' : '未配置'}</Tag></Space>}>
          <SourceNotice source={data.paypal.source} readonly={!paypalWritable} env="PAYPAL_*" />
          <Typography.Paragraph type="secondary">使用同一个沙箱或正式应用的 Client ID 与 Secret。</Typography.Paragraph>
          <Form name="paypal" form={paypalForm} layout="vertical" disabled={!paypalWritable || task.busy} onFinish={savePayPal}>
            <Form.Item name="mode" label="PayPal 模式"><Select options={[{ value: 'sandbox', label: '沙箱' }, { value: 'live', label: '正式' }]} /></Form.Item>
            <Form.Item name="client_id" label="Client ID"><Input autoComplete="off" /></Form.Item>
            <Form.Item name="secret" label="PayPal Secret"><Input.Password autoComplete="new-password" placeholder={secretPlaceholder(!paypalWritable, data.paypal.has_secret)} /></Form.Item>
            <Form.Item name="webhook_id" label="Webhook ID（收款及退款对账必填）"><Input autoComplete="off" /></Form.Item>
            <Typography.Paragraph type="secondary">回调地址为店面域名下的 <Typography.Text code>/webhooks/paypal</Typography.Text>。请订阅 PAYMENT.CAPTURE.COMPLETED、PAYMENT.CAPTURE.REFUNDED、PAYMENT.REFUND.PENDING 和 PAYMENT.REFUND.FAILED。</Typography.Paragraph>
            {paypalWritable && <Button icon={<SaveOutlined aria-hidden />} type="primary" htmlType="submit" loading={task.busy && operation === 'paypal'}>保存 PayPal</Button>}
          </Form>
        </Card>
      </>}
    </PageState>
  </div>
}
