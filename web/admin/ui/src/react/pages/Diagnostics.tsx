// 本机诊断 / Local Diagnostics
// 功能：数据库连接、媒体与磁盘状态、自动备份和升级快照的只读可观测展示
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:17:21

import { ReloadOutlined } from '@ant-design/icons'
import { Alert, Button, Card, Descriptions, Tag } from 'antd'
import { observabilityAPI, formatBytes, backupTime } from '../observability-api'
import { useResource } from '../useResource'
import { PageState } from '../PageState'
export default function Diagnostics() {
  const resource = useResource(observabilityAPI.diagnostics), data = resource.data
  return <div className="read-page">
    <div className="read-page-toolbar"><span>查看本机数据库、媒体磁盘与本地保护点状态。</span><Button icon={<ReloadOutlined aria-hidden />} disabled={resource.loading} onClick={() => void resource.refresh()}>刷新诊断</Button></div>
    <PageState {...resource} retry={resource.refresh}>{data && <>
      <Card title="数据库"><Descriptions column={1} items={[{ key: 'connection', label: '连接', children: <Tag color="success">正常</Tag> }]} /></Card>
      <Card title="媒体文件"><Descriptions column={{ xs: 1, sm: 2 }} items={[
        { key: 'count', label: '资产数量', children: data.media.asset_count }, { key: 'original', label: '原图', children: formatBytes(data.media.original_bytes) },
        { key: 'derived', label: '派生文件', children: formatBytes(data.media.derivative_bytes) }, { key: 'total', label: '媒体合计', children: formatBytes(data.media.total_bytes) },
      ]} /></Card>
      <Card title="媒体目录磁盘">{data.disk.available ? <>
        <Descriptions column={{ xs: 1, sm: 2 }} items={[
          { key: 'total', label: '总容量', children: formatBytes(data.disk.total_bytes) }, { key: 'free', label: '可用空间', children: formatBytes(data.disk.free_bytes) }, { key: 'used', label: '已用空间', children: formatBytes(data.disk.used_bytes) },
        ]} />
        <Alert type={data.disk.health === 'critical' ? 'error' : data.disk.health === 'warning' ? 'warning' : 'success'} showIcon title={data.disk.health === 'critical' ? '磁盘空间紧急' : data.disk.health === 'warning' ? '磁盘空间偏低' : '磁盘容量充足'} description={data.disk.hint} />
      </> : <Alert type="warning" showIcon title={data.disk.message || '暂时无法读取媒体磁盘容量'} />}</Card>
      <Card title="本地备份与升级快照"><Descriptions column={{ xs: 1, sm: 2 }} items={[
        { key: 'automatic', label: '自动备份', children: `${data.backups.automatic_count} 份` }, { key: 'upgrade', label: '升级快照', children: `${data.backups.upgrade_count} 份` },
        { key: 'size', label: '保护点占用', children: formatBytes(data.backups.total_bytes) }, { key: 'latest', label: '最近生成', children: backupTime(data.backups.latest_at) },
      ]} />{data.backups.message && <Alert type="warning" showIcon title={data.backups.message} />}</Card>
    </>}</PageState>
  </div>
}
