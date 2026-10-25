// 账号安全 / Account Security
// 功能：店主修改凭据、员工修改自身密码，成功后立即清除当前会话
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:55:07

import { useEffect } from 'react'
import { Alert, App, Button, Card, Form, Input, Typography } from 'antd'
import { securitySettingsAPI, type AccountFields } from '../security-settings-api'
import { useSession } from '../SessionProvider'
import { useResource, useTask } from '../useResource'
import { PageState } from '../PageState'
interface Fields extends AccountFields { confirm_password: string }
export default function AccountSettings() {
  const { state, controller } = useSession(), resource = useResource(securitySettingsAPI.account), task = useTask()
  const { message } = App.useApp(), [form] = Form.useForm<Fields>()
  const role = state.session?.role, data = resource.data
  const writable = role === 'staff' || (role === 'owner' && data && !data.readonly)
  useEffect(() => { if (data) form.setFieldsValue({ username: data.username, current_password: '', new_password: '', confirm_password: '' }) }, [data, form])
  async function save(values: Fields) {
    if (!writable) return
    await task.run(() => role === 'staff' ? securitySettingsAPI.saveStaff(values) : securitySettingsAPI.saveOwner(values), () => {
      form.resetFields()
      controller.unauthorized()
      void message.success('账号已更新，该账号的所有后台会话已退出，请使用新凭据重新登录。', 8)
    })
  }
  return <div className="settings-page"><PageState {...resource} retry={resource.refresh}>
    {data && <Card title="账号与安全">
      <Typography.Paragraph type="secondary">{role === 'staff' ? '修改自己的登录密码，不影响店主主口令及收款配置。' : '修改用户名或主密码都必须输入当前密码。保存成功后，该账号的所有后台会话都会退出。'}</Typography.Paragraph>
      {!writable && <Alert type="info" title="当前账号只允许查看账号安全设置。公开演示已隐藏管理员身份。" />}
      {task.error && <Alert type="error" showIcon title={task.error} />}
      <Form name="account" form={form} layout="vertical" disabled={task.busy || !writable} onFinish={save}>
        <Form.Item name="username" label="管理员用户名" rules={role === 'owner' ? [{ required: true, whitespace: true, message: '请输入管理员用户名' }, { validator: (_, value: string) => Array.from((value ?? '').trim()).length <= 64 && !/[\u0000-\u001f\u007f-\u009f]/.test(value ?? '') ? Promise.resolve() : Promise.reject(new Error('用户名最多 64 个字符，不能含控制字符')) }] : []}>
          <Input autoComplete="username" disabled={role !== 'owner' || !writable || task.busy} />
        </Form.Item>
        {writable && <>
          <Form.Item name="current_password" label="当前密码" rules={[{ required: true, message: '请输入当前密码确认身份' }]}><Input.Password autoComplete="current-password" /></Form.Item>
          <Form.Item name="new_password" label={role === 'staff' ? '新密码' : '新密码（可选）'} extra={role === 'staff' ? '新密码至少 8 位。' : '留空表示只修改用户名；新密码至少 8 位。'} rules={[{ required: role === 'staff', message: '请输入新密码' }, { min: 8, message: '新密码至少 8 位' }]}><Input.Password autoComplete="new-password" /></Form.Item>
          <Form.Item name="confirm_password" label="确认新密码" dependencies={['new_password']} rules={[({ getFieldValue }) => ({ validator: (_, value: string) => (value ?? '') === (getFieldValue('new_password') ?? '') ? Promise.resolve() : Promise.reject(new Error('两次输入的新密码不一致')) })]}><Input.Password autoComplete="new-password" /></Form.Item>
          {role === 'owner' && <Alert type="warning" showIcon title="请妥善保存新密码。" description="它同时保护后台登录和收款、邮件等加密配置；遗忘后无法恢复这些加密凭据。" />}
          <Button type="primary" htmlType="submit" loading={task.busy}>保存并退出该账号所有会话</Button>
        </>}
      </Form>
    </Card>}
  </PageState></div>
}
