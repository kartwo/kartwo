// 数据导出 / Data Export
// 功能：店主通过浏览器原生流式下载完整 ZIP，演示无下载地址
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:27:44

import { DownloadOutlined } from '@ant-design/icons'
import { Alert, Button, Card, Descriptions, Typography } from 'antd'
import { useSession } from '../SessionProvider'
export default function ExportData() {
  const { state } = useSession(), owner = state.session?.role === 'owner'
  return <div className="settings-page"><Card title="导出数据">
    <Descriptions column={1} items={[
      { key: 'database', label: '数据库', children: 'SQLite 一致性快照' }, { key: 'media', label: '媒体文件', children: '包含' }, { key: 'certificates', label: '证书缓存', children: '不包含' },
    ]} />
    {!owner && <Alert type="info" title="当前账号只可查看导出说明，完整数据包仅店主可下载。" />}
    <Typography.Paragraph type="secondary">导出在新标签页发起，浏览器直接接收 ZIP。请在浏览器下载列表确认结果；如导出失败，新标签页会显示服务器提示。</Typography.Paragraph>
    <Button icon={<DownloadOutlined aria-hidden />} type="primary" disabled={!owner} href={owner ? '/admin/api/export' : undefined} target={owner ? '_blank' : undefined} rel="noopener noreferrer">导出 ZIP</Button>
  </Card></div>
}
