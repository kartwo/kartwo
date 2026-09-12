<!-- 店铺政策资料 / Store Policy Profile. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<script setup>
import { ref, onMounted, inject } from 'vue'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'
import { confirm } from '../confirm.js'

const toast = useToast(); const onUnauthorized = inject('onUnauthorized')
const busy = ref(false); const configured = ref(false); const publish = ref(true)
const regionZH = { Asia:'亚洲', Europe:'欧洲', 'North America':'北美洲', 'South America':'南美洲', Africa:'非洲', Oceania:'大洋洲', Antarctica:'南极洲' }
const form = ref({ support_email:'', ship_from:'', processing_hours:48, return_window_days:7, return_shipping_payer:'buyer', business_name:'', delivery_estimates:[] })

async function load() {
  try { const r=await api.getPolicyProfile(); configured.value=!!r.configured; form.value=r.profile }
  catch(e) { if(e instanceof APIError&&e.status===401)onUnauthorized(); else toast.error(e.message) }
}
async function save(showToast=true) {
  busy.value=true
  try { await api.setPolicyProfile(form.value); configured.value=true; if(showToast)toast.success('店铺政策资料已保存'); return true }
  catch(e) { toast.error(e.message); return false }
  finally { busy.value=false }
}
async function generate(overwrite=false) {
  if(!await save(false))return
  if(overwrite && !await confirm({title:'覆盖标准页面正文',message:'将按当前资料重新生成 7 个标准页面的正文。页面原有的草稿/发布状态会保留，但手工修改的正文会被覆盖。确定继续？',confirmText:'重新生成',danger:true}))return
  busy.value=true
  try { const r=await api.generateFooterPages({publish:publish.value,overwrite}); toast.success(`完成：新建 ${r.created}，更新 ${r.updated}，跳过 ${r.skipped}`) }
  catch(e) { toast.error(e.message) }
  finally { busy.value=false }
}
onMounted(load)
</script>

<template>
  <h2>店铺政策</h2>
  <p class="muted">集中维护经营资料，并生成店面页脚所需的英文内容页。生成文案是可编辑的经营草稿，不构成法律意见；正式发布前请按销售市场复核。</p>
  <form class="panel policy-form" @submit.prevent="save()">
    <h3>基础资料</h3>
    <div class="form-grid">
      <div><label>客服邮箱</label><input v-model.trim="form.support_email" type="email" required placeholder="support@example.com" /></div>
      <div><label>发货国家/地区</label><input v-model.trim="form.ship_from" required placeholder="China" /></div>
      <div><label>订单处理时间（小时内）</label><input v-model.number="form.processing_hours" type="number" min="1" max="720" required /></div>
      <div><label>收货后退货期限（天）</label><input v-model.number="form.return_window_days" type="number" min="1" max="365" required /></div>
      <div><label>退货运费承担方</label><select v-model="form.return_shipping_payer"><option value="buyer">买家</option><option value="merchant">商家</option></select></div>
      <div><label>经营主体名称</label><input v-model.trim="form.business_name" required placeholder="Example Store" /></div>
    </div>

    <h3>预计配送时效</h3>
    <p class="muted">填写发货后的工作日区间。清关、节假日、偏远地区和承运商延误不计入承诺。</p>
    <div class="estimate-list">
      <div v-for="estimate in form.delivery_estimates" :key="estimate.region" class="estimate-row">
        <strong>{{regionZH[estimate.region]}} / {{estimate.region}}</strong>
        <label>最少 <input v-model.number="estimate.min_business_days" type="number" min="1" max="120" required /> 天</label>
        <label>最多 <input v-model.number="estimate.max_business_days" type="number" min="1" max="120" required /> 天</label>
      </div>
    </div>
    <div class="actions"><button class="primary" :disabled="busy">保存资料</button></div>
  </form>

  <section class="panel generator">
    <h3>生成页脚内容</h3>
    <p>将生成 About Us、Contact Us、Shipping Policy、Returns & Refunds、Privacy Policy、Terms of Service 和 FAQ 共 7 页。</p>
    <label class="publish"><input v-model="publish" type="checkbox" /> 新建页面后直接发布到店面页脚</label>
    <p class="muted">“生成缺少页面”不会改动同 slug 的现有页面；“重新生成”才会覆盖标准页面正文，并保留现有发布状态。</p>
    <div class="actions">
      <button class="primary" :disabled="busy||!configured" @click="generate(false)">生成缺少页面</button>
      <button :disabled="busy||!configured" @click="generate(true)">重新生成并覆盖正文</button>
    </div>
  </section>
</template>

<style scoped>
.policy-form,.generator{margin-top:var(--sp-4)}.form-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:var(--sp-3)}.estimate-list{display:grid;gap:var(--sp-2)}.estimate-row{display:grid;grid-template-columns:minmax(12rem,1fr) auto auto;align-items:center;gap:var(--sp-3);padding:var(--sp-3);border:1px solid var(--border);border-radius:var(--radius-md)}.estimate-row label{display:flex;align-items:center;gap:var(--sp-2);margin:0}.estimate-row input{width:5.5rem}.actions{display:flex;gap:var(--sp-2);flex-wrap:wrap;margin-top:var(--sp-4)}.publish{display:flex;align-items:center;gap:var(--sp-2)}.publish input{width:auto}@media(max-width:720px){.form-grid{grid-template-columns:1fr}.estimate-row{grid-template-columns:1fr}.estimate-row input{width:100%}}
</style>
