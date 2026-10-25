// 备份设置 / Backup Settings
// 功能：逐字段环境只读、本地策略及保存后测试 WebDAV，密码留空不变
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:55:07

import { useEffect, useState } from 'react'
import { Alert, App, Button, Card, Form, Input, Space, Switch, Typography } from 'antd'
import { backupKeys, securitySettingsAPI, type BackupFields, type BackupKey } from '../security-settings-api'
import { useSession } from '../SessionProvider'
import { useResource, useTask } from '../useResource'
import { PageState } from '../PageState'
import { APIError, errorMessage } from '../transport'
export default function BackupSettings() {
  const { state } = useSession(), resource = useResource(securitySettingsAPI.backup), task = useTask()
  const [form] = Form.useForm<BackupFields>(), { message } = App.useApp()
  const [operation, setOperation] = useState(''), [testError, setTestError] = useState('')
  const data = resource.data
  const owner = state.session?.role === 'owner' && !data?.demo_readonly
  const writable = !!data && owner && backupKeys.some(key => !data.readonly[key])
  const disabled = (key: BackupKey) => !owner || !data || data.readonly[key] || task.busy
  useEffect(() => { if (data) form.setFieldsValue({ interval: data.interval, retention: String(data.retention), webdav_enabled: data.webdav_enabled, webdav_url: data.webdav_url, webdav_path: data.webdav_path || '/', webdav_username: data.webdav_username, webdav_password: '' }) }, [data, form])
  async function save(values: BackupFields) {
    if (!writable || !data) return
    setTestError('')
    await task.run(async () => { setOperation('save'); return securitySettingsAPI.saveBackup(values, data) }, next => {
      resource.setData(next); void message.success('自动备份设置已保存；本地策略重启后生效')
    })
  }
  async function test() {
    if (!owner || !data) return
    let values: BackupFields
    try { values = await form.validateFields() } catch { return }
    if (!values.webdav_enabled || !values.webdav_url.trim() || !values.webdav_path.trim() || (!data.webdav_password_set && !values.webdav_password)) {
      setTestError('请先启用 WebDAV，并填写地址、目录和密码。'); return
    }
    setTestError('')
    await task.run(async () => {
      setOperation('test')
      const saved = writable ? await securitySettingsAPI.saveBackup(values, data) : data
      try { await securitySettingsAPI.testBackup(); return { saved, error: '' } }
      catch (error) {
        if (error instanceof APIError && error.status === 401) throw error
        return { saved, error: errorMessage(error) }
      }
    }, result => {
      resource.setData(result.saved)
      if (result.error) setTestError(`配置已保存，连接测试失败：${result.error}`)
      else void message.success('WebDAV 测试成功：可连通并可写入')
    })
  }
  return <div className="settings-page"><PageState {...resource} retry={resource.refresh}>
    {data && <Card title="自动备份">
      <Typography.Paragraph type="secondary">服务启动时会立即生成一份完整 ZIP，之后按设定周期继续备份。本地周期与保留份数保存后下次重启生效。</Typography.Paragraph>
      {!owner ? <Alert type="info" title="当前账号只允许查看备份设置；公开演示已隐藏远端地址和账号。" /> : backupKeys.some(key => data.readonly[key]) && <Alert type="info" title="环境变量提供的字段只读。" description="修改对应 KARTWO_BACKUP_* 环境变量后重启即可生效；其他字段仍可在此保存。" />}
      {task.error && <Alert type="error" showIcon title={task.error} />}
      {testError && <Alert type="error" showIcon title={testError} />}
      <Form name="backup" form={form} layout="vertical" disabled={!owner || task.busy} onFinish={save}>
        <Form.Item name="interval" label="备份周期" extra="Go duration，例如 24h、90m，最小 1m。" rules={disabled('interval') ? [] : [{ required: true, whitespace: true, message: '请输入备份周期' }]}><Input disabled={disabled('interval')} autoComplete="off" /></Form.Item>
        <Form.Item name="retention" label="保留自动备份份数" extra="允许 1–365。升级前快照独立保留，不受此数字清理。" rules={disabled('retention') ? [] : [{ validator: (_, value: string) => /^\d+$/.test((value ?? '').trim()) && Number(value) >= 1 && Number(value) <= 365 ? Promise.resolve() : Promise.reject(new Error('请输入 1–365 的整数')) }]}><Input disabled={disabled('retention')} inputMode="numeric" /></Form.Item>
        <Typography.Title level={3}>WebDAV 异地备份</Typography.Title>
        <Form.Item name="webdav_enabled" label="启用 WebDAV 异地备份" valuePropName="checked"><Switch disabled={disabled('webdav_enabled')} /></Form.Item>
        <Form.Item name="webdav_url" label="WebDAV 地址（HTTPS）" rules={disabled('webdav_url') ? [] : [{ validator: (_, value: string) => {
          if (!value?.trim()) return Promise.resolve()
          try { const url = new URL(value.trim()); if (url.protocol === 'https:' && url.hostname && !url.username && !url.password) return Promise.resolve() } catch { /* Show a fixed validation error. */ }
          return Promise.reject(new Error('请输入 HTTPS 地址，用户名和密码请填在独立输入框中'))
        } }]}><Input disabled={disabled('webdav_url')} placeholder="https://dav.example.com" autoComplete="off" /></Form.Item>
        <Form.Item name="webdav_path" label="WebDAV 目录" rules={disabled('webdav_path') ? [] : [{ validator: (_, value: string) => !value?.trim() || /^\/[^ ]*$/.test(value.trim()) ? Promise.resolve() : Promise.reject(new Error('目录必须以 / 开头，不能包含空格')) }]}><Input disabled={disabled('webdav_path')} placeholder="/kartwo/backups" autoComplete="off" /></Form.Item>
        <Form.Item name="webdav_username" label="WebDAV 用户名"><Input disabled={disabled('webdav_username')} autoComplete="off" /></Form.Item>
        <Form.Item name="webdav_password" label="WebDAV 密码 / 授权码" extra="留空保留原密码，密码中的空格原样保存。"><Input.Password disabled={disabled('webdav_password')} autoComplete="new-password" placeholder={data.webdav_password_set ? '已配置，留空保持不变' : '请输入密码'} /></Form.Item>
        <Space wrap>
          {writable && <Button type="primary" htmlType="submit" loading={task.busy && operation === 'save'}>保存备份设置</Button>}
          <Button disabled={!owner || task.busy} loading={task.busy && operation === 'test'} onClick={() => void test()}>{writable ? '保存并测试连接' : '测试已配置连接'}</Button>
        </Space>
      </Form>
      <Typography.Paragraph className="settings-result" type="secondary">测试会使用保存后的配置，在远端写入一个临时文件。无人值守的启动即上传仍须通过 KARTWO_BACKUP_WEBDAV_* 环境变量提供完整配置。</Typography.Paragraph>
    </Card>}
  </PageState></div>
}
