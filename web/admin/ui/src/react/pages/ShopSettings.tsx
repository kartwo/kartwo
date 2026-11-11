// 店铺设置 / Shop Settings
// 功能：编辑品牌名称及上传移除 Logo，保留员工权限与演示只读
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:34:00

import { FilePicker } from '../FilePicker'
import { DeleteOutlined, SaveOutlined } from '@ant-design/icons'
import type { ChangeEvent } from 'react'
import { Alert, App, Button, Card, Form, Input, Typography } from 'antd'
import { storeSettingsAPI } from '../store-settings-api'
import { PageState } from '../PageState'
import { useResource, useTask } from '../useResource'
import { useSession } from '../SessionProvider'
import { can } from '../access'

export default function ShopSettings() {
  const { state } = useSession()
  const resource = useResource(storeSettingsAPI.shop), task = useTask()
  const { message } = App.useApp()
  const [form] = Form.useForm<{ name: string }>()
  const data = resource.data
  const writable = can(state.session, 'settings.shop') && data?.source === 'db' && !data.readonly
  async function save(values: { name: string }) {
    if (!writable) return
    await task.run(() => storeSettingsAPI.saveShop(values.name), next => {
      resource.setData(next); form.setFieldsValue({ name: next.name })
      void message.success('店铺名称已保存，店面会立即使用新名称。')
    })
  }
  async function upload(event: ChangeEvent<HTMLInputElement>) {
    const file = event.currentTarget.files?.[0]
    event.currentTarget.value = ''
    if (!writable || !file) return
    await task.run(() => storeSettingsAPI.uploadLogo(file), next => {
      resource.setData(next); void message.success('Logo 已上传，店面页眉现在只显示 Logo。')
    })
  }
  async function remove() {
    if (!writable || !data?.logo_url) return
    await task.run(storeSettingsAPI.removeLogo, next => {
      resource.setData(next); void message.success('Logo 已删除，店面页眉恢复显示店铺名称。')
    })
  }
  return <div className="settings-page"><PageState {...resource} retry={resource.refresh}>
    {data && <Card title="店铺品牌">
      {!writable && <Alert type="info" title="当前账号只允许查看店铺设置。" />}
      {task.error && <Alert type="error" showIcon title={task.error} />}
      <Typography.Paragraph type="secondary">店铺名称用于页脚、浏览器标题与 SEO；上传 Logo 后，店面左上角只显示 Logo。</Typography.Paragraph>
      {/* Seed the field when the form mounts; only a confirmed name save may replace its draft. */}
      <Form name="shop" form={form} initialValues={{ name: data.name }} layout="vertical" disabled={!writable || task.busy} onFinish={save}>
        <Form.Item name="name" label="店铺名称" extra="最多 120 个字符。" rules={[{ required: true, whitespace: true, message: '请填写店铺名称' }, { validator: (_, value: string) => Array.from((value ?? '').trim()).length <= 120 && !/[\u0000-\u001f\u007f-\u009f]/.test(value ?? '') ? Promise.resolve() : Promise.reject(new Error('店铺名称最多 120 个字符，不能含控制字符')) }]}>
          <Input autoComplete="organization" />
        </Form.Item>
        {writable && <Button icon={<SaveOutlined aria-hidden />} type="primary" htmlType="submit" loading={task.busy}>保存店铺名称</Button>}
      </Form>
      <div className="settings-test">
        <Typography.Title level={3}>店铺 Logo</Typography.Title>
        <Typography.Paragraph type="secondary">支持 JPG、PNG 或 WebP，建议使用透明背景横版图；系统会压缩并去除图片元数据。</Typography.Paragraph>
        {data.logo_url && <div className="brand-logo-preview"><img src={data.logo_url} alt={`${data.name} Logo`} /></div>}
        <div className="brand-logo-actions">
          <label htmlFor="shop-logo">{data.logo_url ? '更换 Logo' : '上传 Logo'}</label>
          <FilePicker label="店铺 Logo" buttonLabel="选择 Logo 文件" id="shop-logo" accept="image/jpeg,image/png,image/webp" disabled={!writable || task.busy} onChange={event => void upload(event)} />
          {writable && data.logo_url && <Button icon={<DeleteOutlined aria-hidden />} danger disabled={task.busy} onClick={() => void remove()}>删除 Logo</Button>}
        </div>
      </div>
    </Card>}
  </PageState></div>
}
