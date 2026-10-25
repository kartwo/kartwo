// 登录与邀请 / Login and Invitation
// 功能：Ant Design 初始化、登录、演示和员工一次性邀请表单
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:11:05

import { useState } from 'react'
import { Alert, Button, Card, Form, Input, Result, Space, Spin, Typography } from 'antd'
import { Link, useLocation } from 'react-router-dom'
import { useSession } from './SessionProvider'
import { errorMessage } from './transport'

interface Credentials { username: string; password: string }
interface Invitation { password: string; confirm: string }
function InvitationForm({ token }: { token: string }) {
  const { controller } = useSession()
  const [inviteForm] = Form.useForm<Invitation>()
  const [invite, setInvite] = useState({ busy: false, done: false, error: '' })
  async function accept(values: Invitation) {
    if (invite.busy || !token) return
    setInvite({ busy: true, done: false, error: '' })
    try {
      await controller.api.acceptInvite(token, values.password)
      inviteForm.resetFields()
      setInvite({ busy: false, done: true, error: '' })
    } catch (error) { setInvite({ busy: false, done: false, error: errorMessage(error) }) }
  }
  return <div className="auth-screen"><Card className="auth-card" title="设置员工密码">
    {!token ? <Alert type="error" title="邀请链接不完整，请联系店主重新发送。" /> : invite.done ?
      <Result status="success" title="密码已设置" extra={<Link to="/dashboard">返回登录</Link>} /> : <>
        <Typography.Paragraph>此链接一次有效，请设置至少 8 位的独立登录密码。</Typography.Paragraph>
        {invite.error && <Alert type="error" showIcon title={invite.error} />}
        <Form form={inviteForm} layout="vertical" onFinish={accept} disabled={invite.busy}>
          <Form.Item name="password" label="新密码" rules={[{ required: true, min: 8, message: '密码至少 8 位' }]}><Input.Password autoComplete="new-password" /></Form.Item>
          <Form.Item name="confirm" label="确认密码" dependencies={['password']} rules={[{ required: true, message: '请再次输入密码' }, ({ getFieldValue }) => ({ validator: (_, value: unknown) => value === getFieldValue('password') ? Promise.resolve() : Promise.reject(new Error('两次输入的密码不一致')) })]}><Input.Password autoComplete="new-password" /></Form.Item>
          <Button type="primary" htmlType="submit" loading={invite.busy} block>设置密码</Button>
        </Form>
      </>}
  </Card></div>
}

function LoginForm() {
  const { state, controller } = useSession()
  const [form] = Form.useForm<Credentials>()
  const setup = state.phase === 'setup'
  return <div className="auth-screen"><Card className="auth-card">
    <div className="auth-brand"><span className="brand-mark" aria-hidden="true">K</span><strong>Kartwo</strong></div>
    <Typography.Title level={2}>{setup ? '初始化 Kartwo' : '欢迎回来，继续经营你的店铺。'}</Typography.Title>
    <Typography.Paragraph type="secondary">{setup ? '创建管理员并设置主口令，用于登录与配置加密。' : '管理商品、订单和店面内容。'}</Typography.Paragraph>
    {state.error && <Alert type="error" showIcon title={state.error} />}
    <Form form={form} layout="vertical" disabled={state.busy} onFinish={(values: Credentials) => {
      void controller.authenticate(setup ? 'setup' : 'login', values.username, values.password)
      form.setFieldValue('password', '')
    }}>
      <Form.Item name="username" label={setup ? '管理员用户名' : '用户名'} rules={[{ required: true, whitespace: true, message: '请输入用户名' }]}><Input autoComplete="username" /></Form.Item>
      <Form.Item name="password" label={setup ? '主口令（至少 8 位）' : '口令'} rules={[{ required: true, message: '请输入口令' }, ...(setup ? [{ min: 8, message: '口令至少 8 位' }] : [])]}><Input.Password autoComplete={setup ? 'new-password' : 'current-password'} /></Form.Item>
      <Space orientation="vertical" className="full-width">
        <Button type="primary" htmlType="submit" block loading={state.busy}>{setup ? '创建并登录' : '登录后台'}</Button>
        {!setup && state.demoAvailable && <Button block disabled={state.busy} onClick={() => void controller.authenticate('demo')}>体验公开演示</Button>}
      </Space>
    </Form>
  </Card></div>
}

export function AuthScreen() {
  const { state, controller } = useSession()
  const { pathname, search } = useLocation()
  const token = new URLSearchParams(search).get('token') ?? ''
  if (pathname === '/accept-invite') return <InvitationForm key={token} token={token} />
  if (state.phase === 'loading') return <div className="auth-screen" role="status"><Spin /><span>加载中…</span></div>
  if (state.phase === 'error') return <Result status="error" title="无法连接后台" subTitle={state.error} extra={<Button onClick={() => void controller.refresh()}>重试</Button>} />
  return <LoginForm key={state.phase} />
}
