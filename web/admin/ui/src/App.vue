<!-- 应用外壳与鉴权 / App Shell & Auth
功能：后台登录、权限状态与导航布局
作者：仗键天涯(daxing)
邮箱：3442535897@qq.com
时间：2026-06-18 10:43:28 -->
<script setup>
import { ref, computed, watch, onMounted, onUnmounted, provide } from 'vue'
import { useRoute } from 'vue-router'
import { api, APIError } from './api.js'
import { useToast } from './toast.js'
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
const accountRole = ref('')
const permissions = ref([])
const demoAvailable = ref(false)
const demoSession = ref(false)
const demoExpiresAt = ref('')
const now = ref(Date.now())
const route = useRoute()
const toast = useToast()
let clock = null
const mobileNavOpen = ref(false)
const expandedGroups = ref({ business: true, catalog: false, data: false, system: false, settings: false })

provide('demoSession', demoSession)
provide('accountRole', accountRole)
provide('accountPermissions', permissions)

const inviteToken = computed(() => route.path === '/accept-invite' ? String(route.query.token || '') : '')
const inviteForm = ref({ password: '', confirm: '' })
const inviteDone = ref(false)
const accessByPath = { '/dashboard': 'dashboard.view', '/diagnostics': 'diagnostics.view', '/audit': 'audit.view', '/products': 'products.view', '/categories': 'products.view', '/merchandising': 'content.manage', '/blog': 'blog.view', '/blog-settings': 'blog.view', '/imports/csv': 'imports.manage', '/orders': 'orders.view', '/contact': 'contact.view', '/contact-settings': 'contact.view', '/shop': 'settings.shop', '/domain': 'settings.domain', '/translation': 'translation.manage', '/shipping': 'settings.shipping', '/policies': 'settings.policy' }
function can(permission) { return accountRole.value === 'owner' || accountRole.value === 'demo' || permissions.value.includes(permission) }
const navGroups = computed(() => [
  { id: 'business', label: '经营', icon: '▤', items: [
    { path: '/orders', label: '订单', visible: can('orders.view') },
    { path: '/contact', label: '客服留言', visible: can('contact.view') },
  ] },
  { id: 'catalog', label: '商品与内容', icon: '▣', items: [
    { path: '/products', label: '商品', visible: can('products.view') },
    { path: '/categories', label: '分类', visible: can('products.view') },
    { path: '/merchandising', label: '内容', visible: can('content.manage') },
    { path: '/blog', label: '博客文章', visible: can('blog.view') },
  ] },
  { id: 'data', label: '数据', icon: '⇄', items: [
    { path: '/imports/csv', label: '导入', visible: can('imports.manage') },
    { path: '/export', label: '导出', visible: accountRole.value !== 'staff' },
  ] },
  { id: 'system', label: '系统', icon: '◷', items: [
    { path: '/diagnostics', label: '诊断', visible: can('diagnostics.view') },
    { path: '/audit', label: '日志', visible: can('audit.view') },
    { path: '/about', label: '关于', visible: true },
  ] },
  { id: 'settings', label: '设置', icon: '⚙', items: [
    { path: '/shop', label: '店铺', visible: can('settings.shop') },
    { path: '/payment', label: '收款', visible: accountRole.value !== 'staff' },
    { path: '/domain', label: '域名', visible: can('settings.domain') },
    { path: '/smtp', label: '邮件', visible: accountRole.value !== 'staff' },
    { path: '/translation', label: '翻译', visible: can('translation.manage') },
    { path: '/backup', label: '备份', visible: accountRole.value !== 'staff' },
    { path: '/shipping', label: '配送', visible: can('settings.shipping') },
    { path: '/policies', label: '店铺政策', visible: can('settings.policy') },
    { path: '/contact-settings', label: '客服留言设置', visible: can('contact.view') },
    { path: '/blog-settings', label: '博客设置', visible: accountRole.value !== 'staff' },
    { path: '/account', label: '账号与安全', visible: true },
    { path: '/users', label: '用户与角色', visible: accountRole.value === 'owner' },
  ] },
].map(group => ({ ...group, items: group.items.filter(item => item.visible) })).filter(group => group.items.length))
function navItemActive(path) {
  return route.path === path || (path === '/products' && route.path.startsWith('/products/')) || (path === '/orders' && route.path.startsWith('/orders/'))
}
const currentNav = computed(() => {
  if (route.path === '/dashboard') return { group: '', label: '概览' }
  const group = navGroups.value.find(section => section.items.some(item => navItemActive(item.path)))
  const item = group?.items.find(entry => navItemActive(entry.path))
  if (route.path === '/products/new') return { group: group?.label || '商品与内容', label: '新建商品' }
  if (route.path.startsWith('/products/')) return { group: group?.label || '商品与内容', label: '编辑商品' }
  if (route.path.startsWith('/orders/')) return { group: group?.label || '经营', label: '订单详情' }
  return { group: group?.label || '后台', label: item?.label || '概览' }
})
watch([() => route.path, navGroups], () => {
  mobileNavOpen.value = false
  const group = navGroups.value.find(section => section.items.some(item => navItemActive(item.path)))
  if (group) expandedGroups.value[group.id] = true
}, { immediate: true })
function closeMobileNavOnEscape(event) { if (event.key === 'Escape') mobileNavOpen.value = false }
const routePermission = computed(() => route.path === '/products/new' ? 'products.create' : route.path.startsWith('/products/') ? 'products.view' : route.path.startsWith('/orders/') ? 'orders.view' : accessByPath[route.path])
const routeAllowed = computed(() => accountRole.value === 'demo' ? route.path !== '/users' : accountRole.value !== 'staff' || (!routePermission.value ? ['/account', '/about'].includes(route.path) : can(routePermission.value)))
const staffReadonlyRoute = computed(() => accountRole.value === 'staff' && (
  (route.path.startsWith('/products/') && !can('products.edit')) ||
  (route.path === '/products' && !can('products.edit') && !can('products.create')) ||
  (route.path.startsWith('/orders') && !can('orders.fulfill')) ||
  (route.path === '/categories' && !can('categories.manage')) ||
  (route.path === '/contact' && !can('contact.manage'))
))

const demoRemaining = computed(() => {
  const seconds = Math.max(0, Math.ceil((new Date(demoExpiresAt.value).getTime() - now.value) / 1000))
  const minutes = Math.floor(seconds / 60)
  return `${String(minutes).padStart(2, '0')}:${String(seconds % 60).padStart(2, '0')}`
})
const demoReadonlyRoute = computed(() => demoSession.value && !!route.meta.demoReadonly)

function guardDemoReadonly(event) {
	if (!demoReadonlyRoute.value && !staffReadonlyRoute.value) return
	const control = event.target.closest('button, input, textarea, select')
	if (!control || control.closest('[data-demo-view-control]')) return
	if (event.type === 'keydown' && event.key === 'Tab') return
	event.preventDefault()
	event.stopPropagation()
	if (event.type === 'click') toast.error('当前账号在此页面没有修改权限。')
}

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
		accountRole.value = me.role || ''
		permissions.value = me.permissions || []
		demoExpiresAt.value = me.expires_at || ''
		if (demoSession.value || accountRole.value === 'staff') {
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

async function acceptInvite() {
  if (inviteForm.value.password.length < 8) return toast.error('密码至少 8 位。')
  if (inviteForm.value.password !== inviteForm.value.confirm) return toast.error('两次输入的密码不一致。')
  busy.value = true; err.value = ''
  try { await api.acceptInvite(inviteToken.value, inviteForm.value.password); inviteDone.value = true; toast.success('密码已设置，可以登录。') }
  catch (e) { err.value = e.message } finally { busy.value = false }
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

onMounted(() => { refresh(); clock = window.setInterval(() => { now.value = Date.now() }, 1000); window.addEventListener('keydown', closeMobileNavOnEscape) })
onUnmounted(() => { if (clock) window.clearInterval(clock); window.removeEventListener('keydown', closeMobileNavOnEscape) })
</script>

<template>
  <!-- 全局 toast 宿主 + 统一确认弹窗：始终挂载，跨所有页面状态可用 -->
  <ToastHost />
  <ConfirmDialog />
  <!-- 明文访问提示：登录页/向导/后台各态都要能看到（商家最可能在登录页就撞上） -->
  <InsecureNotice />

  <div v-if="loading" class="center-screen muted">加载中…</div>

  <div v-else-if="inviteToken" class="center-screen"><div class="panel card"><h2>设置员工密码</h2><p class="muted">此链接一次有效，请设置至少 8 位的独立登录密码。</p><template v-if="!inviteDone"><label>新密码</label><input v-model="inviteForm.password" type="password" autocomplete="new-password" /><label>确认密码</label><input v-model="inviteForm.confirm" type="password" autocomplete="new-password" @keyup.enter="acceptInvite" /><p v-if="err" class="err">{{ err }}</p><button class="primary" :disabled="busy" @click="acceptInvite">设置密码</button></template><RouterLink v-else to="/dashboard">返回登录</RouterLink></div></div>

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
  <div v-else-if="!authed" class="center-screen login-screen">
    <div class="panel card login-card">
      <div class="login-brand"><span class="brand-mark" aria-hidden="true">K</span><span>Kartwo</span></div>
      <p class="login-overline">KARTWO · STORE ADMIN</p>
      <h1>欢迎回来，<br />继续经营你的店铺。</h1>
      <p class="login-intro">登录后台，管理商品、订单和店面内容。</p>
      <label for="login-user">用户名</label>
      <input id="login-user" v-model="form.user" autocomplete="username" />
      <label for="login-password">口令</label>
      <input id="login-password" v-model="form.pass" type="password" autocomplete="current-password" @keyup.enter="doLogin" />
      <p v-if="err" class="err">{{ err }}</p>
      <div class="spacer"></div>
      <button class="primary login-submit" :disabled="busy" @click="doLogin">登录后台</button>
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
    <div class="admin-layout">
      <button v-if="mobileNavOpen" class="nav-backdrop" type="button" aria-label="关闭菜单" @click="mobileNavOpen = false"></button>
      <aside id="admin-sidebar" class="admin-sidebar" :class="{ 'is-open': mobileNavOpen }" aria-label="后台导航">
        <div class="sidebar-brand"><span class="brand-mark" aria-hidden="true">K</span><span class="sidebar-brand-copy"><strong>Kartwo</strong><small>店铺运营后台</small></span><button class="sidebar-close" type="button" aria-label="关闭菜单" @click="mobileNavOpen = false">×</button></div>
        <nav class="side-nav">
          <div class="side-label">工作台</div>
          <RouterLink v-if="can('dashboard.view')" to="/dashboard" class="nav-home" :class="{ 'is-active': route.path === '/dashboard' }" :aria-current="route.path === '/dashboard' ? 'page' : undefined" @click="mobileNavOpen = false"><span class="nav-icon" aria-hidden="true">⌂</span>概览</RouterLink>
          <div class="side-label side-label-menu">功能菜单</div>
          <section v-for="group in navGroups" :key="group.id" class="nav-section">
            <button class="nav-group" :class="{ 'has-active': group.items.some(item => navItemActive(item.path)) }" type="button" :aria-expanded="!!expandedGroups[group.id]" @click="expandedGroups[group.id] = !expandedGroups[group.id]">
              <span class="nav-group-label"><span class="nav-icon" aria-hidden="true">{{ group.icon }}</span>{{ group.label }}</span><span class="nav-chevron" aria-hidden="true">⌄</span>
            </button>
            <div v-if="expandedGroups[group.id]" class="nav-items">
              <RouterLink v-for="item in group.items" :key="item.path" :to="item.path" class="nav-item" :class="{ 'is-active': navItemActive(item.path) }" :aria-current="navItemActive(item.path) ? 'page' : undefined" @click="mobileNavOpen = false">{{ item.label }}</RouterLink>
            </div>
          </section>
        </nav>
      </aside>
      <div class="admin-workspace">
        <header class="app-header admin-header">
          <div class="admin-header-start">
            <button class="nav-toggle" type="button" :aria-expanded="mobileNavOpen" aria-controls="admin-sidebar" aria-label="切换菜单" @click="mobileNavOpen = !mobileNavOpen">☰</button>
            <div class="page-location"><span class="page-parent">{{ currentNav.group || '工作台' }}</span><strong>{{ currentNav.label }}</strong></div>
          </div>
          <div class="admin-header-end"><span class="account-avatar" aria-hidden="true">{{ username.slice(0, 1).toUpperCase() }}</span><span class="account-name">{{ username }}</span><button @click="doLogout">登出</button></div>
        </header>
        <main class="container admin-content" :class="{ 'demo-readonly-route': demoReadonlyRoute || staffReadonlyRoute }" @click.capture="guardDemoReadonly" @keydown.capture="guardDemoReadonly">
          <div v-if="demoReadonlyRoute" class="demo-readonly-notice">🔒 公开演示只读预览：敏感信息已隐藏，修改、导入、导出及订单操作均不可执行。</div>
          <div v-if="!routeAllowed" class="panel card">当前账号没有访问此页面的权限。</div>
          <RouterView v-else />
        </main>
      </div>
    </div>
  </template>
</template>

<style scoped>
.login-screen{padding:var(--sp-6);background:radial-gradient(circle at 76% 14%,var(--login-glow),transparent 34%),linear-gradient(135deg,var(--login-start),var(--login-middle) 58%,var(--login-end))}
.login-card{width:min(460px,100%);max-width:none;padding:var(--sp-8);border:0;border-radius:calc(var(--radius-lg) + 8px);box-shadow:var(--shadow-login)}
.login-brand{display:flex;align-items:center;gap:var(--sp-3);font-size:var(--fs-lg);font-weight:700}
.login-overline{margin:var(--sp-8) 0 var(--sp-3);color:var(--accent);font-size:var(--fs-xs);font-weight:700;letter-spacing:.16em}
.login-card h1{margin:0;font-size:clamp(1.7rem,4vw,2.15rem);line-height:1.25;letter-spacing:-.025em}
.login-intro{margin:var(--sp-3) 0 var(--sp-6);color:var(--text-muted);line-height:1.6}
.login-card label{margin-top:var(--sp-4)}
.login-card input{min-height:44px;background:var(--surface-2)}
.login-submit{width:100%;min-height:46px}
.admin-layout{display:grid;grid-template-columns:244px minmax(0,1fr);min-height:100vh}
.admin-sidebar{position:sticky;top:0;align-self:start;height:100vh;min-width:0;background:var(--sidebar-bg);color:var(--sidebar-text);padding:var(--sp-6) var(--sp-3);display:flex;flex-direction:column}
.sidebar-brand{display:flex;align-items:center;gap:var(--sp-3);padding:0 var(--sp-3) var(--sp-6);font-size:var(--fs-lg);white-space:nowrap}
.sidebar-brand-copy{display:grid;gap:var(--sp-1);line-height:1.1}
.sidebar-brand-copy small{color:var(--sidebar-subtle);font-size:var(--fs-xs);font-weight:400}
.brand-mark{display:grid;place-items:center;width:42px;height:42px;flex:none;border-radius:var(--radius-lg);background:var(--accent);color:var(--on-accent);font-size:var(--fs-lg);font-weight:700}
.side-nav{min-height:0;overflow-y:auto;padding:0 var(--sp-1);scrollbar-width:thin}
.side-label{padding:var(--sp-2) var(--sp-3);color:var(--sidebar-subtle);font-size:var(--fs-xs);font-weight:700;letter-spacing:.1em}
.side-label-menu{margin-top:var(--sp-4);padding-top:var(--sp-4);border-top:1px solid var(--sidebar-border)}
.nav-home{display:flex;align-items:center;gap:var(--sp-3);min-height:44px;padding:var(--sp-2) var(--sp-3);border-radius:var(--radius-md);color:var(--sidebar-muted);font-size:var(--fs-base);font-weight:600;white-space:nowrap}
.nav-home:hover{background:var(--sidebar-hover);color:var(--sidebar-text);text-decoration:none}
.nav-home.is-active{background:var(--sidebar-active);color:var(--on-accent)}
.nav-section+.nav-section{margin-top:var(--sp-1)}
.nav-group{width:100%;display:flex;align-items:center;justify-content:space-between;min-height:44px;padding:var(--sp-2) var(--sp-3);border:0;border-radius:var(--radius-md);box-shadow:none;background:transparent;color:var(--sidebar-muted);font-size:var(--fs-base);font-weight:600;text-align:left}
.nav-group:hover,.nav-group[aria-expanded="true"]{background:var(--sidebar-hover);border:0;color:var(--sidebar-text)}
.nav-group.has-active{color:var(--sidebar-text)}
.nav-group-label{display:flex;align-items:center;gap:var(--sp-3)}
.nav-icon{display:inline-flex;align-items:center;justify-content:center;width:20px;font-size:var(--fs-lg);font-weight:400;line-height:1}
.nav-chevron{font-size:var(--fs-md);transition:transform .15s ease}
.nav-group[aria-expanded="true"] .nav-chevron{transform:rotate(180deg)}
.nav-items{display:grid;gap:var(--sp-1);padding:var(--sp-1) 0 var(--sp-2) var(--sp-3)}
.nav-item{display:block;min-height:38px;padding:var(--sp-2) var(--sp-3) var(--sp-2) var(--sp-8);border-radius:var(--radius-sm);color:var(--sidebar-muted);font-size:var(--fs-sm);white-space:nowrap}
.nav-item:hover{background:var(--sidebar-hover);color:var(--sidebar-text);text-decoration:none}
.nav-item.is-active{background:var(--sidebar-active);color:var(--on-accent);font-weight:600}
.nav-home:focus-visible,.nav-group:focus-visible,.nav-item:focus-visible{outline:2px solid var(--sidebar-focus);outline-offset:-2px}
.admin-workspace{min-width:0}
.admin-header{min-height:86px;gap:var(--sp-4);border-bottom:0;background:transparent;padding:var(--sp-6) clamp(var(--sp-6),3vw,var(--sp-8)) var(--sp-2);box-shadow:none}
.admin-header-start,.admin-header-end{display:flex;align-items:center;gap:var(--sp-3);min-width:0}
.page-location{display:grid;gap:var(--sp-1);min-width:0;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.page-location strong{font-size:var(--fs-xl);line-height:1.2;letter-spacing:-.025em}
.page-parent{color:var(--accent);font-size:var(--fs-xs);font-weight:700;letter-spacing:.1em}
.account-avatar{display:grid;place-items:center;width:36px;height:36px;flex:none;border-radius:var(--radius-pill);background:var(--intro-end);color:var(--accent);font-weight:700}
.account-name{color:var(--text-muted);max-width:12rem;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.admin-content{width:100%;max-width:1500px;margin:0 auto;padding:var(--sp-4) clamp(var(--sp-6),3vw,var(--sp-8)) var(--sp-8);min-width:0}
.nav-toggle,.nav-backdrop,.sidebar-close{display:none}
.demo-banner{display:flex;align-items:center;justify-content:space-between;gap:var(--sp-4);padding:var(--sp-2) var(--sp-4);color:#713f12;background:#fef3c7;border-bottom:1px solid #f59e0b;font-size:var(--fs-sm)}
.demo-actions{display:flex;align-items:center;gap:var(--sp-3);white-space:nowrap;font-variant-numeric:tabular-nums}
.demo-actions button{padding:.3rem .65rem;background:#fff}
.demo-divider{display:flex;align-items:center;gap:var(--sp-3);margin:var(--sp-4) 0;color:var(--text-muted)}
.demo-divider::before,.demo-divider::after{content:'';height:1px;flex:1;background:var(--border)}
.demo-enter{width:100%;color:var(--accent);border-color:var(--accent);font-weight:600}
.demo-note{text-align:center;font-size:var(--fs-xs);margin-bottom:0}
.demo-readonly-notice{margin-bottom:var(--sp-4);padding:var(--sp-3);color:#713f12;background:#fffbeb;border:1px solid #f59e0b;border-radius:var(--radius-md);font-size:var(--fs-sm)}
.demo-readonly-route :is(input,textarea,select,button):not([data-demo-view-control] *){cursor:not-allowed}
@media(max-width:960px){
  .demo-banner{align-items:flex-start;flex-direction:column}.demo-actions{width:100%;justify-content:space-between}
  .admin-layout{display:block}.admin-header{min-height:74px}.admin-header-start,.admin-header-end{gap:var(--sp-2)}
  .admin-header-start{min-width:0}.account-name{display:none}
  .nav-toggle{display:block;padding:var(--sp-1) var(--sp-2);box-shadow:none;font-size:var(--fs-lg)}
  .admin-sidebar{position:fixed;z-index:30;top:0;bottom:0;left:0;width:min(280px,85vw);visibility:hidden;transform:translateX(-100%);transition:transform .2s ease,visibility .2s ease;box-shadow:var(--shadow-md)}
  .admin-sidebar.is-open{visibility:visible;transform:translateX(0)}
  .sidebar-close{display:block;margin-left:auto;padding:0 var(--sp-2);border:0;box-shadow:none;background:transparent;color:var(--sidebar-text);font-size:var(--fs-lg)}
  .sidebar-close:hover{background:var(--sidebar-hover);color:var(--sidebar-text)}
  .side-nav{max-height:calc(100vh - 90px)}
  .nav-backdrop,.nav-backdrop:hover{display:block;position:fixed;z-index:29;inset:0;width:100%;height:100%;padding:0;border:0;border-radius:0;background:var(--nav-overlay)}
  .admin-content{padding:var(--sp-4)}
}
@media(max-width:520px){.account-avatar{display:none}.admin-header{gap:var(--sp-2);padding:var(--sp-4)}.page-location strong{font-size:var(--fs-lg)}.login-card{padding:var(--sp-6)}}
</style>
