// 用户与角色 / Users and Roles
// 功能：店主管理员工邀请、角色权限及会话，一次性链接仅在当前页面保留
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:09:08

import { useEffect, useState } from 'react'
import { Alert, App, Button, Card, Checkbox, Form, Input, Modal, Select, Space, Switch, Tabs, Tag, Typography } from 'antd'
import { permissionGroups, permissionError, reloadAfter, usersAPI, type InviteFields, type ManagedRole, type ManagedUser, type RoleFields, type UserFields } from '../users-api'
import { PageState } from '../PageState'
import { useResource, useTask } from '../useResource'
import { useSession } from '../SessionProvider'

function PermissionPicker({ value = [], onChange, disabled, id }: { value?: string[]; onChange?: (value: string[]) => void; disabled: boolean; id?: string }) {
  return <div id={id} className="role-permissions">{permissionGroups.map(([title, items]) => {
    const keys = new Set<string>(items.map(([key]) => key))
    return <fieldset key={title} disabled={disabled}><legend>{title}</legend><Checkbox.Group disabled={disabled} value={value.filter(key => keys.has(key))} options={items.map(([value, label]) => ({ value, label }))} onChange={selected => onChange?.([...value.filter(key => !keys.has(key)), ...selected.map(String)])} /></fieldset>
  })}</div>
}
const permissionRules = [{ validator: (_: unknown, value: string[] = []) => { const error = permissionError(value); return error ? Promise.reject(new Error(error)) : Promise.resolve() } }]
const nameRules = [{ required: true, whitespace: true, message: '请输入角色名称' }, { validator: (_: unknown, value: string) => Array.from((value ?? '').trim()).length <= 60 ? Promise.resolve() : Promise.reject(new Error('角色名称最多 60 个字符')) }]
function RoleEditor({ role, busy, save, remove }: { role?: ManagedRole; busy: boolean; save: (values: RoleFields, reset: () => void) => void; remove?: () => void }) {
  const [form] = Form.useForm<RoleFields>()
  const id = role?.public_id, name = role?.name, selected = role?.permissions.join(',')
  useEffect(() => { form.setFieldsValue({ name: name ?? '', permissions: selected?.split(',').filter(Boolean) ?? [] }) }, [id, name, selected, form])
  return <Card title={role ? role.name : '新建自定义角色'} extra={role && <span>{role.user_count} 位员工 {role.system_key && <Tag>预设角色</Tag>}</span>}>
    {role?.system_key && <Typography.Paragraph type="secondary">预设角色名称固定，权限可以调整。</Typography.Paragraph>}
    <Form name={`role-${id ?? 'new'}`} form={form} layout="vertical" disabled={busy} initialValues={{ name: '', permissions: [] }} onFinish={values => save(values, () => form.resetFields())}>
      <Form.Item name="name" label="角色名称" rules={nameRules}><Input disabled={busy || !!role?.system_key} /></Form.Item>
      <Form.Item name="permissions" label="可分配权限" rules={permissionRules}><PermissionPicker disabled={busy} /></Form.Item>
      <Space wrap><Button type="primary" htmlType="submit" loading={busy}>{role ? '保存权限' : '创建角色'}</Button>{role && !role.system_key && <Button danger disabled={busy || role.user_count > 0} onClick={remove}>删除角色</Button>}</Space>
      {role && !role.system_key && role.user_count > 0 && <Typography.Paragraph type="secondary">删除前请先将使用该角色的员工调整到其他角色。</Typography.Paragraph>}
    </Form>
  </Card>
}
function UserEditor({ user, roles, busy, save, reset, revoke }: { user: ManagedUser; roles: ManagedRole[]; busy: boolean; save: (values: UserFields) => void; reset: () => void; revoke: () => void }) {
  const [form] = Form.useForm<UserFields>()
  useEffect(() => { if (user.role_id) form.setFieldsValue({ display_name: user.display_name, role_id: user.role_id, disabled: user.status === 'disabled' }) }, [user.display_name, user.role_id, user.status, form])
  return <Card title={user.username} extra={<Tag>{user.status === 'invited' ? '待设置密码' : user.status === 'disabled' ? '已停用' : '正常'}</Tag>}>
    <Typography.Paragraph type="secondary">最近登录：{user.last_login_at || '从未登录'}</Typography.Paragraph>
    {!user.role_id ? <Typography.Paragraph>店主（超管）· 全部权限固定 · 请在账号与安全页修改自身凭据。</Typography.Paragraph> : <Form name={`user-${user.public_id}`} form={form} layout="vertical" disabled={busy} onFinish={save}>
      <div className="settings-columns"><Form.Item name="display_name" label="显示名称" rules={[{ validator: (_, value: string) => Array.from(value ?? '').length <= 100 ? Promise.resolve() : Promise.reject(new Error('显示名称最多 100 个字符')) }]}><Input /></Form.Item>
      <Form.Item name="role_id" label="员工角色" rules={[{ required: true, message: '请选择角色' }]}><Select options={roles.map(role => ({ value: role.public_id, label: role.name }))} /></Form.Item></div>
      <Form.Item name="disabled" label="停用员工" valuePropName="checked"><Switch /></Form.Item>
      <Space wrap><Button type="primary" htmlType="submit" loading={busy}>保存员工</Button><Button disabled={busy || user.status === 'disabled'} onClick={reset}>重置密码</Button><Button disabled={busy} onClick={revoke}>退出设备</Button></Space>
    </Form>}
  </Card>
}
type Confirmation = { kind: 'reset'; user: ManagedUser } | { kind: 'revoke'; user: ManagedUser } | { kind: 'delete'; role: ManagedRole }
export default function UsersRoles() {
  const { state } = useSession(), resource = useResource(usersAPI.load), task = useTask(), { message } = App.useApp()
  const [inviteForm] = Form.useForm<InviteFields>()
  const [invitation, setInvitation] = useState<{ url: string; username: string } | null>(null)
  const [warning, setWarning] = useState(''), [confirmation, setConfirmation] = useState<Confirmation | null>(null)
  const data = resource.data, owner = state.session?.role === 'owner'
  const defaultRole = data?.roles.find(role => role.system_key === 'viewer')?.public_id || data?.roles[0]?.public_id
  const roleIDs = data?.roles.map(role => role.public_id).join(',')
  useEffect(() => { if (roleIDs !== undefined && !roleIDs.split(',').includes(inviteForm.getFieldValue('role_id') as string)) inviteForm.setFieldValue('role_id', defaultRole ?? '') }, [defaultRole, roleIDs, inviteForm])
  async function mutate<T>(operation: () => Promise<T>, success: (value: T) => void) {
    if (!owner) return
    await task.run(async () => { setWarning(''); return reloadAfter(operation) }, result => {
      if (result.snapshot) resource.setData(result.snapshot)
      setWarning(result.warning); success(result.value)
    })
  }
  async function createUser(values: InviteFields) {
    await mutate(async () => { setInvitation(null); return usersAPI.createUser(values) }, url => {
      setInvitation({ url, username: values.username.trim() }); inviteForm.setFieldsValue({ username: '', display_name: '' })
      void message.success('员工账号已创建；请安全地把一次性链接发给本人')
    })
  }
  async function saveUser(user: ManagedUser, values: UserFields) {
    if (!user.role_id) return
    await mutate(() => usersAPI.saveUser(user.public_id, values), () => { void message.success('账号已更新，旧会话已撤销') })
  }
  async function saveRole(values: RoleFields, role: ManagedRole | undefined, reset: () => void) {
    await mutate(() => role ? usersAPI.saveRole(role.public_id, values) : usersAPI.createRole(values), () => {
      if (!role) reset()
      void message.success(role ? '角色已更新；相关员工需重新登录' : '角色已创建')
    })
  }
  async function confirm() {
    if (!confirmation || !owner) return
    const current = confirmation
    if (current.kind === 'reset') await mutate(async () => { setInvitation(null); return usersAPI.resetPassword(current.user.public_id) }, url => {
      setInvitation({ url, username: current.user.username }); setConfirmation(null); void message.success('新链接已生成，原链接及旧会话已失效')
    })
    else if (current.kind === 'revoke') await mutate(() => usersAPI.revokeSessions(current.user.public_id), count => { setConfirmation(null); void message.success(`已撤销 ${count} 个会话`) })
    else if (!current.role.system_key && !current.role.user_count) await mutate(() => usersAPI.deleteRole(current.role.public_id), () => { setConfirmation(null); void message.success('角色已删除') })
  }
  async function copy() {
    if (!invitation) return
    try { await navigator.clipboard.writeText(invitation.url); void message.success('邀请链接已复制') }
    catch { void message.error('复制失败，请手动选择并复制链接') }
  }
  const titles = { reset: '重置员工密码', revoke: '撤销员工会话', delete: '删除角色' }
  async function refresh() { if (owner) await task.run(usersAPI.load, snapshot => { resource.setData(snapshot); setWarning('') }) }
  return <div className="users-roles-page">
    <Typography.Paragraph type="secondary">店主（超管）权限固定且不可删除。员工密码与店铺加密主口令独立；预设角色和自定义角色均可调整权限。邀请链接仅在本页显示，请通过可信渠道发给本人。</Typography.Paragraph>
    {invitation && <Card title={`${invitation.username} · 一次性链接（24 小时有效）`}><Input value={invitation.url} readOnly aria-label="邀请链接" autoComplete="off" /><Space wrap className="settings-result"><Button onClick={() => void copy()}>复制链接</Button><Button onClick={() => setInvitation(null)}>隐藏链接</Button></Space></Card>}
    {task.error && !confirmation && <Alert type="error" showIcon title={task.error} />}
    {warning && <Alert type="warning" showIcon title={warning} action={<Button disabled={task.busy} onClick={() => void refresh()}>刷新列表</Button>} />}
    <PageState {...resource} retry={resource.refresh}>{data && <Tabs items={[
      { key: 'users', label: '用户', children: <div className="users-roles-stack"><Card title="邀请员工"><Form name="invite-user" form={inviteForm} layout="vertical" disabled={!owner || task.busy} onFinish={createUser}>
        <Form.Item name="username" label="登录用户名" extra="3–64 位英文、数字、.-_；保存时转为小写。" rules={[{ required: true, message: '请输入用户名' }, { transform: (value: string) => value.trim(), pattern: /^[a-zA-Z0-9._-]{3,64}$/, message: '请输入 3–64 位英文、数字、.-_' }]}><Input autoComplete="off" /></Form.Item>
        <Form.Item name="display_name" label="显示名称" initialValue="" rules={[{ validator: (_, value: string) => Array.from(value ?? '').length <= 100 ? Promise.resolve() : Promise.reject(new Error('显示名称最多 100 个字符')) }]}><Input autoComplete="off" /></Form.Item>
        <Form.Item name="role_id" label="邀请角色" rules={[{ required: true, message: '请选择角色' }]}><Select options={data.roles.map(role => ({ value: role.public_id, label: role.name }))} /></Form.Item>
        <Button type="primary" htmlType="submit" loading={task.busy}>生成邀请链接</Button>
      </Form></Card>{data.users.map(user => <UserEditor key={user.public_id} user={user} roles={data.roles} busy={task.busy || !owner} save={values => void saveUser(user, values)} reset={() => setConfirmation({ kind: 'reset', user })} revoke={() => setConfirmation({ kind: 'revoke', user })} />)}</div> },
      { key: 'roles', label: '角色与权限', children: <div className="users-roles-stack"><RoleEditor busy={task.busy || !owner} save={(values, reset) => void saveRole(values, undefined, reset)} />{data.roles.map(role => <RoleEditor key={role.public_id} role={role} busy={task.busy || !owner} save={(values, reset) => void saveRole(values, role, reset)} remove={() => setConfirmation({ kind: 'delete', role })} />)}</div> },
    ]} />}</PageState>
    <Modal open={!!confirmation} title={confirmation && titles[confirmation.kind]} okText={confirmation?.kind === 'reset' ? '生成链接' : confirmation?.kind === 'revoke' ? '撤销' : '删除'} cancelText="取消" confirmLoading={task.busy} okButtonProps={{ danger: true }} onCancel={() => { if (!task.busy) setConfirmation(null) }} onOk={() => void confirm()}>
      {task.error && <Alert type="error" showIcon title={task.error} />}
      {confirmation?.kind === 'reset' ? `为 ${confirmation.user.username} 生成新的 24 小时一次性链接？原链接和该员工旧会话会失效。` : confirmation?.kind === 'revoke' ? `让 ${confirmation.user.username} 的所有登录设备立即退出？` : confirmation && `删除「${confirmation.role.name}」？使用此角色的账号需先调整。`}
    </Modal>
  </div>
}
