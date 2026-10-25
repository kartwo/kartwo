// 客服收件箱 / Contact Inbox
// 功能：私密留言筛选与详情、处理人和匿名化展示，保护切换竞态与状态写入
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:17:21

import { useCallback, useRef, useState } from 'react'
import { Alert, App, Button, Card, Descriptions, Empty, Select, Space, Typography } from 'antd'
import { contactAPI, contactStatuses, contactTopics, type ContactStatus } from '../contact-api'
import { can } from '../access'
import { useSession } from '../SessionProvider'
import { useResource, useTask } from '../useResource'
import { PageState } from '../PageState'
function countryName(code: string) {
  if (!code) return '未知（未获得可信定位）'
  try { return `${new Intl.DisplayNames(['zh-CN'], { type: 'region' }).of(code) || code} (${code})` }
  catch { return code }
}
export default function ContactInbox() {
  const { state } = useSession(), { message } = App.useApp()
  const [status, setStatus] = useState<ContactStatus | ''>(''), [selected, setSelected] = useState('')
  const lock = useRef(false), task = useTask(), demo = state.session?.role === 'demo'
  const list = useResource(useCallback((signal: AbortSignal) => contactAPI.list(status, demo, signal), [status, demo]))
  const detail = useResource(useCallback((signal: AbortSignal) => selected && !demo ? contactAPI.get(selected, signal) : Promise.resolve(null), [selected, demo]))
  const row = detail.data?.public_id === selected ? detail.data : null, canManage = can(state.session, 'contact.manage')
  function open(id: string) { if (!lock.current && !demo) setSelected(id) }
  function filter(value: ContactStatus | '') { if (!lock.current) { setSelected(''); setStatus(value) } }
  async function refresh() { if (!lock.current) { setSelected(''); await list.refresh() } }
  async function changeStatus(next: ContactStatus) {
    if (!row || !canManage || row.anonymized_at || row.status === next || lock.current) return
    lock.current = true
    try {
      await task.run(() => contactAPI.setStatus(row.public_id, next), result => {
        detail.setData(result)
        list.setData(current => current && { ...current, messages: current.messages.map(message => message.public_id === result.public_id ? result : message).filter(message => !status || message.status === status) })
        void message.success('处理状态已更新')
      })
    } finally { lock.current = false }
  }
  return <div className="inbox-page">
    <Typography.Paragraph type="secondary">访客留言仅在后台可见；没有公开展示或普通删除操作。回复请使用商家邮箱。请勿索取密码或完整银行卡信息。</Typography.Paragraph>
    <Space wrap><label htmlFor="contact-filter">留言状态</label><Select id="contact-filter" value={status} disabled={task.busy} onChange={filter} options={[{ value: '', label: '全部' }, ...Object.entries(contactStatuses).map(([value, label]) => ({ value, label }))]} /><Button disabled={list.loading || task.busy} onClick={() => void refresh()}>刷新留言</Button></Space>
    {task.error && <Alert type="error" showIcon title={task.error} />}
    <PageState {...list} retry={list.refresh}>{list.data && <>
      {list.data.demo_hidden && <Alert type="info" title="公开演示不会展示真实顾客留言、邮箱或 IP。正式部署并开启后，这里显示私密收件箱。" />}
      <div className="inbox-columns"><Card title="留言列表">
        {!list.data.messages.length && <Empty description="暂无留言" />}
        {list.data.messages.map(item => <Button key={item.public_id} block className="inbox-message-button" type={selected === item.public_id ? 'primary' : 'default'} disabled={task.busy} onClick={() => open(item.public_id)}>
          <strong>{contactTopics[item.topic]} · {contactStatuses[item.status]}</strong><span>{item.name || '已匿名化'} · {item.created_at}</span>
          {item.status === 'processing' && item.processing_by_name && <span>处理人：{item.processing_by_name}</span>}{item.status === 'closed' && item.closed_by_name && <span>结案人：{item.closed_by_name}</span>}
        </Button>)}
      </Card><Card title="留言详情"><PageState {...detail} retry={detail.refresh}>
        {!row ? <Typography.Paragraph type="secondary">选择一条留言查看详情。</Typography.Paragraph> : <>
          <Descriptions column={1} bordered size="small" items={[
            { key: 'status', label: '状态', children: contactStatuses[row.status] },
            { key: 'processing', label: row.status === 'processing' ? '处理人' : '最近处理人', children: `${row.processing_by_name || '尚无记录（或为升级前操作）'}${row.processing_at ? ' · ' + row.processing_at : ''}` },
            { key: 'closed', label: row.status === 'closed' ? '结案人' : '上次结案人', children: `${row.closed_by_name || '尚无记录（或为升级前操作）'}${row.closed_at ? ' · ' + row.closed_at : ''}` },
            { key: 'name', label: '姓名', children: row.name || '已匿名化' }, { key: 'email', label: '邮箱', children: row.email || '已匿名化' },
            { key: 'topic', label: '主题', children: contactTopics[row.topic] }, { key: 'order', label: '订单号', children: row.order_ref || '—' },
            { key: 'ip', label: 'IP 地址', children: row.ip_address || '已清除 / 未记录' }, { key: 'country', label: '大致所属地', children: countryName(row.country_code) },
            { key: 'time', label: '时间', children: row.created_at }, { key: 'body', label: '内容', children: <span className="inbox-body">{row.body || '已匿名化'}</span> },
          ]} />
          {row.anonymized_at ? <Alert className="settings-result" type="info" title="此留言已到期匿名化，无法再修改处理状态。" /> : canManage && <Space wrap className="settings-result"><Button disabled={task.busy || row.status === 'processing'} onClick={() => void changeStatus('processing')}>标记处理中</Button><Button disabled={task.busy || row.status === 'closed'} onClick={() => void changeStatus('closed')}>结案</Button><Button disabled={task.busy || row.status === 'new'} onClick={() => void changeStatus('new')}>重开</Button></Space>}
          <Typography.Paragraph className="settings-result" type="secondary">IP 所属地仅在可信代理提供国家码时显示，不代表精确位置。</Typography.Paragraph>
        </>}
      </PageState></Card></div>
    </>}</PageState>
  </div>
}
