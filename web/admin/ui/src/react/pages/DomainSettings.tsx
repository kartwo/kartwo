// 域名设置 / Domain Settings
// 功能：保留来源只读、域名校验与生产重启及本地 HTTPS 提示
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:23:15

import { SaveOutlined } from '@ant-design/icons'
import { useEffect, useState } from 'react'
import { Alert, App, Button, Card, Form, Input, Typography } from 'antd'
import { settingsAPI } from '../settings-api'
import { useResource, useTask } from '../useResource'
import { PageState, SourceNotice } from '../PageState'
import { useSession } from '../SessionProvider'
import { can } from '../access'
import { useWizardActivity } from '../WizardActivity'

export default function DomainSettings() {
  const { state } = useSession()
  const resource = useResource(settingsAPI.domain)
  const task = useTask()
  const { message } = App.useApp()
  const [form] = Form.useForm<{ domain: string }>()
  const [saved, setSaved] = useState(false)
  const data = resource.data
  const writable = can(state.session, 'settings.domain') && !!data && !data.readonly && ['db', 'none'].includes(data.source)
  useWizardActivity(resource.loading || task.busy)
  useEffect(() => { if (data) form.setFieldsValue({ domain: data.domain }) }, [data, form])
  async function save(values: { domain: string }) {
    if (!writable) return
    await task.run(() => settingsAPI.saveDomain(values.domain), next => {
      resource.setData(next); setSaved(true)
      void message.success('域名已保存')
    })
  }
  return <div className="settings-page"><PageState {...resource} retry={resource.refresh}>
    {data && <Card title="店铺域名与 HTTPS">
      <SourceNotice source={data.source} readonly={!writable} env="KARTWO_DOMAIN" />
      {!data.https_capable && <Alert type="info" showIcon title="当前为本地开发模式" description="域名可以保存，但本地不会签发 HTTPS 证书。正式部署并重启后才会生效。" />}
      {task.error && <Alert type="error" showIcon title={task.error} />}
      <Form name="domain" form={form} layout="vertical" disabled={!writable || task.busy} onFinish={save}>
        <Form.Item name="domain" label="你的店铺域名" rules={[{ required: true, whitespace: true, message: '请输入域名' }, { transform: (value: string) => value.trim(), pattern: /^(?=.*\.)[a-zA-Z0-9.-]+$/, message: '请输入完整域名，不要带协议、路径或空格' }]} extra="只填域名本身，例如 shop.example.com，不要带 http:// 或路径。">
          <Input placeholder="shop.example.com" autoCapitalize="off" autoComplete="off" spellCheck={false} />
        </Form.Item>
        {writable && <Button icon={<SaveOutlined aria-hidden />} type="primary" htmlType="submit" loading={task.busy}>保存域名</Button>}
      </Form>
      {saved && <Alert className="settings-result" type="success" showIcon title="域名已保存" description={data.https_capable ? `请重启一次 Kartwo，之后才会为 ${data.domain} 申请并启用 HTTPS。需提前将域名解析到本服务器，并开放公网 80 / 443 端口。` : '当前为本地开发模式，不会签发 HTTPS；正式部署并重启后才会生效。'} />}
      <Typography.Paragraph type="secondary">保存域名不会自动重启程序，也不会立即切换当前访问地址。</Typography.Paragraph>
    </Card>}
  </PageState></div>
}
