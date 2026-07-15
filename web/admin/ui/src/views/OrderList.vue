<!-- 订单列表 / Order List. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<script setup>
import { ref, onMounted, inject } from 'vue'
import { useRouter } from 'vue-router'
import { api, APIError } from '../api.js'

const router = useRouter()
const onUnauthorized = inject('onUnauthorized')
const orders = ref([])
const err = ref('')

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
onMounted(load)
</script>

<template>
  <div class="container">
    <h2>订单</h2>
    <p v-if="err" class="err">{{ err }}</p>
    <p v-if="!orders.length" class="muted">暂无订单。</p>
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
.orders th,.orders td{text-align:left;padding:.5rem .6rem;border-bottom:1px solid var(--line)}
.orders th{font-size:.8rem;color:var(--muted)}
/* 可点击行：改名避开全局 .row 的 flex 工具类（display:flex 会破坏表格列宽算法致表头竖排） */
.clickable{cursor:pointer}
.clickable:hover{background:var(--panel)}
.badge{font-size:.74rem;border-radius:999px;padding:.1rem .5rem;border:1px solid var(--line)}
.badge.paid{color:var(--on-accent);background:var(--accent);border-color:var(--accent)}
.badge.refunded{color:#7a2e2e;background:#f6dede;border-color:#e6b8b8}
.badge.pending{color:var(--muted)}
</style>
