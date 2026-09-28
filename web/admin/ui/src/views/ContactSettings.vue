<!-- 客服留言设置 / Contact Settings
功能：店主配置公开表单及到期匿名化周期
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
const form = ref({ enabled: false, retention_months: 12, ip_retention_days: 90 })
const busy = ref(false)
async function load() {
  try { form.value = await api.contactSettings() }
  catch (e) { if (e instanceof APIError && e.status === 401) onUnauthorized(); else toast.error(e.message) }
}
async function save() {
  busy.value = true
  try { form.value = await api.saveContactSettings(form.value); toast.success('客服留言设置已保存') }
  catch (e) { if (e instanceof APIError && e.status === 401) onUnauthorized(); else toast.error(e.message) }
  finally { busy.value = false }
}
onMounted(load)
</script>

<template>
  <h2>客服留言设置</h2>
  <p class="muted">这是私密客服表单，不是公开留言板。默认关闭；开启后店面页脚的 Contact Us 会打开表单。</p>
  <form class="panel contact-settings" @submit.prevent="save">
    <label class="toggle"><input v-model="form.enabled" type="checkbox" :disabled="role !== 'owner'" /> 开启在线留言</label>
    <label>结案后保留完整留言（月）<input v-model.number="form.retention_months" type="number" min="1" max="24" required :disabled="role !== 'owner'" /></label>
    <label>IP 地址保留（天）<input v-model.number="form.ip_retention_days" type="number" min="7" max="90" required :disabled="role !== 'owner'" /></label>
    <p class="muted">默认：结案后 12 个月匿名化姓名、邮箱、正文和订单号；IP 从提交起最多保留 90 天。未结案留言保留处理所需内容。记录本身不会从后台普通操作中删除。到期清理任务每日运行。</p>
    <button v-if="role === 'owner'" class="primary" :disabled="busy">保存设置</button>
    <p v-else class="muted">仅店主可修改设置。</p>
  </form>
</template>

<style scoped>
.contact-settings{display:grid;gap:var(--sp-3);max-width:44rem}.contact-settings label:not(.toggle){display:grid;gap:var(--sp-2)}.contact-settings input[type=number]{max-width:14rem}.toggle{display:flex;gap:var(--sp-2);align-items:center}.toggle input{width:auto}.contact-settings button{justify-self:start}
</style>
