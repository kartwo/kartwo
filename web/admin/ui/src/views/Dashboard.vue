<!-- 经营概览 / Store Dashboard
功能：开店进度与真实订单、商品和库存数据总览
作者：仗键天涯(daxing)
邮箱：3442535897@qq.com
时间：2026-10-13 19:30:00 -->
<script setup>
import { ref, computed, onMounted, inject } from 'vue'
import { api, APIError } from '../api.js'
import ErrorState from '../components/ErrorState.vue'

const onUnauthorized = inject('onUnauthorized', null)
const data = ref(null)
const loaded = ref(false)
const err = ref('') // 仅页级：概览加载失败的常驻错误

async function load() {
  err.value = ''
  try {
    data.value = await api.dashboard()
  } catch (e) {
    if (e instanceof APIError && e.status === 401 && onUnauthorized) return onUnauthorized()
    err.value = e.message
  } finally { loaded.value = true }
}

// 金额：整数分 → 按市场货币展示（展示层转换，内核始终整数分）。
function money(cents) {
  const cur = data.value?.currency || 'USD'
  const major = (cents || 0) / 100
  try { return new Intl.NumberFormat(undefined, { style: 'currency', currency: cur }).format(major) }
  catch { return major.toFixed(2) + ' ' + cur }
}

const setup = computed(() => data.value?.setup || {})
const orders = computed(() => data.value?.orders || {})
const products = computed(() => data.value?.products || {})
const noOrdersYet = computed(() => (orders.value.week?.count || 0) === 0)
onMounted(load)
</script>

<template>
  <div class="dashboard">
    <template v-if="data">
      <section class="overview-intro">
        <div>
          <span class="section-kicker">STORE AT A GLANCE</span>
          <h2>经营概览</h2>
          <p>订单、商品与开店进度，在这里一眼掌握。</p>
        </div>
        <span class="overview-status">{{ setup.ready ? '店铺已就绪' : '继续完成开店设置' }}</span>
      </section>

      <!-- 开店进度引导（呼应北极星：有商品 / 已配收款 / 已配域名） -->
      <section class="guide">
        <div v-if="setup.ready" class="ready-banner">
          <span class="ready-icon" aria-hidden="true">✓</span><span><strong>开店就绪</strong><small>商品、收款、域名都已配好，可以正式营业了。</small></span>
        </div>
        <template v-else>
          <div class="section-heading"><div><span class="section-kicker">GET STARTED</span><h3>完成开店准备</h3></div><p>点选下一步，继续配置店铺。</p></div>
          <div class="guide-grid">
            <RouterLink v-if="!setup.has_products" to="/products/new" class="guide-card">
              <span class="g-title">发布首个商品 <span aria-hidden="true">→</span></span>
              <span class="muted g-desc">店里还没有商品，先上架一个能卖的。</span>
            </RouterLink>
            <RouterLink v-if="!setup.payment_configured" to="/payment" class="guide-card">
              <span class="g-title">配置收款 <span aria-hidden="true">→</span></span>
              <span class="muted g-desc">接上 Stripe 或 PayPal，顾客才能真正付款。</span>
            </RouterLink>
            <RouterLink v-if="!setup.domain_configured" to="/domain" class="guide-card">
              <span class="g-title">配置域名与 HTTPS <span aria-hidden="true">→</span></span>
              <span class="muted g-desc">店面当前是 http:// 地址；配好域名可自动申请 HTTPS 绿锁。</span>
            </RouterLink>
          </div>
        </template>
      </section>

      <!-- 统计卡 -->
      <div class="section-heading stats-heading"><div><span class="section-kicker">LIVE NUMBERS</span><h3>店铺数据</h3></div><p>销售额按当前市场货币显示。</p></div>
      <div class="stat-grid">
        <div class="stat">
          <div class="s-label">今日订单</div>
          <div class="s-value">{{ orders.today.count }}</div>
          <div class="muted s-sub">销售额 {{ money(orders.today.sales_cents) }}</div>
        </div>
        <div class="stat">
          <div class="s-label">近 7 日订单</div>
          <div class="s-value">{{ orders.week.count }}</div>
          <div class="muted s-sub">销售额 {{ money(orders.week.sales_cents) }}</div>
        </div>
        <RouterLink to="/orders" class="stat linkable">
          <div class="s-label">待处理订单</div>
          <div class="s-value" :class="{ warn: orders.pending_fulfillment > 0 }">{{ orders.pending_fulfillment }}</div>
          <div class="muted s-sub">已付待发货</div>
        </RouterLink>
        <RouterLink to="/products" class="stat linkable">
          <div class="s-label">商品数</div>
          <div class="s-value">{{ products.count }}</div>
          <div class="muted s-sub">已上架（不含已删）</div>
        </RouterLink>
        <RouterLink to="/products" class="stat linkable">
          <div class="s-label">库存告警</div>
          <div class="s-value" :class="{ warn: products.zero_stock > 0 || products.low_stock > 0 }">
            {{ products.zero_stock }} <span class="s-value-sm">/ {{ products.low_stock }}</span>
          </div>
          <div class="muted s-sub">零库存 / 低库存(≤5)</div>
        </RouterLink>
      </div>

      <p v-if="noOrdersYet" class="empty-hint">还没有订单。可以分享店面链接，邀请顾客访问。</p>
    </template>

    <ErrorState v-else-if="loaded && err" :message="err" :retry="load" />
    <p v-else-if="loaded" class="muted">概览暂时加载不出来，请刷新重试。</p>
    <p v-else class="muted">加载中…</p>
  </div>
</template>

<style scoped>
.dashboard{display:grid;gap:var(--sp-6)}
.overview-intro{display:flex;align-items:center;justify-content:space-between;gap:var(--sp-6);padding:var(--sp-8);border:1px solid var(--border);border-radius:var(--radius-lg);background:linear-gradient(110deg,var(--intro-start),var(--intro-end));box-shadow:var(--shadow-sm)}
.section-kicker{color:var(--accent);font-size:var(--fs-xs);font-weight:700;letter-spacing:.13em}
.overview-intro h2{margin:var(--sp-2) 0;font-size:var(--fs-xl);letter-spacing:-.025em}
.overview-intro p,.section-heading p{margin:0;color:var(--text-muted);font-size:var(--fs-sm)}
.overview-status{flex:none;padding:var(--sp-2) var(--sp-4);border-radius:var(--radius-pill);background:var(--intro-end);color:var(--accent);font-size:var(--fs-sm);font-weight:600}
.guide{display:grid;gap:var(--sp-4)}
.section-heading{display:flex;align-items:end;justify-content:space-between;gap:var(--sp-4)}
.section-heading h3{margin:var(--sp-1) 0 0;font-size:var(--fs-lg)}
.ready-banner{display:flex;align-items:center;gap:var(--sp-4);padding:var(--sp-4) var(--sp-6);border:1px solid var(--success);border-radius:var(--radius-lg);background:var(--success-bg);color:var(--success)}
.ready-icon{display:grid;place-items:center;width:34px;height:34px;flex:none;border:1px solid currentColor;border-radius:var(--radius-pill);font-weight:700}
.ready-banner small{display:block;margin-top:var(--sp-1);font-size:var(--fs-sm)}
.guide-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(240px,1fr));gap:var(--sp-4)}
.guide-card{display:flex;flex-direction:column;gap:var(--sp-3);min-height:124px;padding:var(--sp-6);border:1px solid var(--border);border-radius:var(--radius-lg);background:var(--surface);box-shadow:var(--shadow-sm);color:inherit;text-decoration:none;transition:box-shadow .15s ease,border-color .15s ease,transform .15s ease}
.guide-card:hover{border-color:var(--accent);box-shadow:var(--shadow-md);text-decoration:none;transform:translateY(-2px)}
.guide-card:focus-visible,.stat.linkable:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
.g-title{display:flex;justify-content:space-between;gap:var(--sp-2);color:var(--text);font-weight:700}
.g-title span{color:var(--accent)}
.g-desc{font-size:var(--fs-sm);line-height:1.5}
.stats-heading{margin-top:var(--sp-2)}
.stat-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:var(--sp-4)}
.stat{display:flex;flex-direction:column;gap:var(--sp-2);min-height:148px;padding:var(--sp-6);border:1px solid var(--border);border-radius:var(--radius-lg);background:var(--surface);box-shadow:var(--shadow-sm);color:inherit;text-decoration:none;transition:box-shadow .15s ease,border-color .15s ease,transform .15s ease}
.stat.linkable:hover{border-color:var(--accent);box-shadow:var(--shadow-md);text-decoration:none;transform:translateY(-2px)}
.s-label{color:var(--text-muted);font-size:var(--fs-sm)}
.s-value{margin:0;color:var(--text);font-size:2rem;font-weight:700;line-height:1.15;font-variant-numeric:tabular-nums}
.s-value.warn{color:var(--danger)}
.s-value-sm{color:var(--text-muted);font-size:var(--fs-lg);font-weight:600}
.s-sub{margin-top:auto;color:var(--text-subtle);font-size:var(--fs-xs)}
.empty-hint{margin:0;padding:var(--sp-4) var(--sp-6);border:1px dashed var(--border-strong);border-radius:var(--radius-md);color:var(--text-muted);font-size:var(--fs-sm)}
@media(max-width:700px){.overview-intro{align-items:flex-start;flex-direction:column;padding:var(--sp-6)}.section-heading{align-items:flex-start;flex-direction:column;gap:var(--sp-2)}.stat-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.stat{min-height:136px;padding:var(--sp-4)}}
@media(max-width:420px){.stat-grid{grid-template-columns:1fr}.stat{min-height:120px}}
</style>
