// 邮件设置 / SMTP Settings
// 功能：保留加密保存、环境只读、测试发信和密码留空不变语义
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:23:15

import { SaveOutlined, SendOutlined } from '@ant-design/icons'
import { useEffect, useState } from 'react'
import { Alert, App, Button, Card, Form, Input, Select, Space, Tag, Typography } from 'antd'
import { settingsAPI, type SmtpFields } from '../settings-api'
import { useResource, useTask } from '../useResource'
import { PageState, SourceNotice } from '../PageState'
import { useSession } from '../SessionProvider'
import { useWizardActivity } from '../WizardActivity'

export default function SmtpSettings() {
  const { state } = useSession()
  const resource = useResource(settingsAPI.smtp)
  const task = useTask()
  const { message } = App.useApp()
  const [form] = Form.useForm<SmtpFields>()
  const [testForm] = Form.useForm<{ to: string }>()
  const [operation, setOperation] = useState('')
  const data = resource.data
  const owner = state.session?.role === 'owner'
  const writable = owner && data?.source === 'db' && !data.readonly
  useWizardActivity(resource.loading || task.busy)
  useEffect(() => {
    if (!data) return
    form.setFieldsValue({ host: data.host, port: data.port || '587', username: data.username, password: '', from_address: data.from_address, from_name: data.from_name, encryption: data.encryption })
  }, [data, form])
  async function save(values: SmtpFields) {
    if (!writable) return
    await task.run(async () => { setOperation('save'); return settingsAPI.saveSmtp(values) }, next => {
      resource.setData(next)
      void message.success('SMTP 配置已保存')
    })
  }
  async function sendTest(values: { to: string }) {
    if (!owner || !data?.configured || data.source === 'demo') return
    await task.run(async () => { setOperation('test'); return settingsAPI.testSmtp(values.to) }, () => {
      void message.success('测试邮件已发送，请查收（含垃圾箱）。')
    })
  }
  return <div className="settings-page"><PageState {...resource} retry={resource.refresh}>
    {data && <Card title={<Space>发信邮箱（SMTP）<Tag color={data.configured ? 'success' : 'default'}>{data.configured ? '已配置' : '未配置'}</Tag></Space>}>
      <SourceNotice source={data.source} readonly={!writable} env="SMTP_*" />
      <Typography.Paragraph type="secondary">用于发送订单确认信和订单访问验证码。密码加密保存，不会回传到浏览器。</Typography.Paragraph>
      {task.error && <Alert type="error" showIcon title={task.error} />}
      <Form name="smtp" form={form} layout="vertical" disabled={!writable || task.busy} onFinish={save}>
        <Form.Item name="host" label="SMTP 主机" rules={[{ required: true, whitespace: true, message: '请填写 SMTP 主机' }]}><Input autoComplete="off" placeholder="smtp.example.com" /></Form.Item>
        <div className="settings-columns">
          <Form.Item name="port" label="端口" rules={[{ required: true, message: '请填写端口' }, { validator: (_, value: string) => /^\d+$/.test((value ?? '').trim()) && Number(value) >= 1 && Number(value) <= 65535 ? Promise.resolve() : Promise.reject(new Error('端口需为 1–65535 的整数')) }]}><Input inputMode="numeric" /></Form.Item>
          <Form.Item name="encryption" label="加密方式"><Select options={[{ value: 'starttls', label: 'STARTTLS（常用 587）' }, { value: 'tls', label: 'SSL/TLS（常用 465）' }, { value: 'none', label: '无（不安全）' }]} /></Form.Item>
        </div>
        <Form.Item name="username" label="SMTP 用户名" extra="通常是完整邮箱；无鉴权服务器可留空。"><Input autoComplete="off" /></Form.Item>
        <Form.Item name="password" label="密码 / 授权码" extra="留空保留原密码，密码中的空格会原样保存。"><Input.Password autoComplete="new-password" placeholder={!writable ? '凭证不会回传到浏览器' : data.has_password ? '已保存，留空不改' : '邮箱 SMTP 授权码'} /></Form.Item>
        <Form.Item name="from_address" label="发件地址（From）" rules={[{ required: true, message: '请填写发件地址' }, { transform: (value: string) => value.trim(), type: 'email', message: '请输入合法邮箱' }]}><Input autoComplete="off" /></Form.Item>
        <Form.Item name="from_name" label="发件人名称（可选）"><Input autoComplete="off" /></Form.Item>
        {writable && <Button icon={<SaveOutlined aria-hidden />} type="primary" htmlType="submit" loading={task.busy && operation === 'save'}>保存 SMTP</Button>}
      </Form>
      <div className="settings-test">
        <Form name="smtp-test" form={testForm} layout="vertical" disabled={!owner || !data.configured || data.source === 'demo' || task.busy} onFinish={sendTest}>
          <Form.Item name="to" label="发送测试邮件到" rules={[{ required: true, message: '请填写收件邮箱' }, { transform: (value: string) => value.trim(), type: 'email', message: '请输入合法邮箱' }]}><Input placeholder="test@example.com" autoComplete="off" /></Form.Item>
          <Button icon={<SendOutlined aria-hidden />} htmlType="submit" loading={task.busy && operation === 'test'}>发送测试邮件</Button>
        </Form>
        <Typography.Paragraph type="secondary">使用已保存并已解锁的配置发送。请先保存修改，再发送测试邮件。</Typography.Paragraph>
      </div>
    </Card>}
  </PageState></div>
}
