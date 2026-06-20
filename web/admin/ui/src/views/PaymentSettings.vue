<!-- 收款设置 / Payment Settings (Stripe). 作者：仗键天涯(daxing) 3442535897@qq.com -->
<script setup>
import { ref, onMounted, inject } from 'vue'
import { api, APIError } from '../api.js'

const onUnauthorized = inject('onUnauthorized')
const mode = ref('test')
const publishable = ref('')
const secret = ref('')
const webhookSecret = ref('')
const hasSecret = ref(false)
const hasWebhook = ref(false)
const err = ref('')
const msg = ref('')
const busy = ref(false)

async function load() {
  err.value = ''
  try {
    const p = await api.getPayment()
    mode.value = p.mode || 'test'
    publishable.value = p.publishable || ''
    hasSecret.value = !!p.has_secret
    hasWebhook.value = !!p.has_webhook
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    err.value = e.message
  }
}

async function save() {
  if (busy.value) return
  busy.value = true; err.value = ''; msg.value = ''
  try {
    // 留空的 sk/whsec 表示「保持原值」，不必每次重输。
    const r = await api.setPayment({
      mode: mode.value,
      publishable: publishable.value.trim(),
      secret: secret.value.trim(),
      webhook_secret: webhookSecret.value.trim(),
    })
    hasSecret.value = !!r.has_secret
    hasWebhook.value = !!r.has_webhook
    secret.value = ''
    webhookSecret.value = ''
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
    <h2>收款设置（Stripe）</h2>
    <p class="muted" style="max-width:64ch">
      在 Stripe 后台「开发者 → API 密钥」复制以下密钥填入。<strong>默认沙箱（测试模式）</strong>，可放心用测试卡
      <code>4242 4242 4242 4242</code> 走通整条流程后再切正式。密钥<strong>加密保存</strong>，绝不进日志或导出明文。
    </p>
    <p v-if="err" class="err">{{ err }}</p>
    <p v-if="msg" class="ok">{{ msg }}</p>

    <div class="panel card" style="margin-top:1rem">
      <label>模式</label>
      <select v-model="mode">
        <option value="test">测试 / 沙箱（推荐先用）</option>
        <option value="live">正式（真实收款）</option>
      </select>

      <label>可发布密钥（Publishable key，pk_…）</label>
      <input v-model="publishable" placeholder="pk_test_…" autocomplete="off" />

      <label>
        密钥（Secret key，sk_…）
        <span class="chip" :class="{ on: hasSecret }">{{ hasSecret ? '已配置' : '未配置' }}</span>
      </label>
      <input v-model="secret" type="password" :placeholder="hasSecret ? '已保存，留空表示不修改' : 'sk_test_…'" autocomplete="off" />

      <label>
        Webhook 签名密钥（whsec_…）
        <span class="chip" :class="{ on: hasWebhook }">{{ hasWebhook ? '已配置' : '未配置' }}</span>
      </label>
      <input v-model="webhookSecret" type="password" :placeholder="hasWebhook ? '已保存，留空表示不修改' : 'whsec_…'" autocomplete="off" />
      <p class="muted" style="font-size:.82rem">
        本地测试用 Stripe CLI 转发时，<code>stripe listen</code> 会打印一个 whsec；正式上线用 Stripe 后台「Webhooks」端点的签名密钥。两者填这里即可，按需替换。
      </p>

      <div class="spacer"></div>
      <button class="primary" :disabled="busy" @click="save">保存收款设置</button>
    </div>
  </div>
</template>

<style scoped>
.chip{font-size:.72rem;border:1px solid var(--line);border-radius:999px;padding:.05rem .5rem;color:var(--muted);margin-left:.4rem}
.chip.on{color:#03263a;background:var(--accent);border-color:var(--accent)}
.panel.card label{display:block;margin-top:.8rem}
.panel.card input,.panel.card select{width:100%}
</style>
