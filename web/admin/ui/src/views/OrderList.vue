<!-- 订单列表 / Order List. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<script setup>
import { ref, onMounted, inject } from 'vue'
import { useRouter } from 'vue-router'
import { api, APIError } from '../api.js'
import ErrorState from '../components/ErrorState.vue'
import { useToast } from '../toast.js'

const router = useRouter()
const onUnauthorized = inject('onUnauthorized')
const toast = useToast()
const orders = ref([])
const err = ref('')
const exporting = ref(false)
const exportFilter = ref({ status: '', from: '', to: '' })

function money(cents, cur) { return (cents / 100).toFixed(2) + ' ' + cur }
function statusText(s) {
  return { pending: '待付款', paid: '已付款', refunded: '已退款', cancelled: '已取消', fulfilled: '已发货' }[s] || s
}

async function load() {
  err.value = ''
  try {
    const r = await api.listOrders()
    orders.value = r.orders || []
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    err.value = e.message
  }
}
async function exportOrders() {
  exporting.value = true
  try {
    const blob = await api.exportOrdersCSV(exportFilter.value)
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = 'kartwo-orders.csv'
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
    toast.success('订单 CSV 已开始下载')
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    toast.error(e.message)
  } finally {
    exporting.value = false
  }
}
onMounted(load)
</script>

<template>
  <div class="container">
    <div class="title-row">
      <h2>订单</h2>
      <div class="export-actions">
        <select v-model="exportFilter.status" aria-label="导出订单状态">
          <option value="">全部状态</option><option value="pending">待付款</option><option value="paid">已付款</option><option value="fulfilled">已发货</option><option value="refunded">已退款</option><option value="cancelled">已取消</option>
        </select>
        <label>从<input v-model="exportFilter.from" type="date" /></label>
        <label>至<input v-model="exportFilter.to" type="date" /></label>
        <button :disabled="exporting" @click="exportOrders">{{ exporting ? '正在导出…' : '导出订单 CSV' }}</button>
      </div>
    </div>
    <ErrorState v-if="err" :message="err" :retry="load" />
    <p v-else-if="!orders.length" class="muted">暂无订单。</p>
    <table v-else class="orders">
      <thead>
        <tr><th>订单号</th><th>状态</th><th>邮箱</th><th>金额</th><th>时间</th></tr>
      </thead>
      <tbody>
        <tr v-for="o in orders" :key="o.public_id" class="clickable" @click="router.push('/orders/' + o.public_id)">
          <td><code>{{ o.public_id.slice(0, 8) }}</code></td>
          <td><span class="badge" :class="o.status">{{ statusText(o.status) }}</span></td>
          <td>{{ o.email }}</td>
          <td>{{ money(o.total_cents, o.currency) }}</td>
          <td class="muted">{{ o.created_at.slice(0, 19).replace('T', ' ') }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
.orders{width:100%;border-collapse:collapse;margin-top:1rem}
.title-row{display:flex;align-items:center;justify-content:space-between;gap:var(--sp-3)}
.title-row h2{margin:0}
.export-actions{display:flex;align-items:center;justify-content:flex-end;gap:var(--sp-2);flex-wrap:wrap}
.export-actions label{display:flex;align-items:center;gap:var(--sp-1);font-size:var(--fs-sm);color:var(--text-muted)}
.export-actions select,.export-actions input{width:auto}
.orders th,.orders td{text-align:left;padding:.5rem .6rem;border-bottom:1px solid var(--border)}
.orders th{font-size:var(--fs-sm);color:var(--text-muted)}
/* 可点击行：改名避开全局 .row 的 flex 工具类（display:flex 会破坏表格列宽算法致表头竖排） */
.clickable{cursor:pointer}
.clickable:hover{background:var(--surface)}
.badge{font-size:var(--fs-xs);border-radius:var(--radius-pill);padding:.1rem .5rem;border:1px solid var(--border)}
.badge.paid{color:var(--on-accent);background:var(--accent);border-color:var(--accent)}
.badge.refunded{color:var(--danger);background:var(--danger-bg);border-color:var(--danger-bg)}
.badge.pending{color:var(--text-muted)}
</style>
