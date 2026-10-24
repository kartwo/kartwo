<!--
订单详情与退款 / Order Detail & Refund
功能：展示收款与履约记录，按付款及金额退款并继续核对未确认请求
作者：仗键天涯(daxing)
邮箱：3442535897@qq.com
时间：2026-10-24 23:40:00
-->
<script setup>
import { ref, computed, onMounted, inject } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'
import { confirm } from '../confirm.js'
import { refundCents } from '../refund-amount.js'
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
const selectedPayment = ref('')
const refundAmount = ref('')
const requestKey = ref('')
const reconciliation = ref({})
const refundable = computed(() => (o.value?.receipts || []).filter(p => p.refunded_cents < p.amount_cents && p.review_reason !== 'legacy_refund'))
const selectedReceipt = computed(() => refundable.value.find(p => `${p.provider}:${p.reference}` === selectedPayment.value))
function refundState(state) {
  return { prepared:'待提交', pending:'处理中', succeeded:'已成功', failed:'未成功', review:'待核对' }[state] || state
}
function refundReason(reason) {
  return { gateway_unconfirmed:'尚未确认网关结果，请继续原请求。', gateway_pending:'支付平台正在处理。', unknown_result:'请在支付平台找到这笔退款的编号，填写后核对。', result_mismatch:'返回信息不一致，请核对支付平台退款记录。', rejected:'支付平台拒绝了退款参数，请核对可退余额。', gateway_failed:'支付平台确认本次退款未成功。' }[reason] || ''
}
function choosePayment() {
  const p = selectedReceipt.value
  refundAmount.value = p ? ((p.amount_cents - p.refunded_cents) / 100).toFixed(2) : ''
  requestKey.value = ''
}
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
  const p = selectedReceipt.value
  const amount = refundCents(refundAmount.value)
  if (!p || amount === null || amount > p.amount_cents - p.refunded_cents) {
    toast.error('请选择付款，并填写不超过可退余额的金额（最多两位小数）。'); return
  }
  const ok = await confirm({ title:'确认退款', message:`向 ${p.provider} 付款 ${p.reference} 退回 ${money(amount, p.currency)}？请核对付款和金额。`, confirmText:'提交退款', danger:true })
  if (!ok || busy.value) return
  busy.value = true
  if (!requestKey.value) requestKey.value = crypto.randomUUID()
  try {
    const result = await api.createRefund(route.params.id, { request_key:requestKey.value, provider:p.provider, payment_ref:p.reference, amount_cents:amount, expected_refunded_cents:p.refunded_cents })
    toast.success(`退款请求：${refundState(result.state)}。请查看下方处理记录。`)
    requestKey.value = ''
    await load()
    selectedPayment.value = ''; refundAmount.value = ''
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    toast.error(e.message)
    await load()
  } finally { busy.value = false }
}
async function refreshRefund(r) {
  if (busy.value) return
  busy.value = true
  try {
    const result = await api.refreshRefund(route.params.id, r.public_id, reconciliation.value[r.public_id] || '')
    toast.success(`退款请求：${refundState(result.state)}`)
    await load()
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
	    <p>请检查下方每笔收款记录。超时到账、重复付款或历史退款需要分别核对；重复付款请选中对应交易办理退款，避免退错正常付款。</p>
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
      <div v-if="o.status === 'paid' && !(o.refund_requests || []).some(r => ['prepared','pending','review'].includes(r.state)) && (accountRole !== 'staff' || accountPermissions.includes('orders.fulfill'))" class="panel"><h3>发货与追踪</h3><input v-model="shipment.carrier" placeholder="承运商，例如 DHL"/><input v-model="shipment.tracking_number" placeholder="运单号"/><input v-model="shipment.tracking_url" placeholder="追踪链接（可选，https://）"/><button class="primary" :disabled="busy" @click="fulfill">确认发货</button></div>
      <section v-if="accountRole === 'owner' && refundable.length" class="panel refund-panel">
        <h3>按付款退款</h3>
        <label for="refund-payment">选择付款</label>
        <select id="refund-payment" v-model="selectedPayment" :disabled="busy" @change="choosePayment">
          <option value="">请选择要退款的付款</option>
          <option v-for="p in refundable" :key="`${p.provider}:${p.reference}`" :value="`${p.provider}:${p.reference}`">
            {{ p.provider }} · {{ p.reference }} · 可退 {{ money(p.amount_cents - p.refunded_cents, p.currency) }} · {{ reviewText(p.review_reason) }}
          </option>
        </select>
        <label for="refund-amount">退款金额 {{ selectedReceipt?.currency || '' }}</label>
        <input id="refund-amount" v-model="refundAmount" :disabled="busy" inputmode="decimal" maxlength="24" placeholder="例如 10.00" @input="requestKey = ''" />
        <p class="muted">退款不会自动补回库存；已发货订单的运单会保留。</p>
        <button class="danger" :disabled="busy || !selectedReceipt" @click="refund">{{ busy ? '处理中…' : '提交退款' }}</button>
      </section>
      <section v-if="o.refund_requests?.length" class="panel refund-panel">
        <h3>退款请求</h3>
        <article v-for="r in o.refund_requests" :key="r.public_id" class="refund-request">
          <strong>{{ money(r.amount_cents, r.currency) }} · {{ r.provider }} · {{ refundState(r.state) }}</strong>
          <p class="muted">{{ r.payment_ref }} · {{ r.created_at }}</p>
          <p v-if="accountRole === 'owner'" class="muted">请求编号：{{ r.public_id }}<template v-if="r.gateway_ref"> · 支付平台退款编号：{{ r.gateway_ref }}</template></p>
          <p v-if="r.reason">{{ refundReason(r.reason) }}</p>
          <template v-if="accountRole === 'owner' && ['prepared','pending','review'].includes(r.state)">
            <label v-if="r.state === 'review' && !r.gateway_ref">
              支付平台退款编号
              <input v-model="reconciliation[r.public_id]" :disabled="busy" placeholder="从这笔退款的支付平台记录中复制" />
            </label>
            <button :disabled="busy" @click="refreshRefund(r)">继续处理 / 核对结果</button>
          </template>
        </article>
      </section>
      <p v-if="accountRole === 'staff'" class="muted">退款仅限店主操作。</p>
    </template>
  </div>
</template>

<style scoped>
.refund-panel{margin-top:var(--sp-3)}
.refund-request{padding:var(--sp-3) 0;border-bottom:1px solid var(--border)}
.lines{width:100%;border-collapse:collapse;margin:.5rem 0 1rem}
.lines th,.lines td{text-align:left;padding:.4rem .6rem;border-bottom:1px solid var(--border)}
.lines th{font-size:var(--fs-sm);color:var(--text-muted)}
.badge{font-size:var(--fs-sm);border-radius:var(--radius-pill);padding:.15rem .6rem;border:1px solid var(--border)}
.badge.paid{color:var(--on-accent);background:var(--accent);border-color:var(--accent)}
.badge.refunded{color:var(--danger);background:var(--danger-bg);border-color:var(--danger-bg)}
.badge.payment_review{color:var(--danger);background:var(--danger-bg);border-color:var(--danger-bg)}
.payment-warning{margin-top:var(--sp-3);color:var(--danger);border-color:var(--danger)}
</style>
