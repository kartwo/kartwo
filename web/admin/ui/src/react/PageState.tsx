// 页面反馈 / Page Feedback
// 功能：统一加载、失败重试和只读来源提示
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:21:49

import type { ReactNode } from 'react'
import { Alert, Button, Spin } from 'antd'
import type { Source } from './settings-api'
export function PageState({ loading, error, retry, children }: { loading: boolean; error: string; retry: () => Promise<void>; children: ReactNode }) {
  if (loading) return <div className="page-loading" role="status"><Spin /><span>加载中…</span></div>
  if (error) return <Alert type="error" showIcon title={error} action={<Button onClick={() => void retry()}>重试</Button>} />
  return children
}
export function SourceNotice({ source, readonly, env }: { source: Source; readonly: boolean; env: string }) {
  if (source === 'demo') return <Alert type="info" showIcon title="公开演示只读预览，敏感配置已隐藏。" />
  if (source === 'env') return <Alert type="info" showIcon title="由环境变量提供，此处只读。" description={`修改 ${env} 后重启，或清空该环境配置后改用后台设置。`} />
  return readonly ? <Alert type="info" title="当前账号或配置只允许查看。" /> : null
}
