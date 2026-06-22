<!-- 收款设置 / Payment Settings (Stripe + PayPal). 作者：仗键天涯(daxing) 3442535897@qq.com -->
<script setup>
import { ref, onMounted, inject } from 'vue'
import { api, APIError } from '../api.js'

const onUnauthorized = inject('onUnauthorized')
const err = ref('')
const msg = ref('')
const busy = ref(false)

// Stripe
const s = ref({ source: 'db', readonly: false, mode: 'test', publishable: '', has_secret: false, has_webhook: false, secret: '', webhook_secret: '' })
// PayPal
const p = ref({ source: 'db', readonly: false, mode: 'sandbox', client_id: '', has_secret: false, secret: '' })

async function load() {
  err.value = ''
  try {
    const r = await api.getPayment()
    Object.assign(s.value, r.stripe, { secret: '', webhook_secret: '' })
    Object.assign(p.value, r.paypal, { secret: '' })
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    err.value = e.message
  }
}

async function saveStripe() {
  await save({ stripe: { mode: s.value.mode, publishable: (s.value.publishable || '').trim(), secret: s.value.secret.trim(), webhook_secret: s.value.webhook_secret.trim() } })
}
async function savePaypal() {
  await save({ paypal: { mode: p.value.mode, client_id: (p.value.client_id || '').trim(), secret: p.value.secret.trim() } })
}
async function save(payload) {
  if (busy.value) return
  busy.value = true; err.value = ''; msg.value = ''
  try {
    const r = await api.setPayment(payload)
    Object.assign(s.value, r.stripe, { secret: '', webhook_secret: '' })
    Object.assign(p.value, r.paypal, { secret: '' })
    msg.value = '已保存，收款密钥已即时生效。'
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    err.value = e.message
  } finally { busy.value = false }
}
onMounted(load)
</script>

<template>
  <div class="container" style="max-width:720px">
    <h2>收款设置</h2>
    <p class="muted" style="max-width:64ch">密钥<strong>加密保存</strong>，绝不进日志或导出明文。<strong>默认沙箱/测试</strong>，可用测试卡 <code>4242 4242 4242 4242</code> 走通后再切正式。</p>
    <p v-if="err" class="err">{{ err }}</p>
    <p v-if="msg" class="ok">{{ msg }}</p>

    <!-- Stripe -->
    <div class="panel card" style="margin-top:1rem">
      <h3 style="margin-top:0">Stripe（信用卡）<span class="chip" :class="{ on: s.has_secret }">{{ s.has_secret ? '已配置' : '未配置' }}</span></h3>
      <p v-if="s.readonly" class="env-banner">🔒 由<strong>环境变量</strong>提供（只读）。改环境变量后重启，或清空 STRIPE_* 改用此页。</p>
      <label>模式</label>
      <select v-model="s.mode" :disabled="s.readonly">
        <option value="test">测试 / 沙箱</option><option value="live">正式</option>
      </select>
      <label>Publishable key（pk_…）</label>
      <input v-model="s.publishable" placeholder="pk_test_…" autocomplete="off" :disabled="s.readonly" />
      <label>Secret key（sk_… 或 rk_…，推荐 rk_）</label>
      <input v-model="s.secret" type="password" :placeholder="s.readonly ? '由环境变量提供' : (s.has_secret ? '已保存，留空不改' : 'sk_test_…')" autocomplete="off" :disabled="s.readonly" />
      <label>Webhook 签名密钥（whsec_…）<span class="chip" :class="{ on: s.has_webhook }">{{ s.has_webhook ? '已配置' : '未配置' }}</span></label>
      <input v-model="s.webhook_secret" type="password" :placeholder="s.readonly ? '由环境变量提供' : (s.has_webhook ? '已保存，留空不改' : 'whsec_…')" autocomplete="off" :disabled="s.readonly" />
      <div class="spacer"></div>
      <button class="primary" :disabled="busy || s.readonly" @click="saveStripe">保存 Stripe</button>
    </div>

    <!-- PayPal -->
    <div class="panel card" style="margin-top:1rem">
      <h3 style="margin-top:0">PayPal<span class="chip" :class="{ on: p.has_secret }">{{ p.has_secret ? '已配置' : '未配置' }}</span></h3>
      <p v-if="p.readonly" class="env-banner">🔒 由<strong>环境变量</strong>提供（只读）。改环境变量后重启，或清空 PAYPAL_* 改用此页。</p>
      <p class="muted" style="font-size:.82rem">在 PayPal 开发者后台「My Apps」创建应用，复制 Client ID 与 Secret（同一沙箱/正式账号）。</p>
      <label>模式</label>
      <select v-model="p.mode" :disabled="p.readonly">
        <option value="sandbox">沙箱</option><option value="live">正式</option>
      </select>
      <label>Client ID</label>
      <input v-model="p.client_id" placeholder="AY…" autocomplete="off" :disabled="p.readonly" />
      <label>Secret</label>
      <input v-model="p.secret" type="password" :placeholder="p.readonly ? '由环境变量提供' : (p.has_secret ? '已保存，留空不改' : 'EM…')" autocomplete="off" :disabled="p.readonly" />
      <div class="spacer"></div>
      <button class="primary" :disabled="busy || p.readonly" @click="savePaypal">保存 PayPal</button>
    </div>
  </div>
</template>

<style scoped>
.chip{font-size:.72rem;border:1px solid var(--line);border-radius:999px;padding:.05rem .5rem;color:var(--muted);margin-left:.5rem}
.chip.on{color:#03263a;background:var(--accent);border-color:var(--accent)}
.panel.card label{display:block;margin-top:.8rem}
.panel.card input,.panel.card select{width:100%}
.env-banner{margin:.6rem 0;padding:.5rem .7rem;border:1px solid var(--line);border-radius:8px;background:var(--panel);font-size:.85rem}
</style>
