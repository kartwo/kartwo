<!-- 自动备份设置 / Backup Settings. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<!-- 功能：保存本地自动备份周期和保留份数；环境变量覆盖时对应字段只读，保存后下次重启生效 -->
<script setup>
import { inject, onMounted, ref } from 'vue'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'

const onUnauthorized = inject('onUnauthorized', null)
const toast = useToast()
const busy = ref(false)
const loaded = ref(false)
const f = ref({ interval: '24h', retention: '7' })
const fWebDAV = ref({
  webdav_enabled: false,
  webdav_url: '',
  webdav_path: '/',
  webdav_username: '',
  webdav_password: '',
})
const state = ref({
  interval_readonly: false,
  retention_readonly: false,
  webdav_enabled_readonly: false,
  webdav_url_readonly: false,
  webdav_path_readonly: false,
  webdav_username_readonly: false,
  webdav_password_readonly: false,
  webdav_password_set: false,
  webdav_username: '',
})
const testing = ref(false)

async function load() {
  try {
    const d = await api.getBackupSettings()
    f.value = { interval: d.interval || '24h', retention: String(d.retention || 7) }
    fWebDAV.value = {
      webdav_enabled: !!(d.webdav_enabled === true || String(d.webdav_enabled) === 'true'),
      webdav_url: d.webdav_url || '',
      webdav_path: d.webdav_path || '/',
      webdav_username: d.webdav_username || '',
      webdav_password: '',
    }
    state.value = d
  } catch (e) {
    if (e instanceof APIError && e.status === 401 && onUnauthorized) return onUnauthorized()
    toast.error(e.message)
  } finally { loaded.value = true }
}

async function save() {
  if (busy.value) return
  busy.value = true
  try {
    const d = await api.setBackupSettings({
      ...f.value,
      ...fWebDAV.value,
      webdav_password: fWebDAV.value.webdav_password || undefined,
    })
    state.value = d
    state.value.webdav_password_set = !!(fWebDAV.value.webdav_password || state.value.webdav_password_set)
    fWebDAV.value.webdav_password = ''
    toast.success('自动备份设置已保存；本地策略重启后生效')
  } catch (e) {
    if (e instanceof APIError && e.status === 401 && onUnauthorized) return onUnauthorized()
    toast.error(e.message)
  } finally { busy.value = false }
}
onMounted(load)

async function testConnection() {
  if (testing.value) return
  if (!fWebDAV.value.webdav_enabled) {
    toast.error('请先启用 WebDAV 异地备份')
    return
  }
  if (!fWebDAV.value.webdav_url || !fWebDAV.value.webdav_path) {
    toast.error('请先填写 WebDAV 地址与目录')
    return
  }
  if (!state.value.webdav_password_set && !fWebDAV.value.webdav_password) {
    toast.error('请先填写 WebDAV 密码')
    return
  }
  testing.value = true
  try {
    const d = await api.setBackupSettings({
      ...f.value,
      ...fWebDAV.value,
      webdav_password: fWebDAV.value.webdav_password || undefined,
    })
    state.value = d
    state.value.webdav_password_set = !!(fWebDAV.value.webdav_password || state.value.webdav_password_set)
    fWebDAV.value.webdav_password = ''
    await api.testBackup()
    toast.success('WebDAV 测试成功：可连通并可写入')
  } catch (e) {
    if (e instanceof APIError && e.status === 401 && onUnauthorized) return onUnauthorized()
    toast.error(e.message)
  } finally {
    testing.value = false
    await load()
  }
}
</script>

<template>
  <div class="backup-page">
    <h2>自动备份</h2>
    <section v-if="loaded" class="panel card">
      <p class="muted">服务启动时会立即生成一份完整 ZIP，之后按设定周期继续备份。周期与保留份数保存后下次重启生效。</p>
      <p v-if="state.interval_readonly || state.retention_readonly" class="env-banner">由环境变量提供的字段为只读；修改环境变量后重启服务即可生效。</p>
      <label>备份周期</label>
      <input v-model="f.interval" :disabled="state.interval_readonly" placeholder="24h" autocomplete="off" />
      <p class="muted hint">使用 Go duration，例如 `24h`、`90m`；最小为 `1m`。</p>
      <label>保留自动备份份数</label>
      <input v-model="f.retention" :disabled="state.retention_readonly" placeholder="7" inputmode="numeric" autocomplete="off" />
      <p class="muted hint">允许 1–365。升级前快照独立保留，不受此数字清理。</p>

      <section class="card section">
        <h3 class="webdav-title">WebDAV 异地备份</h3>
        <label>
          <input type="checkbox" v-model="fWebDAV.webdav_enabled" :disabled="state.webdav_enabled_readonly" />
          启用 WebDAV 异地备份
        </label>
        <label>WebDAV 地址（必须是 https）</label>
        <input v-model="fWebDAV.webdav_url" :disabled="state.webdav_url_readonly" placeholder="https://dav.example.com" autocomplete="off" />
        <label>WebDAV 目录（必须以 / 开头）</label>
        <input v-model="fWebDAV.webdav_path" :disabled="state.webdav_path_readonly" placeholder="/kartwo/backups" autocomplete="off" />
        <label>WebDAV 用户名（通常是账号）</label>
        <input v-model="fWebDAV.webdav_username" :disabled="state.webdav_username_readonly" placeholder="webdav-user" autocomplete="off" />
        <label>WebDAV 密码 / 授权码</label>
        <input v-model="fWebDAV.webdav_password" type="password" autocomplete="off" :disabled="state.webdav_password_readonly"
               :placeholder="state.webdav_password_set ? '已配置，留空保持不变' : '请输入密码'" />
        <div class="row webdav-actions">
          <button :disabled="busy" @click="save">{{ busy ? '保存中…' : '保存 WebDAV 配置' }}</button>
          <button style="flex:0" :disabled="testing" @click="testConnection">{{ testing ? '测试中…' : '测试连接' }}</button>
        </div>
        <p class="muted webdav-note">
          测试会按已保存的配置写入一个临时文件。无人值守的启动即上传仍须通过 KARTWO_BACKUP_WEBDAV_* 环境变量提供完整配置。
        </p>
      </section>

      <button v-if="!state.interval_readonly || !state.retention_readonly" class="primary" :disabled="busy" @click="save">{{ busy ? '保存中…' : '保存设置' }}</button>
    </section>
  </div>
</template>

<style scoped>
.backup-page { display: grid; gap: var(--sp-4); }
.backup-page h2 { margin: 0; }
.panel.card { max-width: 680px; }
.panel.card label { display: block; margin-top: var(--sp-3); }
.panel.card input { width: 100%; }
.hint { font-size: var(--fs-sm); margin: var(--sp-1) 0 0; }
.primary { margin-top: var(--sp-4); }
.env-banner { margin: 0; padding: var(--sp-2); border: 1px solid var(--border); border-radius: var(--radius-md); background: var(--surface-2); }
.webdav-title { margin: var(--sp-3) 0 var(--sp-2); }
.webdav-actions { flex: 0; gap: var(--sp-2); margin-top: var(--sp-3); }
.webdav-actions > button { flex: 0; }
.webdav-note { font-size: var(--fs-sm); margin: var(--sp-1) 0 0; }
</style>
