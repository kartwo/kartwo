<!-- SMTP 设置 / SMTP Settings. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<!-- 功能：录入 SMTP（password 加密存/其余明文）+ 发送测试邮件；env 覆盖只读；向导邮件步与后台 /smtp 页共用 -->
<script setup>
import { ref, onMounted, inject } from 'vue'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'

const onUnauthorized = inject('onUnauthorized', null)
const toast = useToast()

const source = ref('db')       // env | db
const readonly = ref(false)    // env 覆盖 → 只读
const hasPassword = ref(false)
const configured = ref(false)
const busy = ref(false)
const testTo = ref('')
const testing = ref(false)

const f = ref({ host: '', port: '587', username: '', password: '', from_address: '', from_name: '', encryption: 'starttls' })

async function load() {
  try {
    const d = await api.getSmtp()
    source.value = d.source
    readonly.value = !!d.readonly
    hasPassword.value = !!d.has_password
    configured.value = !!d.configured
    f.value = {
      host: d.host || '', port: d.port || '587', username: d.username || '',
      password: '', from_address: d.from_address || '', from_name: d.from_name || '',
      encryption: d.encryption || 'starttls',
    }
  } catch (e) {
    if (e instanceof APIError && e.status === 401 && onUnauthorized) return onUnauthorized()
    toast.error(e.message)
  }
}

async function save() {
  if (busy.value || readonly.value) return
  busy.value = true
  try {
    const d = await api.setSmtp({ ...f.value })
    configured.value = !!d.configured
    hasPassword.value = !!d.has_password
    f.value.password = ''
    toast.success('SMTP 配置已保存')
  } catch (e) {
    if (e instanceof APIError && e.status === 401 && onUnauthorized) return onUnauthorized()
    toast.error(e.message)
  } finally { busy.value = false }
}

async function sendTest() {
  if (testing.value) return
  const to = (testTo.value || '').trim()
  if (!to.includes('@')) { toast.error('请填写合法的收件邮箱'); return }
  testing.value = true
  try {
    await api.smtpTest(to)
    toast.success('测试邮件已发送，请查收（含垃圾箱）')
  } catch (e) {
    if (e instanceof APIError && e.status === 401 && onUnauthorized) return onUnauthorized()
    toast.error(e.message)
  } finally { testing.value = false }
}
onMounted(load)
</script>

<template>
  <div class="panel card">
    <h3 style="margin-top:0">
      发信邮箱（SMTP）
      <span class="chip" :class="{ on: configured }">{{ configured ? '已配置' : '未配置' }}</span>
    </h3>

    <p v-if="readonly" class="env-banner">🔒 由<strong>环境变量</strong>提供（只读）。改环境变量后重启，或清空 SMTP_* 改用此页。</p>
    <p class="muted" style="font-size:.85rem;max-width:64ch">
      用于给顾客发订单确认信。填你的邮箱服务商 SMTP（如 Gmail、腾讯企业邮、或 Mailtrap/MailHog 等测试服务）。
      <strong>密码加密保存</strong>，绝不进日志或导出明文。
    </p>

    <label>SMTP 主机</label>
    <input v-model="f.host" placeholder="smtp.example.com" autocomplete="off" :disabled="readonly" />
    <div class="row" style="gap:.8rem">
      <div>
        <label>端口</label>
        <input v-model="f.port" placeholder="587" autocomplete="off" :disabled="readonly" />
      </div>
      <div>
        <label>加密方式</label>
        <select v-model="f.encryption" :disabled="readonly">
          <option value="starttls">STARTTLS（587，常用）</option>
          <option value="tls">SSL/TLS（465）</option>
          <option value="none">无（25，不安全）</option>
        </select>
      </div>
    </div>
    <label>用户名（通常是完整邮箱；无鉴权服务器可留空）</label>
    <input v-model="f.username" placeholder="you@example.com" autocomplete="off" :disabled="readonly" />
    <label>密码 / 授权码</label>
    <input v-model="f.password" type="password" autocomplete="off" :disabled="readonly"
           :placeholder="readonly ? '由环境变量提供' : (hasPassword ? '已保存，留空不改' : '邮箱 SMTP 授权码')" />
    <label>发件地址（From）</label>
    <input v-model="f.from_address" placeholder="shop@example.com" autocomplete="off" :disabled="readonly" />
    <label>发件人名称（可选）</label>
    <input v-model="f.from_name" placeholder="My Shop" autocomplete="off" :disabled="readonly" />

    <div class="row" style="flex:0;gap:.6rem;margin-top:.8rem">
      <button v-if="!readonly" class="primary" :disabled="busy" @click="save">保存</button>
    </div>

    <!-- 测试发信：用已保存配置发一封 -->
    <div class="test-box">
      <label>发送测试邮件到</label>
      <div class="row" style="gap:.6rem">
        <input v-model="testTo" placeholder="test@example.com" autocomplete="off" />
        <button style="flex:0" :disabled="testing || !configured" @click="sendTest">
          {{ testing ? '发送中…' : '发送测试邮件' }}
        </button>
      </div>
      <p class="muted" style="font-size:.8rem;margin:.4rem 0 0">
        用<strong>已保存</strong>的配置发送，请先保存再测试。收到即说明 SMTP 可用。
      </p>
    </div>
  </div>
</template>

<style scoped>
.chip { font-size: var(--fs-xs); border: 1px solid var(--border); border-radius: var(--radius-pill); padding: .05rem .5rem; color: var(--text-muted); margin-left: .5rem; }
.chip.on { color: var(--on-accent); background: var(--accent); border-color: var(--accent); }
.panel.card label { display: block; margin-top: .8rem; }
.panel.card input, .panel.card select { width: 100%; }
.env-banner { margin: .6rem 0; padding: .5rem .7rem; border: 1px solid var(--border); border-radius: var(--radius-md); background: var(--surface-2); font-size: var(--fs-sm); }
.test-box { margin-top: 1.2rem; padding-top: 1rem; border-top: 1px solid var(--border); }
</style>
