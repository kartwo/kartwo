<!-- 概览 / Dashboard. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<!-- 功能：登录后首页——开店进度引导 + 今日/近7日订单与销售额 + 待处理 + 商品数 + 库存告警；空态诚实、无假数据 -->
<script setup>
import { ref, computed, onMounted, inject } from 'vue'
import { api, APIError } from '../api.js'

const onUnauthorized = inject('onUnauthorized', null)
const data = ref(null)
const loaded = ref(false)

async function load() {
  try {
    data.value = await api.dashboard()
  } catch (e) {
    if (e instanceof APIError && e.status === 401 && onUnauthorized) return onUnauthorized()
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
  <div class="container">
    <h2>概览</h2>

    <template v-if="data">
      <!-- 开店进度引导（呼应北极星：有商品 / 已配收款 / 已配域名） -->
      <section class="guide">
        <div v-if="setup.ready" class="ready-banner">
          🎉 <strong>开店就绪</strong>——商品、收款、域名都已配好，可以正式营业了。
        </div>
        <template v-else>
          <p class="muted" style="margin:.2rem 0 .6rem">开店还差几步，点卡片继续：</p>
          <div class="guide-grid">
            <RouterLink v-if="!setup.has_products" to="/products/new" class="guide-card">
              <span class="g-title">发布首个商品 →</span>
              <span class="muted g-desc">店里还没有商品，先上架一个能卖的。</span>
            </RouterLink>
            <RouterLink v-if="!setup.payment_configured" to="/payment" class="guide-card">
              <span class="g-title">配置收款 →</span>
              <span class="muted g-desc">接上 Stripe 或 PayPal，顾客才能真正付款。</span>
            </RouterLink>
            <RouterLink v-if="!setup.domain_configured" to="/domain" class="guide-card">
              <span class="g-title">配置域名，让店面上 HTTPS →</span>
              <span class="muted g-desc">店面当前是 http:// 地址；配好域名可自动申请 HTTPS 绿锁。</span>
            </RouterLink>
          </div>
        </template>
      </section>

      <!-- 统计卡 -->
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

      <p v-if="noOrdersYet" class="muted empty-hint">还没有订单——把店面链接分享出去，第一单很快就来。</p>
    </template>

    <p v-else-if="loaded" class="muted">概览暂时加载不出来，请刷新重试。</p>
    <p v-else class="muted">加载中…</p>
  </div>
</template>

<style scoped>
.guide { margin: .4rem 0 1.4rem; }
.ready-banner { border: 1px solid var(--ok); border-radius: 12px; padding: .9rem 1.1rem; line-height: 1.5; }
.guide-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: .8rem; }
.guide-card { display: flex; flex-direction: column; gap: .3rem; text-decoration: none; color: inherit;
  border: 1px solid var(--accent); border-radius: 12px; padding: .9rem 1rem; background: var(--panel); }
.guide-card:hover { border-color: var(--text); }
.g-title { font-weight: 600; white-space: nowrap; }
.g-desc { font-size: .85rem; line-height: 1.4; white-space: normal; }

.stat-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: .8rem; }
.stat { border: 1px solid var(--line); border-radius: 12px; padding: .9rem 1rem; background: var(--panel);
  text-decoration: none; color: inherit; display: block; }
.stat.linkable:hover { border-color: var(--accent); }
.s-label { font-size: .85rem; color: var(--muted); white-space: nowrap; }
.s-value { font-size: 1.8rem; font-weight: 700; margin: .2rem 0; }
.s-value.warn { color: var(--danger); }
.s-value-sm { font-size: 1.1rem; font-weight: 600; color: var(--muted); }
.s-sub { font-size: .8rem; white-space: nowrap; }
.empty-hint { margin-top: 1rem; }
</style>
