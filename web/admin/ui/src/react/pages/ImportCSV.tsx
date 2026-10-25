// CSV 商品导入 / CSV Product Import
// 功能：通用及 Shopify 预览、行错误展示、同任务确认和只读状态核对
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:27:44

import { useRef, useState, type ChangeEvent } from 'react'
import { Alert, App, Button, Card, Descriptions, Select, Space, Table, Typography } from 'antd'
import { Link } from 'react-router-dom'
import { importAPI, type ImportFormat, type ImportPreview } from '../import-api'
import { can, canVisit, matchRoute } from '../access'
import { useSession } from '../SessionProvider'
import { useTask } from '../useResource'
export default function ImportCSV() {
  const { state } = useSession(), task = useTask(), { message } = App.useApp()
  const [file, setFile] = useState<File | null>(null), [format, setFormat] = useState<ImportFormat>('generic')
  const [preview, setPreview] = useState<ImportPreview | null>(null), [operation, setOperation] = useState('')
  const lock = useRef(false), writable = can(state.session, 'imports.manage')
  function choose(event: ChangeEvent<HTMLInputElement>) {
    if (!writable || lock.current) return
    setFile(event.currentTarget.files?.[0] ?? null); setPreview(null); task.clearError()
  }
  function changeFormat(value: ImportFormat) { if (writable && !lock.current) { setFormat(value); setPreview(null); task.clearError() } }
  async function run(kind: 'preview' | 'execute' | 'check') {
    if (!writable || lock.current || (kind === 'preview' ? !file : !preview)) return
    lock.current = true
    try {
      await task.run(async () => {
        setOperation(kind)
        if (kind === 'preview') { setPreview(null); return importAPI.preview(file!, format) }
        return kind === 'execute' ? importAPI.execute(preview!) : importAPI.check(preview!)
      }, next => {
        setPreview(next)
        if (next.status === 'succeeded') void message.success(kind === 'execute' ? '商品已导入' : '该任务已成功导入，无需再次确认')
      })
    } finally { lock.current = false }
  }
  return <div className="read-page">
    <div className="read-page-toolbar"><Typography.Paragraph type="secondary">先预览检查，确认无错误后才会写入商品。</Typography.Paragraph>{canVisit(state.session, matchRoute('/products')) && <Link to="/products">返回商品</Link>}</div>
    {!writable && <Alert type="info" title="当前账号只允许查看导入说明，无法上传或执行导入。" />}
    {task.error && <Alert type="error" showIcon title={task.error} />}
    <Card title="选择 CSV"><div className="import-fields">
      <label htmlFor="import-format">导入来源</label><Select id="import-format" value={format} disabled={!writable || task.busy} onChange={changeFormat} options={[{ value: 'generic', label: '通用 CSV' }, { value: 'shopify', label: 'Shopify 商品 CSV' }]} />
      <label htmlFor="import-file">CSV 文件</label><input id="import-file" type="file" accept=".csv,text/csv" disabled={!writable || task.busy} onChange={choose} />
      <Typography.Paragraph type="secondary">{format === 'generic' ? '列：title、slug、status、description、option1_name、option1_value、option2_name、option2_value、sku、price_cents、quantity。金额填写整数分。' : '支持 Shopify 商品、两轴变体、SKU、价格、库存、状态和公开 HTTPS 图片；暂不支持第三变体轴。'} 单个 CSV 最多 5MB。</Typography.Paragraph>
      <div><Button type="primary" disabled={!writable || !file || task.busy} loading={task.busy && operation === 'preview'} onClick={() => void run('preview')}>预览导入</Button></div>
    </div></Card>
    {preview && <Card title="预览结果"><Descriptions column={{ xs: 1, sm: 3 }} items={[
      { key: 'rows', label: '行数', children: preview.total_rows }, { key: 'products', label: '商品', children: preview.product_count }, { key: 'variants', label: '变体', children: preview.variant_count },
    ]} />
      {preview.status === 'succeeded' ? <Alert type="success" title="该文件已成功导入；重复确认不会再创建商品。" /> : preview.status === 'rejected' || preview.errors.length ? <>
        <Alert type="error" title={`预览未通过${preview.errors.length ? `：发现 ${preview.errors.length} 行错误` : ''}，修正 CSV 后请重新预览。`} />
        {!!preview.errors.length && <Table rowKey={(_, index) => String(index)} dataSource={preview.errors} pagination={false} columns={[{ title: '行', dataIndex: 'row' }, { title: '问题', dataIndex: 'message' }]} />}
      </> : <>
        <Space wrap><Button type="primary" disabled={!writable || task.busy} loading={task.busy && operation === 'execute'} onClick={() => void run('execute')}>确认导入</Button><Button disabled={!writable || task.busy} loading={task.busy && operation === 'check'} onClick={() => void run('check')}>核对任务状态</Button></Space>
        <Typography.Paragraph className="settings-result" type="secondary">如执行时断线，请保持文件和来源不变，先核对任务状态，或用当前预览重试确认；已成功任务不会再次写入。</Typography.Paragraph>
      </>}
    </Card>}
  </div>
}
