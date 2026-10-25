// 翻译设置 / Translation Settings
// 功能：加密保存已有 DeepL 服务配置，保留按商家点击才调用及演示只读
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:55:07

import { useEffect } from 'react'
import { Alert, App, Button, Card, Form, Input, Select, Tag, Typography } from 'antd'
import { securitySettingsAPI, type TranslationFields } from '../security-settings-api'
import { useSession } from '../SessionProvider'
import { useResource, useTask } from '../useResource'
import { PageState } from '../PageState'
export default function TranslationSettings() {
  const { state } = useSession(), resource = useResource(securitySettingsAPI.translation), task = useTask()
  const [form] = Form.useForm<TranslationFields>(), { message } = App.useApp()
  const data = resource.data, writable = state.session?.role === 'owner'
  useEffect(() => { if (data) form.setFieldsValue({ plan: data.plan, api_key: '' }) }, [data, form])
  async function save(values: TranslationFields) {
    if (!writable) return
    await task.run(() => securitySettingsAPI.saveTranslation(values), next => {
      resource.setData(next); void message.success('DeepL 配置已保存')
    })
  }
  return <div className="settings-page"><PageState {...resource} retry={resource.refresh}>
    {data && <Card title="英文内容翻译" extra={<Tag color={data.has_api_key ? 'success' : 'default'}>{data.has_api_key ? '已配置' : '未配置'}</Tag>}>
      <Typography.Paragraph type="secondary">在商品页点击翻译按钮时，才会将中文标题、slug 或 SEO 描述发送给 DeepL 并生成英文建议；不会自动调用。API Key 加密保存且不会回传到浏览器。</Typography.Paragraph>
      <Typography.Paragraph><a href="https://www.deepl.com/en/developers" target="_blank" rel="noopener noreferrer">DeepL 官方开发者页面</a></Typography.Paragraph>
      {!writable && <Alert type="info" title="当前账号只允许查看翻译设置。" />}
      {task.error && <Alert type="error" showIcon title={task.error} />}
      <Form name="translation" form={form} layout="vertical" disabled={!writable || task.busy} onFinish={save}>
        <Form.Item label="服务商"><Input value="DeepL" disabled /></Form.Item>
        <Form.Item name="plan" label="套餐"><Select options={[{ value: 'developer', label: `Developer API：${data.developer_quota}` }, { value: 'growth', label: `Growth API：${data.growth_quota}` }]} /></Form.Item>
        <Typography.Paragraph type="secondary">{data.price_note}</Typography.Paragraph>
        <Form.Item name="api_key" label="DeepL API Key" extra="留空保留原密钥。"><Input.Password autoComplete="new-password" placeholder={data.has_api_key ? '已保存，留空不改' : '粘贴 DeepL API Key'} /></Form.Item>
        {writable && <Button type="primary" htmlType="submit" loading={task.busy}>保存配置</Button>}
      </Form>
    </Card>}
  </PageState></div>
}
