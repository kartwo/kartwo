// 店铺政策 / Store Policy Settings
// 功能：经营资料与七洲时效编辑，按独立权限生成页面并确认正文覆盖
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:01:04

import { useEffect, useState } from 'react'
import { Alert, App, Button, Card, Checkbox, Form, Input, InputNumber, Modal, Select, Space, Typography } from 'antd'
import { policyAPI, type PolicyProfile } from '../policy-api'
import { continentNames, type Continent } from '../shipping-api'
import { useSession } from '../SessionProvider'
import { can } from '../access'
import { useResource, useTask } from '../useResource'
import { PageState } from '../PageState'
import { APIError, errorMessage } from '../transport'
export default function PolicySettings() {
  const { state } = useSession(), resource = useResource(policyAPI.load), task = useTask(), { message } = App.useApp()
  const [form] = Form.useForm<PolicyProfile>(), [publish, setPublish] = useState(true)
  const [overwrite, setOverwrite] = useState<PolicyProfile | null>(null), [generationError, setGenerationError] = useState('')
  const data = resource.data, writable = can(state.session, 'settings.policy'), canGenerate = writable && can(state.session, 'content.manage')
  useEffect(() => { if (data) form.setFieldsValue(data.profile) }, [data, form])
  async function save(values: PolicyProfile) {
    if (!writable) return
    setGenerationError('')
    await task.run(() => policyAPI.save(values), () => {
      resource.setData({ configured: true, profile: values }); void message.success('店铺政策资料已保存')
    })
  }
  async function generate(values: PolicyProfile, replace: boolean) {
    if (!canGenerate) return
    setGenerationError('')
    await task.run(async () => {
      await policyAPI.save(values)
      try { return { result: await policyAPI.generate(publish, replace), error: '' } }
      catch (error) {
        if (error instanceof APIError && error.status === 401) throw error
        return { result: null, error: errorMessage(error) }
      }
    }, output => {
      resource.setData({ configured: true, profile: values }); setOverwrite(null)
      if (output.error) setGenerationError(`资料已保存，页面生成失败：${output.error}`)
      else if (output.result) void message.success(`完成：新建 ${output.result.created}，更新 ${output.result.updated}，跳过 ${output.result.skipped}`)
    })
  }
  async function beginGeneration(replace: boolean) {
    if (!canGenerate) return
    let values: PolicyProfile
    try { values = await form.validateFields() } catch { return }
    if (replace) setOverwrite(values)
    else await generate(values, false)
  }
  return <div className="settings-page"><PageState {...resource} retry={resource.refresh}>
    {data && <>
      {!writable && <Alert type="info" title="当前账号只允许查看店铺政策资料。" />}
      {task.error && <Alert type="error" showIcon title={task.error} />}
      {generationError && <Alert type="error" showIcon title={generationError} />}
      <Card title="基础资料">
        <Typography.Paragraph type="secondary">集中维护经营资料，并生成店面页脚所需的英文内容页。生成文案是可编辑的经营草稿，不构成法律意见；正式发布前请按销售市场复核。</Typography.Paragraph>
        <Form name="policy" form={form} layout="vertical" disabled={!writable || task.busy || !!overwrite} onFinish={save}>
          <div className="settings-columns">
            <Form.Item name="support_email" label="客服邮箱" rules={[{ required: true, message: '请输入客服邮箱' }, { transform: (value: string) => value.trim(), type: 'email', message: '请输入合法邮箱' }]}><Input autoComplete="email" /></Form.Item>
            <Form.Item name="ship_from" label="发货国家/地区" rules={[{ required: true, whitespace: true, message: '请输入发货地' }]}><Input placeholder="China" /></Form.Item>
            <Form.Item name="processing_hours" label="订单处理时间（小时内）" rules={[{ required: true, type: 'integer', min: 1, max: 720, message: '请输入 1–720 的整数' }]}><InputNumber min={1} max={720} /></Form.Item>
            <Form.Item name="return_window_days" label="收货后退货期限（天）" rules={[{ required: true, type: 'integer', min: 1, max: 365, message: '请输入 1–365 的整数' }]}><InputNumber min={1} max={365} /></Form.Item>
            <Form.Item name="return_shipping_payer" label="退货运费承担方"><Select options={[{ value: 'buyer', label: '买家' }, { value: 'merchant', label: '商家' }]} /></Form.Item>
            <Form.Item name="business_name" label="经营主体名称" rules={[{ required: true, whitespace: true, message: '请输入经营主体名称' }]}><Input autoComplete="organization" /></Form.Item>
          </div>
          <Typography.Title level={3}>预计配送时效</Typography.Title>
          <Typography.Paragraph type="secondary">填写发货后的工作日区间。清关、节假日、偏远地区和承运商延误不计入承诺。</Typography.Paragraph>
          <Form.List name="delivery_estimates">{fields => fields.map(field => {
            const region = data.profile.delivery_estimates[field.name]?.region ?? ''
            const title = `${continentNames[region as Continent] || region} / ${region}`
            return <div className="policy-estimate" key={field.key}><strong>{title}</strong>
              <Form.Item name={[field.name, 'region']} hidden><Input /></Form.Item>
              <Form.Item name={[field.name, 'min_business_days']} label={`${title} 最少天数`} rules={[{ required: true, type: 'integer', min: 1, max: 120, message: '请输入 1–120 的整数' }]}><InputNumber min={1} max={120} /></Form.Item>
              <Form.Item name={[field.name, 'max_business_days']} label={`${title} 最多天数`} dependencies={[[ 'delivery_estimates', field.name, 'min_business_days' ]]} rules={[{ required: true, type: 'integer', min: 1, max: 120, message: '请输入 1–120 的整数' }, ({ getFieldValue }) => ({ validator: (_, value: number) => value >= getFieldValue(['delivery_estimates', field.name, 'min_business_days']) ? Promise.resolve() : Promise.reject(new Error('最多天数不能小于最少天数')) })]}><InputNumber min={1} max={120} /></Form.Item>
            </div>
          })}</Form.List>
          {writable && <Button type="primary" htmlType="submit" loading={task.busy}>保存资料</Button>}
        </Form>
      </Card>
      <Card title="生成页脚内容">
        <Typography.Paragraph>将生成 About Us、Contact Us、Shipping Policy、Returns &amp; Refunds、Privacy Policy、Terms of Service 和 FAQ 共 7 页。</Typography.Paragraph>
        <Checkbox checked={publish} disabled={!canGenerate || task.busy || !!overwrite} onChange={event => setPublish(event.target.checked)}>新建页面后直接发布到店面页脚</Checkbox>
        <Typography.Paragraph className="settings-result" type="secondary">生成缺少页面不会改动现有同链接页面；重新生成会覆盖标准页面正文，并保留现有草稿/发布状态。生成前会先保存当前资料。</Typography.Paragraph>
        {!canGenerate && writable && <Alert type="info" title="当前账号可编辑政策资料；生成页面还需内容管理权限。" />}
        {canGenerate && <Space wrap><Button type="primary" disabled={task.busy || !!overwrite} onClick={() => void beginGeneration(false)}>生成缺少页面</Button><Button danger disabled={task.busy || !!overwrite} onClick={() => void beginGeneration(true)}>重新生成并覆盖正文</Button></Space>}
      </Card>
      <Modal open={!!overwrite} title="覆盖标准页面正文" okText="重新生成" cancelText="取消" confirmLoading={task.busy} okButtonProps={{ danger: true }} onCancel={() => { if (!task.busy) setOverwrite(null) }} onOk={() => { if (overwrite) void generate(overwrite, true) }}>将按当前资料重新生成 7 个标准页面的正文。原有草稿/发布状态会保留，手工修改的正文会被覆盖。确定继续？</Modal>
    </>}
  </PageState></div>
}
