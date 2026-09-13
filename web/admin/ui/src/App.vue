<!-- 应用外壳与鉴权 / App Shell & Auth. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<script setup>
import { ref, computed, onMounted, onUnmounted, provide } from 'vue'
import { api, APIError } from './api.js'
import PaymentWizard from './views/PaymentWizard.vue'
import DomainWizard from './views/DomainWizard.vue'
import SmtpWizard from './views/SmtpWizard.vue'
import WizardProgress from './components/WizardProgress.vue'
import ToastHost from './components/ToastHost.vue'
import ConfirmDialog from './components/ConfirmDialog.vue'
import InsecureNotice from './components/InsecureNotice.vue'

const loading = ref(true)
const initialized = ref(false)
const authed = ref(false)
const paymentStepNeeded = ref(false)
const domainStepNeeded = ref(false)
const smtpStepNeeded = ref(false)
const username = ref('')
const demoAvailable = ref(false)
const demoSession = ref(false)
const demoExpiresAt = ref('')
const now = ref(Date.now())
let clock = null

provide('demoSession', demoSession)

const demoRemaining = computed(() => {
  const seconds = Math.max(0, Math.ceil((new Date(demoExpiresAt.value).getTime() - now.value) / 1000))
  const minutes = Math.floor(seconds / 60)
  return `${String(minutes).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}`
})

// 向导「第 X / N 步」进度：固定三步流，跳过的步骤仍占位、步号不跳变。
const wizardStep = computed(() => {
  if (paymentStepNeeded.value) return 1
  if (domainStepNeeded.value) return 2
  return 3 // 邮件步（smtpStepNeeded 为真时展示）
})

// checkDomainStep 查询是否仍需展示域名步骤（收款步完成后调用）；不需要则继续查邮件步。
async function checkDomainStep() {
  try { domainStepNeeded.value = !!(await api.wizardDomain()).needed } catch (_) { domainStepNeeded.value = false }
  if (!domainStepNeeded.value) await checkSmtpStep()
}
// checkSmtpStep 查询是否仍需展示邮件步骤（域名步完成后调用）。
async function checkSmtpStep() {
  try { smtpStepNeeded.value = !!(await api.wizardSmtp()).needed } catch (_) { smtpStepNeeded.value = false }
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
		demoAvailable.value = !!s.demo_mode
    if (initialized.value) {
      try {
        const me = await api.me()
        authed.value = true
        username.value = me.username
		demoSession.value = me.role === 'demo'
		demoExpiresAt.value = me.expires_at || ''
		if (demoSession.value) {
			paymentStepNeeded.value = false
			domainStepNeeded.value = false
			smtpStepNeeded.value = false
			return
		}
        try { paymentStepNeeded.value = !!(await api.wizardPayment()).needed } catch (_) { paymentStepNeeded.value = false }
        if (!paymentStepNeeded.value) await checkDomainStep()
      } catch (e) {
        authed.value = false
      }
    }
  } finally {
    loading.value = false
  }
}

// 收款步完成 → 进入域名步（若仍需要）。
async function onPaymentStepDone() {
  paymentStepNeeded.value = false
  await checkDomainStep()
}
// 域名步完成（保存或跳过）→ 进入邮件步（若仍需要）。
async function onDomainStepDone() {
  domainStepNeeded.value = false
  await checkSmtpStep()
}
// 域名步「上一步」→ 回到收款步；收款步完成后会再回到域名步。
function onDomainBack() { paymentStepNeeded.value = true }
// 邮件步完成（保存或跳过）→ 进入后台。
function onSmtpStepDone() { smtpStepNeeded.value = false }
// 邮件步「上一步」→ 回到域名步。
function onSmtpBack() { domainStepNeeded.value = true }
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

async function doDemo() {
  busy.value = true; err.value = ''
  try {
    await api.demoSession()
    await refresh()
  } catch (e) {
    err.value = e.message
  } finally { busy.value = false }
}

async function resetDemo() {
  if (!window.confirm('清空本次演示创建的临时商品与图片？示例数据不会受影响。')) return
  try {
    await api.resetDemo()
    window.location.hash = '#/products'
    window.location.reload()
  } catch (e) { err.value = e.message }
}

async function doLogout() {
  try { await api.logout() } catch (_) { /* ignore */ }
  authed.value = false
	demoSession.value = false
  form.value = { user: '', pass: '' }
}

// 子组件遇 401 时调用，回到登录态。
provide('onUnauthorized', () => { authed.value = false })

onMounted(() => { refresh(); clock = window.setInterval(() => { now.value = Date.now() }, 1000) })
onUnmounted(() => { if (clock) window.clearInterval(clock) })
</script>

<template>
  <!-- 全局 toast 宿主 + 统一确认弹窗：始终挂载，跨所有页面状态可用 -->
  <ToastHost />
  <ConfirmDialog />
  <!-- 明文访问提示：登录页/向导/后台各态都要能看到（商家最可能在登录页就撞上） -->
  <InsecureNotice />

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
			<template v-if="demoAvailable">
				<div class="demo-divider"><span>或</span></div>
				<button class="demo-enter" :disabled="busy" @click="doDemo">一键进入公开演示</button>
				<p class="muted demo-note">无需密码 · 45 分钟独立体验 · 示例数据受保护</p>
			</template>
    </div>
  </div>

  <!-- 收款未配且未跳过：走「配置收款」向导步骤 -->
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

  <!-- 域名已配/跳过、邮件未配且未跳过：走「配置邮件」向导步骤 -->
  <template v-else-if="smtpStepNeeded">
    <header class="app-header">
      <div class="brand">Kartwo Admin · 开店向导</div>
      <button @click="doLogout">登出</button>
    </header>
    <WizardProgress :step="wizardStep" />
    <SmtpWizard @done="onSmtpStepDone" @back="onSmtpBack" />
  </template>

  <!-- 已登录：应用 -->
  <template v-else>
		<div v-if="demoSession" class="demo-banner">
			<span><strong>公开演示模式</strong>　可查看后台并新建 3 个临时草稿商品；示例商品和关键配置受保护。</span>
			<span class="demo-actions">剩余 {{ demoRemaining }} <button @click="resetDemo">重置本次演示</button></span>
		</div>
    <header class="app-header">
      <div class="brand">Kartwo Admin</div>
      <div class="row" style="gap:1rem; flex: 0;">
        <RouterLink to="/dashboard">概览</RouterLink>
				<template v-if="!demoSession">
					<RouterLink to="/diagnostics">诊断</RouterLink>
					<RouterLink to="/export">导出</RouterLink>
					<RouterLink to="/audit">审计</RouterLink>
				</template>
        <RouterLink to="/products">商品</RouterLink>
        <RouterLink to="/categories">分类</RouterLink>
        <RouterLink to="/merchandising">内容</RouterLink>
				<RouterLink v-if="!demoSession" to="/imports/csv">导入</RouterLink>
				<RouterLink v-if="!demoSession" to="/orders">订单</RouterLink>
				<details v-if="!demoSession" class="settings-menu">
          <summary>设置</summary>
          <div class="settings-submenu">
            <RouterLink to="/shop">店铺</RouterLink>
            <RouterLink to="/payment">收款</RouterLink>
            <RouterLink to="/domain">域名</RouterLink>
            <RouterLink to="/smtp">邮件</RouterLink>
            <RouterLink to="/translation">翻译</RouterLink>
            <RouterLink to="/backup">备份</RouterLink>
            <RouterLink to="/shipping">配送</RouterLink>
            <RouterLink to="/policies">店铺政策</RouterLink>
          </div>
        </details>
        <span class="muted">{{ username }}</span>
        <button @click="doLogout">登出</button>
      </div>
    </header>
    <main class="container">
      <RouterView />
    </main>
  </template>
</template>

<style scoped>
.settings-menu{position:relative}
.settings-menu summary{cursor:pointer;list-style:none;white-space:nowrap}
.settings-menu summary::-webkit-details-marker{display:none}
.settings-menu summary::after{content:'⌄';margin-left:var(--sp-1);color:var(--text-muted)}
.settings-menu[open] summary::after{content:'⌃'}
.settings-submenu{position:absolute;right:0;top:calc(100% + var(--sp-2));z-index:10;display:grid;min-width:9rem;padding:var(--sp-2);border:1px solid var(--border);border-radius:var(--radius-md);background:var(--panel);box-shadow:var(--shadow-md)}
.settings-submenu a{padding:var(--sp-2) var(--sp-3);border-radius:var(--radius-sm);white-space:nowrap}
.settings-submenu a:hover{background:var(--surface-2)}
.demo-banner{display:flex;align-items:center;justify-content:space-between;gap:var(--sp-4);padding:var(--sp-2) var(--sp-4);color:#713f12;background:#fef3c7;border-bottom:1px solid #f59e0b;font-size:var(--fs-sm)}
.demo-actions{display:flex;align-items:center;gap:var(--sp-3);white-space:nowrap;font-variant-numeric:tabular-nums}
.demo-actions button{padding:.3rem .65rem;background:#fff}
.demo-divider{display:flex;align-items:center;gap:var(--sp-3);margin:var(--sp-4) 0;color:var(--text-muted)}
.demo-divider::before,.demo-divider::after{content:'';height:1px;flex:1;background:var(--border)}
.demo-enter{width:100%;color:var(--accent);border-color:var(--accent);font-weight:600}
.demo-note{text-align:center;font-size:var(--fs-xs);margin-bottom:0}
@media(max-width:760px){.demo-banner{align-items:flex-start;flex-direction:column}.demo-actions{width:100%;justify-content:space-between}}
</style>
