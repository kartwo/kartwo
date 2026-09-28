<!-- 客服留言收件箱 / Contact Inbox
功能：查看私密留言、IP 概略所属地与处理状态；公开演示不读取真实留言
作者：仗键天涯(daxing)
邮箱：3442535897@qq.com
时间：2026-09-28 15:00:00 -->
<script setup>
import { ref, onMounted, inject } from 'vue'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'

const toast = useToast()
const onUnauthorized = inject('onUnauthorized')
const role = inject('accountRole')
const permissions = inject('accountPermissions')
const status = ref('')
const messages = ref([])
const selected = ref(null)
const demoHidden = ref(false)
const busy = ref(false)
const topics = { product: '商品', shipping: '配送', order: '订单', returns: '退货', other: '其他' }
const statuses = { new: '新留言', processing: '处理中', closed: '已结案' }
const canManage = () => role.value === 'owner' || (role.value === 'staff' && permissions.value.includes('contact.manage'))
function countryName(code) {
  if (!code) return '未知（未获得可信定位）'
  try { return new Intl.DisplayNames(['zh-CN'], { type: 'region' }).of(code) || code }
  catch (_) { return code }
}
function fail(e) { if (e instanceof APIError && e.status === 401) onUnauthorized(); else toast.error(e.message) }
async function load() {
  busy.value = true
  try {
    const result = await api.contactMessages(status.value)
    messages.value = result.messages || []
    demoHidden.value = !!result.demo_hidden
    selected.value = null
  } catch (e) { fail(e) }
  finally { busy.value = false }
}
async function open(message) {
  try { selected.value = await api.contactMessage(message.public_id) }
  catch (e) { fail(e) }
}
async function setStatus(next) {
  if (!selected.value || !canManage()) return
  busy.value = true
  try {
    selected.value = await api.updateContactStatus(selected.value.public_id, next)
    const i = messages.value.findIndex(x => x.public_id === selected.value.public_id)
    if (i >= 0) messages.value[i] = selected.value
    toast.success('处理状态已更新')
  } catch (e) { fail(e) }
  finally { busy.value = false }
}
onMounted(load)
</script>

<template>
  <h2>客服留言</h2>
  <p class="muted">访客留言仅在后台可见；没有公开展示或普通删除操作。请勿在回复中索取密码或完整银行卡信息。</p>
  <p v-if="demoHidden" class="panel">公开演示不会展示真实顾客留言、邮箱或 IP。正式部署并开启功能后，这里显示私密收件箱。</p>
  <div class="contact-toolbar" data-demo-view-control>
    <label>状态 <select v-model="status" @change="load"><option value="">全部</option><option value="new">新留言</option><option value="processing">处理中</option><option value="closed">已结案</option></select></label>
    <button :disabled="busy" @click="load">刷新</button>
  </div>
  <div class="contact-columns">
    <section class="panel">
      <h3>留言列表</h3>
      <p v-if="!messages.length" class="muted">暂无留言。</p>
      <button v-for="m in messages" :key="m.public_id" class="contact-row" @click="open(m)">
        <strong>{{ topics[m.topic] || m.topic }} · {{ statuses[m.status] || m.status }}</strong>
        <span>{{ m.name || '已匿名化' }} · {{ m.created_at }}</span>
        <span v-if="m.status === 'processing' && m.processing_by_name">处理人：{{ m.processing_by_name }}</span>
        <span v-if="m.status === 'closed' && m.closed_by_name">结案人：{{ m.closed_by_name }}</span>
      </button>
    </section>
    <section class="panel">
      <template v-if="selected">
        <h3>留言详情</h3>
        <dl class="contact-detail">
          <dt>状态</dt><dd>{{ statuses[selected.status] || selected.status }}</dd>
          <dt>{{ selected.status === 'processing' ? '处理人' : '最近处理人' }}</dt><dd>{{ selected.processing_by_name || '尚无记录（或为升级前操作）' }}<template v-if="selected.processing_at"> · {{ selected.processing_at }}</template></dd>
          <dt>{{ selected.status === 'closed' ? '结案人' : '上次结案人' }}</dt><dd>{{ selected.closed_by_name || '尚无记录（或为升级前操作）' }}<template v-if="selected.closed_at"> · {{ selected.closed_at }}</template></dd>
          <dt>姓名</dt><dd>{{ selected.name || '已匿名化' }}</dd>
          <dt>邮箱</dt><dd>{{ selected.email || '已匿名化' }}</dd>
          <dt>主题</dt><dd>{{ topics[selected.topic] || selected.topic }}</dd>
          <dt>订单号</dt><dd>{{ selected.order_ref || '—' }}</dd>
          <dt>IP 地址</dt><dd>{{ selected.ip_address || '已清除 / 未记录' }}</dd>
          <dt>大致所属地</dt><dd>{{ countryName(selected.country_code) }}{{ selected.country_code ? ` (${selected.country_code})` : '' }}</dd>
          <dt>时间</dt><dd>{{ selected.created_at }}</dd>
          <dt>内容</dt><dd class="contact-body">{{ selected.body || '已匿名化' }}</dd>
        </dl>
        <div v-if="canManage() && !selected.anonymized_at" class="contact-actions">
          <button :disabled="busy || selected.status === 'processing'" @click="setStatus('processing')">标记处理中</button>
          <button :disabled="busy || selected.status === 'closed'" @click="setStatus('closed')">结案</button>
          <button :disabled="busy || selected.status === 'new'" @click="setStatus('new')">重开</button>
        </div>
        <p class="muted">回复请使用商家邮箱。IP 所属地仅在可信代理提供国家码时显示，不代表精确位置。</p>
      </template>
      <p v-else class="muted">选择一条留言查看详情。</p>
    </section>
  </div>
</template>

<style scoped>
.contact-toolbar,.contact-actions{display:flex;align-items:center;gap:var(--sp-2);flex-wrap:wrap;margin:var(--sp-3) 0}.contact-toolbar label{display:flex;align-items:center;gap:var(--sp-2)}.contact-columns{display:grid;grid-template-columns:minmax(15rem,1fr) minmax(20rem,1.5fr);gap:var(--sp-3)}.contact-row{display:flex;width:100%;flex-direction:column;text-align:left;gap:.3rem;margin:.4rem 0;white-space:normal}.contact-row span{font-size:.85rem;color:var(--muted)}.contact-detail{display:grid;grid-template-columns:7rem minmax(0,1fr);gap:.7rem}.contact-detail dt{color:var(--muted)}.contact-detail dd{margin:0;overflow-wrap:anywhere}.contact-body{white-space:pre-wrap}@media(max-width:720px){.contact-columns{grid-template-columns:1fr}}
</style>
