// 配送设置 / Shipping Settings
// 功能：国家分组与搜索、默认及特殊运费规则，保留独立权限与演示只读
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 11:01:04

import { CheckSquareOutlined, ClearOutlined, CloseOutlined, DeleteOutlined, EditOutlined, SaveOutlined } from '@ant-design/icons'
import { useEffect, useState } from 'react'
import { Alert, App, Button, Card, Checkbox, Empty, Form, Input, Modal, Radio, Select, Space, Tabs, Typography } from 'antd'
import type { FormInstance } from 'antd'
import { shippingAPI, continents, continentNames, rateFields, type Country, type CountryStatus, type RateFields, type RuleFields, type ShippingZone } from '../shipping-api'
import { centsToMoney, moneyToCents } from '../money'
import { useSession } from '../SessionProvider'
import { can } from '../access'
import { useResource, useTask } from '../useResource'
import { PageState } from '../PageState'
const regionNames = typeof Intl.DisplayNames === 'function' ? new Intl.DisplayNames(['zh-CN'], { type: 'region' }) : null
function label(country: Country) { return `${regionNames?.of(country.code) || country.name} / ${country.name} (${country.code})` }

function CountrySelection({ data, writable, busy, save }: { data: CountryStatus; writable: boolean; busy: boolean; save: (codes: string[]) => void }) {
  const [enabled, setEnabled] = useState(data.enabled), [search, setSearch] = useState('')
  useEffect(() => setEnabled(data.enabled), [data.enabled])
  const visible = data.countries.filter(country => `${label(country)} ${country.continent}`.toLowerCase().includes(search.trim().toLowerCase()))
  function toggle(codes: string[], checked: boolean) { setEnabled(current => checked ? [...new Set([...current, ...codes])] : current.filter(code => !codes.includes(code))) }
  return <Card title={`可配送国家/地区（已选择 ${enabled.length} 个）`}>
    <Typography.Paragraph type="secondary">只有这里勾选并保存的国家才会显示在顾客结账页。</Typography.Paragraph>
    <Space wrap className="shipping-actions">
      <Input aria-label="搜索配送国家" placeholder="搜索中文、英文或国家代码" value={search} onChange={event => setSearch(event.target.value)} allowClear />
      <Button icon={<CheckSquareOutlined aria-hidden />} disabled={!writable || busy} onClick={() => setEnabled(data.countries.map(country => country.code))}>全选</Button>
      <Button icon={<ClearOutlined aria-hidden />} disabled={!writable || busy} onClick={() => setEnabled([])}>清空</Button>
    </Space>
    {!visible.length && <Empty description="没有匹配的国家或地区" />}
    {continents.map(continent => {
      const items = visible.filter(country => country.continent === continent)
      if (!items.length) return null
      const count = items.filter(country => enabled.includes(country.code)).length
      return <div className="shipping-continent" key={continent}>
        <Checkbox disabled={!writable || busy} checked={count === items.length} indeterminate={count > 0 && count < items.length} onChange={event => toggle(items.map(country => country.code), event.target.checked)}>{continentNames[continent]} / {continent}（{count} / {items.length}）</Checkbox>
        <div className="shipping-country-grid">{items.map(country => <Checkbox key={country.code} disabled={!writable || busy} checked={enabled.includes(country.code)} onChange={event => toggle([country.code], event.target.checked)}>{label(country)}</Checkbox>)}</div>
      </div>
    })}
    {writable && <Button icon={<SaveOutlined aria-hidden />} type="primary" disabled={busy} onClick={() => save(enabled)}>保存配送范围</Button>}
  </Card>
}
function RateInputs({ form, prefix }: { form: FormInstance; prefix: string }) {
  const mode = Form.useWatch('mode', form)
  const moneyRule = { validator: (_: unknown, value: string) => { try { moneyToCents(value ?? ''); return Promise.resolve() } catch (error) { return Promise.reject(error) } } }
  return <>
    <Form.Item name="mode" label={`${prefix}收费方式`}><Radio.Group options={[{ value: 'free', label: '免费配送' }, { value: 'flat', label: '固定运费' }, { value: 'threshold', label: '固定运费，满额包邮' }]} /></Form.Item>
    {mode !== 'free' && <Form.Item name="rate" label={`${prefix}固定运费（US$）`} rules={[moneyRule]}><Input inputMode="decimal" /></Form.Item>}
    {mode === 'threshold' && <Form.Item name="freeOver" label={`${prefix}包邮门槛（US$）`} rules={[{ validator: (_, value: string) => {
      try { if (moneyToCents(value ?? '') > 0) return Promise.resolve() } catch (error) { return Promise.reject(error) }
      return Promise.reject(new Error('包邮门槛必须大于 0'))
    } }]}><Input inputMode="decimal" /></Form.Item>}
  </>
}
function DefaultRate({ zone, writable, busy, save }: { zone?: ShippingZone; writable: boolean; busy: boolean; save: (values: RateFields) => void }) {
  const [form] = Form.useForm<RateFields>()
  useEffect(() => form.setFieldsValue(rateFields(zone)), [zone, form])
  return <Card title="默认配送费"><Typography.Paragraph type="secondary">适用于全部已启用、且没有特殊规则的国家/地区。</Typography.Paragraph>
    <Form name="shipping-default" form={form} layout="vertical" disabled={!writable || busy} onFinish={values => { if (writable) save(values) }}>
      <RateInputs form={form} prefix="默认" />
      {writable && <Button icon={<SaveOutlined aria-hidden />} type="primary" htmlType="submit" loading={busy}>保存默认运费</Button>}
    </Form>
  </Card>
}
function SpecialRules({ zones, countries, writable, busy, save, remove }: { zones: ShippingZone[]; countries: CountryStatus; writable: boolean; busy: boolean; save: (values: RuleFields, id: string, done: () => void) => void; remove: (id: string) => void }) {
  const [form] = Form.useForm<RuleFields>()
  const [editing, setEditing] = useState(''), [deleting, setDeleting] = useState<ShippingZone | null>(null)
  const selected = Form.useWatch('countries', form) as string[] | undefined
  function reset() { setEditing(''); form.resetFields() }
  const special = zones.filter(zone => !zone.is_default)
  useEffect(() => { if (editing && !zones.some(zone => zone.public_id === editing)) { setEditing(''); form.resetFields() } }, [editing, zones, form])
  const options = countries.countries.filter(country => countries.enabled.includes(country.code) || selected?.includes(country.code)).map(country => ({ value: country.code, label: label(country) + (countries.enabled.includes(country.code) ? '' : '（未启用，请移除）') }))
  return <Card title="特殊地区规则">
    <Typography.Paragraph type="secondary">每个国家只能属于一条特殊规则；适用国家需先在配送范围中保存启用。</Typography.Paragraph>
    {!special.length && <Empty description="尚无特殊地区规则" />}
    {special.map(zone => <div key={zone.public_id} className="shipping-rule-row"><div><strong>{zone.name}</strong><Typography.Paragraph>{zone.countries.split(',').map(code => regionNames?.of(code) || code).join('、')}</Typography.Paragraph><Typography.Text type="secondary">运费 US${centsToMoney(zone.rate_cents)}{zone.free_over_cents > 0 && ` · 满 US$${centsToMoney(zone.free_over_cents)} 包邮`}{!zone.active && ' · 未启用'}</Typography.Text></div><Space>
      <Button icon={<EditOutlined aria-hidden />} disabled={busy} onClick={() => { setEditing(zone.public_id); form.setFieldsValue({ name: zone.name, countries: zone.countries.split(','), ...rateFields(zone) }) }}>编辑</Button>
      {writable && <Button icon={<DeleteOutlined aria-hidden />} danger disabled={busy} onClick={() => setDeleting(zone)}>删除</Button>}
    </Space></div>)}
    <Form name="shipping-rule" form={form} layout="vertical" disabled={!writable || busy} initialValues={{ name: '', countries: [], mode: 'flat', rate: '5.00', freeOver: '100.00' }} onFinish={values => { if (writable) save(values, editing, reset) }}>
      <Typography.Title level={3}>{editing ? '编辑特殊规则' : '新增特殊规则'}</Typography.Title>
      <Form.Item name="name" label="规则名称" rules={[{ required: true, whitespace: true, message: '请填写规则名称' }]}><Input /></Form.Item>
      <Form.Item name="countries" label="适用国家/地区" rules={[{ required: true, type: 'array', min: 1, message: '请至少选择一个已启用国家' }, { validator: (_, values: string[] = []) => values.every(code => countries.enabled.includes(code)) ? Promise.resolve() : Promise.reject(new Error('请先保存配送范围，或移除未启用的国家')) }]}><Select mode="multiple" options={options} optionFilterProp="label" /></Form.Item>
      <RateInputs form={form} prefix="特殊规则" />
      <Space wrap>{writable && <Button icon={<SaveOutlined aria-hidden />} type="primary" htmlType="submit" loading={busy}>{editing ? '保存修改' : '新增特殊规则'}</Button>}{editing && <Button icon={<CloseOutlined aria-hidden />} disabled={busy} onClick={reset}>取消编辑</Button>}</Space>
    </Form>
    <Modal open={!!deleting} title="删除配送规则" okText="删除" cancelText="取消" okButtonProps={{ danger: true, disabled: busy }} onCancel={() => setDeleting(null)} onOk={() => { if (writable && deleting) { remove(deleting.public_id); setDeleting(null) } }}>删除“{deleting?.name}”后，这些国家将改用默认运费。</Modal>
  </Card>
}
export default function ShippingSettings() {
  const { state } = useSession(), resource = useResource(shippingAPI.load), task = useTask(), { message } = App.useApp()
  const data = resource.data, writable = can(state.session, 'settings.shipping')
  async function saveCountries(codes: string[]) {
    if (!writable) return
    await task.run(() => shippingAPI.saveCountries(codes), countries => { resource.setData(current => current && { ...current, countries }); void message.success('可配送范围已保存') })
  }
  async function saveDefault(values: RateFields) {
    if (!writable) return
    await task.run(() => shippingAPI.saveDefault(values), zones => { resource.setData(current => current && { ...current, zones }); void message.success('默认运费已保存') })
  }
  async function saveRule(values: RuleFields, id: string, done: () => void) {
    if (!writable) return
    await task.run(() => shippingAPI.saveRule(values, id), zones => { resource.setData(current => current && { ...current, zones }); done(); void message.success('特殊地区规则已保存') })
  }
  async function remove(id: string) {
    if (!writable) return
    await task.run(() => shippingAPI.removeRule(id), zones => { resource.setData(current => current && { ...current, zones }); void message.success('配送规则已删除') })
  }
  return <div className="shipping-settings"><PageState {...resource} retry={resource.refresh}>
    {data && <>
      {!writable && <Alert type="info" title="当前账号只允许查看配送设置。搜索、标签页和规则查看仍可使用。" />}
      {task.error && <Alert type="error" showIcon title={task.error} />}
      <Tabs aria-label="配送设置" items={[
        { key: 'countries', label: `可配送国家/地区 ${data.countries.enabled.length}`, children: <CountrySelection data={data.countries} writable={writable} busy={task.busy} save={codes => void saveCountries(codes)} /> },
        { key: 'default', label: '默认配送费', children: <DefaultRate zone={data.zones.find(zone => zone.is_default)} writable={writable} busy={task.busy} save={values => void saveDefault(values)} /> },
        { key: 'special', label: `特殊地区规则 ${data.zones.filter(zone => !zone.is_default).length}`, children: <SpecialRules zones={data.zones} countries={data.countries} writable={writable} busy={task.busy} save={(values, id, done) => void saveRule(values, id, done)} remove={id => void remove(id)} /> },
      ]} />
    </>}
  </PageState></div>
}
