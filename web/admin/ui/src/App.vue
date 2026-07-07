<!-- 应用外壳与鉴权 / App Shell & Auth. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<script setup>
import { ref, computed, onMounted, onUnmounted, provide } from 'vue'
import { api, APIError } from './api.js'
import MarketSelect from './views/MarketSelect.vue'
import PaymentWizard from './views/PaymentWizard.vue'
import DomainWizard from './views/DomainWizard.vue'
import WizardProgress from './components/WizardProgress.vue'
import ToastHost from './components/ToastHost.vue'

const loading = ref(true)
const initialized = ref(false)
const authed = ref(false)
const marketConfigured = ref(false)
const paymentStepNeeded = ref(false)
const domainStepNeeded = ref(false)
const username = ref('')

// 向导「第 X / N 步」进度：固定三步流，跳过的步骤仍占位、步号不跳变（口径见 DECISIONS）。
const wizardStep = computed(() => {
  if (!marketConfigured.value) return 1
  if (paymentStepNeeded.value) return 2
  return 3 // 域名步（domainStepNeeded 为真时展示）
})

// checkDomainStep 查询是否仍需展示域名步骤（收款步完成后调用）。
async function checkDomainStep() {
  try { domainStepNeeded.value = !!(await api.wizardDomain()).needed } catch (_) { domainStepNeeded.value = false }
}
const form = ref({ user: '', pass: '' })
const err = ref('')
const busy = ref(false)

async function refresh() {
  loading.value = true
  err.value = ''
  try {
    const s = await api.status()
    initialized.value = !!s.initialized
    if (initialized.value) {
      try {
        const me = await api.me()
        authed.value = true
        username.value = me.username
        const mk = await api.getMarket()
        marketConfigured.value = !!mk.configured
        if (marketConfigured.value) {
          try { paymentStepNeeded.value = !!(await api.wizardPayment()).needed } catch (_) { paymentStepNeeded.value = false }
          if (!paymentStepNeeded.value) await checkDomainStep()
        }
      } catch (e) {
        authed.value = false
      }
    }
  } finally {
    loading.value = false
  }
}

async function onMarketConfigured() {
  marketConfigured.value = true
  try { paymentStepNeeded.value = !!(await api.wizardPayment()).needed } catch (_) { paymentStepNeeded.value = false }
  if (!paymentStepNeeded.value) await checkDomainStep()
}
// 收款步完成 → 进入域名步（若仍需要）。
async function onPaymentStepDone() {
  paymentStepNeeded.value = false
  await checkDomainStep()
}
// 域名步完成（保存或跳过）→ 进入后台。
function onDomainStepDone() { domainStepNeeded.value = false }
// 域名步「上一步」→ 回到收款步（不允许回退已配市场）；收款步完成后会再回到域名步。
function onDomainBack() { paymentStepNeeded.value = true }
onMounted(() => window.addEventListener('market-configured', onMarketConfigured))
onUnmounted(() => window.removeEventListener('market-configured', onMarketConfigured))

async function doSetup() {
  busy.value = true; err.value = ''
  try {
    await api.setup(form.value.user, form.value.pass)
    await api.login(form.value.user, form.value.pass)
    await refresh()
  } catch (e) {
    err.value = e.message
  } finally { busy.value = false }
}

async function doLogin() {
  busy.value = true; err.value = ''
  try {
    await api.login(form.value.user, form.value.pass)
    await refresh()
  } catch (e) {
    err.value = e.message
  } finally { busy.value = false }
}

async function doLogout() {
  try { await api.logout() } catch (_) { /* ignore */ }
  authed.value = false
  form.value = { user: '', pass: '' }
}

// 子组件遇 401 时调用，回到登录态。
provide('onUnauthorized', () => { authed.value = false })

onMounted(refresh)
</script>

<template>
  <!-- 全局 toast 宿主：始终挂载、悬浮不随滚动，跨所有页面状态可用 -->
  <ToastHost />

  <div v-if="loading" class="center-screen muted">加载中…</div>

  <!-- 未初始化：建管理员 + 设主口令 -->
  <div v-else-if="!initialized" class="center-screen">
    <div class="panel card">
      <h2>初始化 Kartwo</h2>
      <p class="muted">创建管理员并设置主口令（用于登录与配置加密）。</p>
      <label>管理员用户名</label>
      <input v-model="form.user" autocomplete="username" />
      <label>主口令（至少 8 位）</label>
      <input v-model="form.pass" type="password" autocomplete="new-password" @keyup.enter="doSetup" />
      <p v-if="err" class="err">{{ err }}</p>
      <div class="spacer"></div>
      <button class="primary" :disabled="busy" @click="doSetup">创建并登录</button>
    </div>
  </div>

  <!-- 已初始化未登录：登录 -->
  <div v-else-if="!authed" class="center-screen">
    <div class="panel card">
      <h2>登录 Kartwo Admin</h2>
      <label>用户名</label>
      <input v-model="form.user" autocomplete="username" />
      <label>口令</label>
      <input v-model="form.pass" type="password" autocomplete="current-password" @keyup.enter="doLogin" />
      <p v-if="err" class="err">{{ err }}</p>
      <div class="spacer"></div>
      <button class="primary" :disabled="busy" @click="doLogin">登录</button>
    </div>
  </div>

  <!-- 已登录但未选市场：强制走「选择主攻市场」向导步骤 -->
  <template v-else-if="!marketConfigured">
    <header class="app-header">
      <div class="brand">Kartwo Admin · 开店向导</div>
      <button @click="doLogout">登出</button>
    </header>
    <WizardProgress :step="wizardStep" />
    <MarketSelect />
  </template>

  <!-- 市场已选、收款未配且未跳过：走「配置收款」向导步骤 -->
  <template v-else-if="paymentStepNeeded">
    <header class="app-header">
      <div class="brand">Kartwo Admin · 开店向导</div>
      <button @click="doLogout">登出</button>
    </header>
    <WizardProgress :step="wizardStep" />
    <PaymentWizard @done="onPaymentStepDone" />
  </template>

  <!-- 收款已配/跳过、域名未配且未跳过：走「配置域名」向导步骤 -->
  <template v-else-if="domainStepNeeded">
    <header class="app-header">
      <div class="brand">Kartwo Admin · 开店向导</div>
      <button @click="doLogout">登出</button>
    </header>
    <WizardProgress :step="wizardStep" />
    <DomainWizard @done="onDomainStepDone" @back="onDomainBack" />
  </template>

  <!-- 已登录：应用 -->
  <template v-else>
    <header class="app-header">
      <div class="brand">Kartwo Admin</div>
      <div class="row" style="gap:1rem; flex: 0;">
        <RouterLink to="/products">商品</RouterLink>
        <RouterLink to="/orders">订单</RouterLink>
        <RouterLink to="/market">市场</RouterLink>
        <RouterLink to="/payment">收款</RouterLink>
        <RouterLink to="/domain">域名</RouterLink>
        <span class="muted">{{ username }}</span>
        <button @click="doLogout">登出</button>
      </div>
    </header>
    <main class="container">
      <RouterView />
    </main>
  </template>
</template>
