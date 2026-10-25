// 客服设置 / Contact Settings
// 功能：店主控制私密留言表单及匿名化周期，保留员工和演示只读
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:34:00

import { SaveOutlined } from '@ant-design/icons'
import { useEffect } from 'react'
import { Alert, App, Button, Card, Form, InputNumber, Switch, Typography } from 'antd'
import { storeSettingsAPI, type ContactSettings as Fields } from '../store-settings-api'
import { useSession } from '../SessionProvider'
import { useResource, useTask } from '../useResource'
import { PageState } from '../PageState'

export default function ContactSettings() {
  const { state } = useSession(), resource = useResource(storeSettingsAPI.contact), task = useTask()
  const [form] = Form.useForm<Fields>()
  const { message } = App.useApp()
  const writable = state.session?.role === 'owner'
  useEffect(() => { if (resource.data) form.setFieldsValue(resource.data) }, [resource.data, form])
  async function save(values: Fields) {
    if (!writable) return
    await task.run(() => storeSettingsAPI.saveContact(values), next => {
      resource.setData(next); void message.success('客服留言设置已保存')
    })
  }
  return <div className="settings-page"><PageState {...resource} retry={resource.refresh}>
    {resource.data && <Card title="客服留言设置">
      {!writable && <Alert type="info" title="仅店主可修改设置。" />}
      {task.error && <Alert type="error" showIcon title={task.error} />}
      <Typography.Paragraph type="secondary">这是私密客服表单，不是公开留言板。默认关闭；开启后店面页脚的 Contact Us 会打开表单。</Typography.Paragraph>
      <Form name="contact-settings" form={form} layout="vertical" disabled={!writable || task.busy} onFinish={save}>
        <Form.Item name="enabled" label="开启在线留言" valuePropName="checked"><Switch /></Form.Item>
        <Form.Item name="retention_months" label="结案后保留完整留言（月）" rules={[{ required: true, type: 'integer', min: 1, max: 24, message: '请输入 1–24 的整数' }]}><InputNumber min={1} max={24} /></Form.Item>
        <Form.Item name="ip_retention_days" label="IP 地址保留（天）" rules={[{ required: true, type: 'integer', min: 7, max: 90, message: '请输入 7–90 的整数' }]}><InputNumber min={7} max={90} /></Form.Item>
        <Typography.Paragraph type="secondary">默认：结案后 12 个月匿名化姓名、邮箱、正文和订单号；IP 从提交起最多保留 90 天。未结案留言保留处理所需内容。记录本身不会从后台普通操作中删除。到期清理任务每日运行。</Typography.Paragraph>
        {writable && <Button icon={<SaveOutlined aria-hidden />} type="primary" htmlType="submit" loading={task.busy}>保存设置</Button>}
      </Form>
    </Card>}
  </PageState></div>
}
