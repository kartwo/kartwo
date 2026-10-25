// 博客开关 / Blog Settings
// 功能：店主保存博客启用状态，员工与演示只读，读取失败不可误覆盖
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:34:00

import { useEffect } from 'react'
import { Alert, App, Button, Card, Form, Switch, Typography } from 'antd'
import { storeSettingsAPI, type BlogSettings as Fields } from '../store-settings-api'
import { useSession } from '../SessionProvider'
import { useResource, useTask } from '../useResource'
import { PageState } from '../PageState'

export default function BlogSettings() {
  const { state } = useSession(), resource = useResource(storeSettingsAPI.blog), task = useTask()
  const [form] = Form.useForm<Fields>()
  const { message } = App.useApp()
  const writable = state.session?.role === 'owner'
  useEffect(() => { if (resource.data) form.setFieldsValue(resource.data) }, [resource.data, form])
  async function save(values: Fields) {
    if (!writable) return
    await task.run(() => storeSettingsAPI.saveBlog(values.enabled), next => {
      resource.setData(next); void message.success('博客设置已保存')
    })
  }
  return <div className="settings-page"><PageState {...resource} retry={resource.refresh}>
    {resource.data && <Card title="博客设置">
      {!writable && <Alert type="info" title="仅店主可修改设置。" />}
      {task.error && <Alert type="error" showIcon title={task.error} />}
      <Form name="blog-settings" form={form} layout="vertical" disabled={!writable || task.busy} onFinish={save}>
        <Form.Item name="enabled" label="开启店面博客" valuePropName="checked"><Switch /></Form.Item>
        <Typography.Paragraph type="secondary">默认开启。关闭后顾客无法访问博客，导航和站点地图不再显示；文章与草稿都会保留。</Typography.Paragraph>
        {writable && <Button type="primary" htmlType="submit" loading={task.busy}>保存设置</Button>}
      </Form>
    </Card>}
  </PageState></div>
}
