// 审计日志 / Audit Events
// 功能：最近 100 条关键操作只读展示，错误重试及演示标识脱敏
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:17:21

import { ReloadOutlined } from '@ant-design/icons'
import { useCallback } from 'react'
import { Button, Card, Table, Typography } from 'antd'
import { observabilityAPI, type AuditEvent } from '../observability-api'
import { auditLabels } from '../audit-labels'
import { useSession } from '../SessionProvider'
import { useResource } from '../useResource'
import { PageState } from '../PageState'
export default function AuditEvents() {
  const { state } = useSession(), demo = state.session?.role === 'demo'
  const resource = useResource(useCallback((signal: AbortSignal) => observabilityAPI.audit(demo, signal), [demo]))
  return <div className="read-page">
    <div className="read-page-toolbar"><Typography.Paragraph type="secondary">最近 100 条关键后台操作；不记录口令、密钥、会话令牌或请求内容。</Typography.Paragraph><Button icon={<ReloadOutlined aria-hidden />} disabled={resource.loading} onClick={() => void resource.refresh()}>刷新日志</Button></div>
    <PageState {...resource} retry={resource.refresh}>{resource.data && <Card title="操作日志">
      <Table<AuditEvent> rowKey="public_id" dataSource={resource.data} pagination={false} scroll={{ x: 680 }} locale={{ emptyText: '尚无日志记录' }} columns={[
        { title: '时间', dataIndex: 'created_at', key: 'time' },
        { title: '操作人', dataIndex: 'admin_username', key: 'actor' },
        { title: '操作', key: 'action', render: (_, event) => auditLabels[event.action] || event.action },
        { title: '对象', key: 'target', render: (_, event) => event.target_public_id || '—' },
      ]} />
    </Card>}</PageState>
  </div>
}
