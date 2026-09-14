<!-- 管理员账号与安全 / Administrator Account & Security -->
<!-- 功能：校验当前密码后修改店主用户名或主密码；主密码变化会安全轮换加密配置 -->
<!-- 作者：仗键天涯(daxing) -->
<!-- 邮箱：3442535897@qq.com -->
<!-- 时间：2026-09-14 10:10:00 -->
<script setup>
import { inject, onMounted, ref } from 'vue'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'
import ErrorState from '../components/ErrorState.vue'

const onUnauthorized = inject('onUnauthorized')
const toast = useToast()
const loading = ref(true)
const busy = ref(false)
const err = ref('')
const readonly = ref(false)
const form = ref({ username: '', current_password: '', new_password: '', confirm_password: '' })

async function load() {
  loading.value = true
  err.value = ''
  try {
    const account = await api.getAccount()
    form.value.username = account.username || ''
    readonly.value = !!account.readonly
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    err.value = e.message
  } finally { loading.value = false }
}

async function save() {
  if (busy.value || readonly.value) return
  const username = form.value.username.trim()
  if (!username) return toast.error('请输入管理员用户名。')
  if (!form.value.current_password) return toast.error('请输入当前密码确认身份。')
  if (form.value.new_password && form.value.new_password.length < 8) return toast.error('新密码至少 8 位。')
  if (form.value.new_password !== form.value.confirm_password) return toast.error('两次输入的新密码不一致。')

  busy.value = true
  try {
    await api.updateAccount({
      username,
      current_password: form.value.current_password,
      new_password: form.value.new_password,
    })
    toast.success('账号已更新，所有后台会话已退出，请使用新凭据重新登录。')
    window.setTimeout(() => window.location.reload(), 1200)
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    toast.error(e.message)
  } finally { busy.value = false }
}

onMounted(load)
</script>

<template>
  <div class="account-page">
    <h2>账号与安全</h2>
    <p class="muted">修改后台管理员用户名或密码。保存成功后，所有已登录的后台会话都会退出。</p>
    <ErrorState v-if="err" :message="err" :retry="load" />
    <section v-else class="panel card">
      <p v-if="readonly" class="readonly-note">公开演示仅展示账号安全设置，管理员身份信息已隐藏且不可修改。</p>
      <label for="account-username">管理员用户名</label>
      <input id="account-username" v-model="form.username" maxlength="64" autocomplete="username" :disabled="loading || readonly" />

      <template v-if="!readonly">
        <label for="account-current-password">当前密码</label>
        <input id="account-current-password" v-model="form.current_password" type="password" autocomplete="current-password" />
        <p class="muted hint">修改用户名或密码都必须输入当前密码。</p>

        <label for="account-new-password">新密码（可选）</label>
        <input id="account-new-password" v-model="form.new_password" type="password" minlength="8" autocomplete="new-password" />
        <p class="muted hint">留空表示只修改用户名；新密码至少 8 位。</p>

        <label for="account-confirm-password">确认新密码</label>
        <input id="account-confirm-password" v-model="form.confirm_password" type="password" autocomplete="new-password" @keyup.enter="save" />

        <div class="security-warning">
          <strong>请妥善保存新密码。</strong>
          <span>它同时保护后台登录和收款、邮件等加密配置；遗忘后无法恢复这些加密凭据。</span>
        </div>
        <button class="primary" :disabled="loading || busy" @click="save">{{ busy ? '正在安全更新…' : '保存并退出所有会话' }}</button>
      </template>
    </section>
  </div>
</template>

<style scoped>
.account-page{max-width:720px}.account-page h2{margin-bottom:0}.panel{display:grid;gap:var(--sp-2)}
.panel label{margin-top:var(--sp-3);font-weight:700}.panel input{width:100%}.hint{margin:0;font-size:var(--fs-sm)}
.readonly-note,.security-warning{padding:var(--sp-3);border:1px solid var(--border);border-radius:var(--radius-md);background:var(--surface-2)}
.readonly-note{margin:0}.security-warning{display:grid;gap:var(--sp-1);margin:var(--sp-3) 0;color:var(--text-muted)}
.security-warning strong{color:var(--text)}.primary{justify-self:start}
</style>
