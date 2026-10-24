<!-- 订单详情 + 退款 / Order Detail & Refund. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<script setup>
import { ref, computed, onMounted, inject } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'
import { confirm } from '../confirm.js'
import ErrorState from '../components/ErrorState.vue'

const route = useRoute()
const router = useRouter()
const onUnauthorized = inject('onUnauthorized')
const accountRole = inject('accountRole', ref('owner'))
const accountPermissions = inject('accountPermissions', ref([]))
const toast = useToast()
const o = ref(null)
const err = ref('') // 仅页级：订单加载失败 / 404「订单不存在」的常驻错误（D2 保留 inline）
const busy = ref(false)
const shipment = ref({ carrier:'', tracking_number:'', tracking_url:'' })
const canRefund = computed(() => o.value && ['paid','fulfilled','payment_review'].includes(o.value.status)
  && !o.value.refunded_cents && !(o.value.receipts || []).some(p => p.review_reason === 'duplicate_payment' && p.refunded_cents < p.amount_cents))
function reviewText(reason) {
  return { late_payment:'超时到账', duplicate_payment:'重复付款', legacy_refund:'历史退款待核对' }[reason] || '正常收款'
}


function money(cents, cur) { return (cents / 100).toFixed(2) + ' ' + (cur || '') }
function statusText(s) {
  return { pending: '待付款', paid: '已付款', refunded: '已退款', cancelled: '已取消', fulfilled: '已发货', payment_review: '收款异常待处理' }[s] || s
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
  const ok = await confirm({
    title: '确认退款',
    message: '确认对该订单整单全额退款？此操作不可撤销。',
    confirmText: '退款', danger: true,
  })
  if (!ok) return
  busy.value = true
  try {
    const r = await api.refundOrder(route.params.id)
    o.value.status = r.status || 'refunded'
    await load()
    toast.success('退款结果已同步，请核对收款与退款记录。')
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    toast.error(e.message)
  } finally { busy.value = false }
}
async function fulfill() { if(!shipment.value.carrier||!shipment.value.tracking_number){toast.error('请填写承运商和运单号');return};busy.value=true;try{await api.fulfillOrder(route.params.id,shipment.value);await load();toast.success('订单已标记为发货')}catch(e){toast.error(e.message)}finally{busy.value=false} }
onMounted(load)
</script>

<template>
  <div class="container" style="max-width:760px">
    <p><a href="javascript:void(0)" @click="router.push('/orders')">← 返回订单列表</a></p>
    <ErrorState v-if="err" :message="err" :retry="load" />

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
	  <div v-if="o.payment_review || o.status === 'payment_review'" class="panel payment-warning">
	    <strong>收款需要核对，请勿继续发货。</strong>
	    <p>请检查下方每笔收款记录。超时到账、重复付款或历史退款需要分别核对；重复付款请按对应交易在支付平台处理，避免退错正常付款。</p>
	  </div>

      <template v-if="o.receipts?.length">
        <h3>收款与退款</h3>
        <p>累计收款 {{ money(o.paid_cents, o.currency) }} · 已退 {{ money(o.refunded_cents, o.currency) }}</p>
        <p>履约：{{ o.fulfillment_status === 'fulfilled' ? '已发货（退款不删除发货记录）' : '未发货' }}</p>
        <ul>
          <li v-for="(p, i) in o.receipts" :key="i">
            {{ p.provider }} · {{ money(p.amount_cents, p.currency) }} · 已退 {{ money(p.refunded_cents, p.currency) }}
            · {{ reviewText(p.review_reason) }}{{ p.source === 'legacy' ? '（历史记录）' : '' }}
            <p class="muted">{{ p.reference }}</p>
          </li>
        </ul>
      </template>
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
      <div v-if="o.status === 'paid' && (accountRole !== 'staff' || accountPermissions.includes('orders.fulfill'))" class="panel"><h3>发货与追踪</h3><input v-model="shipment.carrier" placeholder="承运商，例如 DHL"/><input v-model="shipment.tracking_number" placeholder="运单号"/><input v-model="shipment.tracking_url" placeholder="追踪链接（可选，https://）"/><button class="primary" :disabled="busy" @click="fulfill">确认发货</button></div>
      <button v-if="canRefund && accountRole !== 'staff'" class="danger" :disabled="busy" @click="refund">
        {{ busy ? '退款中…' : '整单全额退款' }}
      </button>
      <p v-else-if="o.status === 'refunded'" class="muted">该订单已退款。</p>
      <p v-else-if="accountRole === 'staff'" class="muted">退款仅限店主操作。</p>
      <p v-else class="muted">当前无法整单退款。部分退款或重复收款请按上方交易在支付平台核对处理。</p>
    </template>
  </div>
</template>

<style scoped>
.lines{width:100%;border-collapse:collapse;margin:.5rem 0 1rem}
.lines th,.lines td{text-align:left;padding:.4rem .6rem;border-bottom:1px solid var(--border)}
.lines th{font-size:var(--fs-sm);color:var(--text-muted)}
.badge{font-size:var(--fs-sm);border-radius:var(--radius-pill);padding:.15rem .6rem;border:1px solid var(--border)}
.badge.paid{color:var(--on-accent);background:var(--accent);border-color:var(--accent)}
.badge.refunded{color:var(--danger);background:var(--danger-bg);border-color:var(--danger-bg)}
.badge.payment_review{color:var(--danger);background:var(--danger-bg);border-color:var(--danger-bg)}
.payment-warning{margin-top:var(--sp-3);color:var(--danger);border-color:var(--danger)}
</style>
