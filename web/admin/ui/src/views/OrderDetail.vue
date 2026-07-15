<!-- 订单详情 + 退款 / Order Detail & Refund. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<script setup>
import { ref, onMounted, inject } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'

const route = useRoute()
const router = useRouter()
const onUnauthorized = inject('onUnauthorized')
const toast = useToast()
const o = ref(null)
const err = ref('') // 仅页级：订单加载失败 / 404「订单不存在」的常驻错误（D2 保留 inline）
const busy = ref(false)

function money(cents, cur) { return (cents / 100).toFixed(2) + ' ' + (cur || '') }
function statusText(s) {
  return { pending: '待付款', paid: '已付款', refunded: '已退款', cancelled: '已取消', fulfilled: '已发货' }[s] || s
}

async function load() {
  err.value = ''
  try {
    o.value = await api.getOrder(route.params.id)
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    if (e instanceof APIError && e.status === 404) { err.value = '订单不存在'; return }
    err.value = e.message
  }
}

async function refund() {
  if (busy.value) return
  if (!window.confirm('确认对该订单整单全额退款？此操作不可撤销。')) return
  busy.value = true
  try {
    const r = await api.refundOrder(route.params.id)
    o.value.status = r.status || 'refunded'
    await load()
    toast.success('退款成功，订单已转为「已退款」。')
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    toast.error(e.message)
  } finally { busy.value = false }
}
onMounted(load)
</script>

<template>
  <div class="container" style="max-width:760px">
    <p><a href="javascript:void(0)" @click="router.push('/orders')">← 返回订单列表</a></p>
    <p v-if="err" class="err">{{ err }}</p>

    <template v-if="o">
      <div class="row" style="justify-content:space-between;align-items:center">
        <h2 style="margin:0">订单 <code>{{ o.public_id.slice(0, 8) }}</code></h2>
        <span class="badge" :class="o.status">{{ statusText(o.status) }}</span>
      </div>

      <div class="panel card" style="margin-top:1rem">
        <p><strong>金额：</strong>{{ money(o.total_cents, o.currency) }}（小计 {{ money(o.subtotal_cents, o.currency) }}）</p>
        <p><strong>邮箱：</strong>{{ o.email }}</p>
        <p><strong>收货：</strong>{{ o.ship_name }} · {{ o.ship_phone }} · {{ o.ship_address }} {{ o.ship_country }}</p>
        <p><strong>支付通道：</strong>{{ o.payment_provider || '—' }}</p>
        <p class="muted">{{ (o.created_at || '').slice(0, 19).replace('T', ' ') }}</p>
      </div>

      <h3>商品</h3>
      <table class="lines">
        <thead><tr><th>商品</th><th>规格</th><th>单价</th><th>数量</th><th>小计</th></tr></thead>
        <tbody>
          <tr v-for="(l, i) in o.lines" :key="i">
            <td>{{ l.title }}</td><td class="muted">{{ l.spec }}</td>
            <td>{{ money(l.unit_cents, o.currency) }}</td><td>{{ l.quantity }}</td>
            <td>{{ money(l.line_cents, o.currency) }}</td>
          </tr>
        </tbody>
      </table>

      <template v-if="o.refunds && o.refunds.length">
        <h3>退款记录</h3>
        <ul>
          <li v-for="(rf, i) in o.refunds" :key="i" class="muted">
            {{ money(rf.amount_cents, o.currency) }} · {{ rf.provider }} · {{ rf.provider_refund_id }} · {{ (rf.created_at || '').slice(0,19).replace('T',' ') }}
          </li>
        </ul>
      </template>

      <div class="spacer"></div>
      <button v-if="o.status === 'paid'" class="danger" :disabled="busy" @click="refund">
        {{ busy ? '退款中…' : '整单全额退款' }}
      </button>
      <p v-else-if="o.status === 'refunded'" class="muted">该订单已退款。</p>
      <p v-else class="muted">仅「已付款」订单可退款。</p>
    </template>
  </div>
</template>

<style scoped>
.lines{width:100%;border-collapse:collapse;margin:.5rem 0 1rem}
.lines th,.lines td{text-align:left;padding:.4rem .6rem;border-bottom:1px solid var(--line)}
.lines th{font-size:.8rem;color:var(--muted)}
.badge{font-size:.78rem;border-radius:999px;padding:.15rem .6rem;border:1px solid var(--line)}
.badge.paid{color:#03263a;background:var(--accent);border-color:var(--accent)}
.badge.refunded{color:#7a2e2e;background:#f6dede;border-color:#e6b8b8}
.danger{background:#c0392b;color:#fff;border-color:#c0392b}
.danger:disabled{opacity:.6}
</style>
