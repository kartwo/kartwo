<!-- 配送设置 / Shipping Settings
功能：分离维护按洲归类的可配送范围、默认运费与特殊地区规则
作者：仗键天涯(daxing) ｜ 邮箱：3442535897@qq.com ｜ 时间：2026-09-11 10:17:20 -->
<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api.js'
import { useToast } from '../toast.js'

const toast = useToast()
const countries = ref([])
const enabled = ref([])
const zones = ref([])
const search = ref('')
const busy = ref(false)
const activeTab = ref('countries')
const continentOrder = ['Asia', 'Europe', 'North America', 'South America', 'Africa', 'Oceania', 'Antarctica', 'Other']
const continentZH = { Asia: '亚洲', Europe: '欧洲', 'North America': '北美洲', 'South America': '南美洲', Africa: '非洲', Oceania: '大洋洲', Antarctica: '南极洲', Other: '其他' }
const zhNames = typeof Intl !== 'undefined' && Intl.DisplayNames ? new Intl.DisplayNames(['zh-CN'], { type: 'region' }) : null

const defaultForm = ref({ mode: 'flat', rate: '5.00', freeOver: '200.00' })
const ruleForm = ref({ public_id: '', name: '', countries: [], mode: 'flat', rate: '5.00', freeOver: '100.00' })

const label = (country) => `${zhNames?.of(country.code) || country.name} / ${country.name} (${country.code})`
const filteredCountries = computed(() => {
  const needle = search.value.trim().toLowerCase()
  if (!needle) return countries.value
  return countries.value.filter((country) => `${label(country)} ${country.continent}`.toLowerCase().includes(needle))
})
const groupedCountries = computed(() => continentOrder.map((continent) => ({
  continent,
  items: filteredCountries.value.filter((country) => country.continent === continent),
})).filter((group) => group.items.length))
const enabledCountries = computed(() => countries.value.filter((country) => enabled.value.includes(country.code)))
const groupedEnabled = computed(() => continentOrder.map((continent) => ({
  continent,
  items: enabledCountries.value.filter((country) => country.continent === continent),
})).filter((group) => group.items.length))
const defaultZone = computed(() => zones.value.find((zone) => zone.is_default))
const specialZones = computed(() => zones.value.filter((zone) => !zone.is_default))

const centsToMoney = (cents) => (Number(cents || 0) / 100).toFixed(2)
const moneyToCents = (value) => {
  const text = String(value).trim()
  if (!/^\d+(\.\d{1,2})?$/.test(text)) throw new Error('金额最多保留两位小数')
  const [whole, fraction = ''] = text.split('.')
  return Number(whole) * 100 + Number((fraction + '00').slice(0, 2))
}
const rulePayload = (form, includeName = true) => {
  const payload = {
    name: includeName ? form.name.trim() : 'Default',
    countries: includeName ? form.countries.join(',') : '',
    rate_cents: form.mode === 'free' ? 0 : moneyToCents(form.rate),
    free_over_cents: form.mode === 'threshold' ? moneyToCents(form.freeOver) : 0,
  }
  if (form.mode === 'threshold' && payload.free_over_cents <= 0) throw new Error('包邮门槛必须大于 0')
  return payload
}
const applyZone = (form, zone) => {
  form.mode = zone.rate_cents === 0 && zone.free_over_cents === 0 ? 'free' : (zone.free_over_cents > 0 ? 'threshold' : 'flat')
  form.rate = centsToMoney(zone.rate_cents)
  form.freeOver = centsToMoney(zone.free_over_cents)
}
const load = async () => {
  try {
    const [countryData, zoneData] = await Promise.all([api.listShippingCountries(), api.listShippingZones()])
    countries.value = countryData.countries || []
    enabled.value = countryData.enabled || []
    zones.value = zoneData.zones || []
    if (defaultZone.value) applyZone(defaultForm.value, defaultZone.value)
  } catch (error) { toast.error(error.message) }
}
const toggleGroup = (items, checked) => {
  const codes = items.map((item) => item.code)
  enabled.value = checked ? [...new Set([...enabled.value, ...codes])] : enabled.value.filter((code) => !codes.includes(code))
}
const groupChecked = (items) => items.every((item) => enabled.value.includes(item.code))
const saveCountries = async () => {
  busy.value = true
  try { await api.saveShippingCountries(enabled.value); toast.success('可配送范围已保存') }
  catch (error) { toast.error(error.message) }
  finally { busy.value = false }
}
const saveDefault = async () => {
  busy.value = true
  try { await api.saveDefaultShippingZone(rulePayload(defaultForm.value, false)); await load(); toast.success('默认运费已保存') }
  catch (error) { toast.error(error.message) }
  finally { busy.value = false }
}
const resetRule = () => { ruleForm.value = { public_id: '', name: '', countries: [], mode: 'flat', rate: '5.00', freeOver: '100.00' } }
const editRule = (zone) => {
  ruleForm.value = { public_id: zone.public_id, name: zone.name, countries: zone.countries.split(',').filter(Boolean), mode: 'flat', rate: '', freeOver: '' }
  applyZone(ruleForm.value, zone)
  document.querySelector('#special-rule-form')?.scrollIntoView({ behavior: 'smooth' })
}
const saveRule = async () => {
  if (!ruleForm.value.name.trim() || !ruleForm.value.countries.length) { toast.error('请填写规则名称并至少选择一个国家'); return }
  busy.value = true
  try {
    const payload = rulePayload(ruleForm.value)
    if (ruleForm.value.public_id) await api.updateShippingZone(ruleForm.value.public_id, payload)
    else await api.createShippingZone(payload)
    resetRule(); await load(); toast.success('特殊地区规则已保存')
  } catch (error) { toast.error(error.message) }
  finally { busy.value = false }
}
const removeRule = async (zone) => {
  if (!window.confirm(`删除配送规则“${zone.name}”？这些国家将改用默认运费。`)) return
  try { await api.deleteShippingZone(zone.public_id); await load(); toast.success('配送规则已删除') }
  catch (error) { toast.error(error.message) }
}
const zoneNames = (raw) => raw.split(',').filter(Boolean).map((code) => zhNames?.of(code) || code).join('、')

onMounted(load)
</script>

<template>
  <h2>配送</h2>
  <p class="muted">先选择可以配送的国家/地区，再设置默认运费；少数地区价格不同时再添加特殊规则。</p>

  <nav class="shipping-tabs" role="tablist" aria-label="配送设置" data-demo-view-control>
    <button role="tab" :aria-selected="activeTab === 'countries'" :class="{ active: activeTab === 'countries' }" @click="activeTab = 'countries'">可配送国家/地区 <span>{{ enabled.length }}</span></button>
    <button role="tab" :aria-selected="activeTab === 'default'" :class="{ active: activeTab === 'default' }" @click="activeTab = 'default'">默认配送费</button>
    <button role="tab" :aria-selected="activeTab === 'special'" :class="{ active: activeTab === 'special' }" @click="activeTab = 'special'">特殊地区规则 <span>{{ specialZones.length }}</span></button>
  </nav>

  <section v-show="activeTab === 'countries'" class="panel shipping-card" role="tabpanel">
    <div class="section-head"><div><h3>可配送国家/地区</h3><p class="muted">只有这里勾选的国家才会显示在顾客结账页。</p></div><strong>已选择 {{ enabled.length }} 个</strong></div>
    <div class="country-actions"><input v-model="search" placeholder="搜索中文、英文或代码，例如 中国 / China / CN" /><button type="button" @click="enabled = countries.map(c => c.code)">全选</button><button type="button" @click="enabled = []">清空</button></div>
    <details v-for="group in groupedCountries" :key="group.continent" class="continent" :open="!!search">
      <summary><span class="summary-main"><span class="chevron">›</span><label @click.stop><input type="checkbox" :checked="groupChecked(group.items)" @change="toggleGroup(group.items, $event.target.checked)" /><span>{{ continentZH[group.continent] }} / {{ group.continent }}</span></label></span><span>{{ group.items.filter(c => enabled.includes(c.code)).length }} / {{ group.items.length }}</span></summary>
      <div class="country-grid"><label v-for="country in group.items" :key="country.code"><input v-model="enabled" type="checkbox" :value="country.code" /><span>{{ label(country) }}</span></label></div>
    </details>
    <button class="primary" :disabled="busy" @click="saveCountries">{{ busy ? '保存中…' : '保存配送范围' }}</button>
  </section>

  <section v-show="activeTab === 'default'" class="panel shipping-card" role="tabpanel">
    <h3>默认配送费</h3><p class="muted">适用于全部已启用、且没有特殊规则的国家/地区。</p>
    <div class="mode-list">
      <label><input v-model="defaultForm.mode" type="radio" value="free" /> 免费配送</label>
      <label><input v-model="defaultForm.mode" type="radio" value="flat" /> 固定运费</label>
      <label><input v-model="defaultForm.mode" type="radio" value="threshold" /> 固定运费，满额包邮</label>
    </div>
    <label v-if="defaultForm.mode !== 'free'">固定运费（US$）</label><input v-if="defaultForm.mode !== 'free'" v-model="defaultForm.rate" inputmode="decimal" />
    <label v-if="defaultForm.mode === 'threshold'">包邮门槛（US$）</label><input v-if="defaultForm.mode === 'threshold'" v-model="defaultForm.freeOver" inputmode="decimal" />
    <button class="primary" :disabled="busy" @click="saveDefault">保存默认运费</button>
  </section>

  <section v-show="activeTab === 'special'" class="panel shipping-card" role="tabpanel">
    <h3>特殊地区规则</h3><p class="muted">仅在某些国家运费不同的时候使用；每个国家只能属于一条特殊规则。</p>
    <div v-if="specialZones.length" class="rule-list">
      <article v-for="zone in specialZones" :key="zone.public_id"><div><strong>{{ zone.name }}</strong><p>{{ zoneNames(zone.countries) }}</p><p class="muted">运费 US${{ centsToMoney(zone.rate_cents) }}<span v-if="zone.free_over_cents"> · 满 US${{ centsToMoney(zone.free_over_cents) }} 包邮</span></p></div><div><button @click="editRule(zone)">编辑</button><button class="danger" @click="removeRule(zone)">删除</button></div></article>
    </div>
    <div id="special-rule-form" class="rule-editor">
      <h4>{{ ruleForm.public_id ? '编辑特殊规则' : '新增特殊规则' }}</h4>
      <label>规则名称</label><input v-model="ruleForm.name" placeholder="例如 North America" />
      <label>适用国家/地区</label>
      <p v-if="!enabledCountries.length" class="muted">请先在上方选择并保存可配送国家。</p>
      <details v-for="group in groupedEnabled" :key="group.continent" class="continent">
        <summary><span class="summary-main"><span class="chevron">›</span><span>{{ continentZH[group.continent] }} / {{ group.continent }}</span></span><span>{{ group.items.filter(c => ruleForm.countries.includes(c.code)).length }} / {{ group.items.length }}</span></summary>
        <div class="country-grid"><label v-for="country in group.items" :key="country.code"><input v-model="ruleForm.countries" type="checkbox" :value="country.code" /><span>{{ label(country) }}</span></label></div>
      </details>
      <div class="mode-list"><label><input v-model="ruleForm.mode" type="radio" value="free" /> 免费配送</label><label><input v-model="ruleForm.mode" type="radio" value="flat" /> 固定运费</label><label><input v-model="ruleForm.mode" type="radio" value="threshold" /> 固定运费，满额包邮</label></div>
      <label v-if="ruleForm.mode !== 'free'">固定运费（US$）</label><input v-if="ruleForm.mode !== 'free'" v-model="ruleForm.rate" inputmode="decimal" />
      <label v-if="ruleForm.mode === 'threshold'">包邮门槛（US$）</label><input v-if="ruleForm.mode === 'threshold'" v-model="ruleForm.freeOver" inputmode="decimal" />
      <div class="editor-actions"><button class="primary" :disabled="busy" @click="saveRule">{{ ruleForm.public_id ? '保存修改' : '新增特殊规则' }}</button><button v-if="ruleForm.public_id" @click="resetRule">取消编辑</button></div>
    </div>
  </section>
</template>

<style scoped>
.shipping-card{max-width:980px;margin-bottom:var(--sp-5)}
.shipping-tabs{display:flex;gap:var(--sp-2);max-width:980px;margin:var(--sp-4) 0 0;border-bottom:1px solid var(--border)}.shipping-tabs button{border:0;border-bottom:3px solid transparent;border-radius:var(--radius-md) var(--radius-md) 0 0;box-shadow:none;padding:.75rem 1rem;color:var(--text-muted)}.shipping-tabs button.active{border-bottom-color:var(--accent);color:var(--accent);background:var(--surface);font-weight:700}.shipping-tabs button span{display:inline-block;min-width:1.5rem;margin-left:.3rem;padding:.05rem .4rem;border-radius:var(--radius-pill);background:var(--surface-2);font-size:var(--fs-xs)}
.section-head,.country-actions,.rule-list article,.editor-actions{display:flex;align-items:center;justify-content:space-between;gap:var(--sp-3)}
.section-head h3,.section-head p,.rule-list p{margin:.25rem 0}.country-actions{margin:var(--sp-3) 0}.country-actions input{flex:1}.country-actions button,.rule-list button,.editor-actions button{width:auto}
.continent{border:1px solid var(--line);border-radius:10px;margin:.65rem 0;background:white}.continent summary{display:flex;align-items:center;justify-content:space-between;gap:1rem;list-style:none;padding:.8rem 1rem;cursor:pointer;font-weight:700;text-align:left}.continent summary::-webkit-details-marker{display:none}.summary-main{display:flex;align-items:center;justify-content:flex-start;gap:.65rem;min-width:0;text-align:left}.summary-main label{display:flex;align-items:center;justify-content:flex-start;gap:.65rem;margin:0;text-align:left}.summary-main input{width:auto;min-width:1rem;margin:0}.chevron{width:1rem;text-align:center;transform:rotate(0);transition:transform .12s ease;color:var(--text-muted)}.continent[open] .chevron{transform:rotate(90deg)}.country-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(260px,1fr));gap:.3rem 1rem;padding:.4rem 1rem 1rem}.country-grid label,.mode-list label{display:flex;align-items:flex-start;gap:.55rem;font-weight:400;margin:0}.country-grid input,.mode-list input{width:auto;margin-top:.25rem}
.shipping-card>.primary{margin-top:var(--sp-3)}.mode-list{display:flex;flex-wrap:wrap;gap:1.2rem;margin:1rem 0}.shipping-card>label,.rule-editor>label{display:block;margin-top:var(--sp-3)}.shipping-card>input,.rule-editor>input{width:100%}
.rule-list article{border-bottom:1px solid var(--line);padding:1rem 0}.rule-list article>div:last-child{display:flex;gap:.5rem}.danger{color:#b42318}.rule-editor{margin-top:1rem;padding-top:1rem;border-top:1px solid var(--line)}.editor-actions{justify-content:flex-start;margin-top:1rem}
@media(max-width:640px){.shipping-tabs{overflow-x:auto}.shipping-tabs button{flex:0 0 auto}.section-head,.country-actions{align-items:stretch;flex-direction:column}.country-actions button{width:100%}.country-grid{grid-template-columns:1fr}.rule-list article{align-items:flex-start;flex-direction:column}}
</style>
