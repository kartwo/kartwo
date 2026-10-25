// 组件化文件选择 / Styled File Picker
// 功能：用 Ant Design 按钮与官方图标打开原生文件选择器，保留上传事件、权限禁用和相同文件重选
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 14:52:16

import { useRef, useState, type ChangeEventHandler } from 'react'
import { Button, Space, Typography } from 'antd'
import { UploadOutlined } from '@ant-design/icons'
interface Props { id?: string; label: string; buttonLabel?: string; accept: string; disabled: boolean; onChange: ChangeEventHandler<HTMLInputElement> }
export function FilePicker({ id, label, buttonLabel = label, accept, disabled, onChange }: Props) {
  const input = useRef<HTMLInputElement>(null), [name, setName] = useState('')
  return <Space wrap className="file-picker"><Button disabled={disabled} icon={<UploadOutlined aria-hidden />} onClick={() => input.current?.click()}>{buttonLabel}</Button>
    {name && <Typography.Text type="secondary" className="file-picker-name">{name}</Typography.Text>}
    <input ref={input} id={id} type="file" hidden aria-label={label} accept={accept} disabled={disabled} onChange={event => {
      setName(event.currentTarget.files?.[0]?.name ?? '')
      try { onChange(event) } finally { event.currentTarget.value = '' }
    }} />
  </Space>
}
