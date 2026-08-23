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
const state = ref({ interval_readonly: false, retention_readonly: false })

async function load() {
  try {
    const d = await api.getBackupSettings()
    f.value = { interval: d.interval || '24h', retention: String(d.retention || 7) }
    state.value = d
  } catch (e) {
    if (e instanceof APIError && e.status === 401 && onUnauthorized) return onUnauthorized()
    toast.error(e.message)
  } finally { loaded.value = true }
}

async function save() {
  if (busy.value || (state.value.interval_readonly && state.value.retention_readonly)) return
  busy.value = true
  try {
    const d = await api.setBackupSettings({ ...f.value })
    state.value = d
    toast.success('自动备份设置已保存，重启服务后生效')
  } catch (e) {
    if (e instanceof APIError && e.status === 401 && onUnauthorized) return onUnauthorized()
    toast.error(e.message)
  } finally { busy.value = false }
}
onMounted(load)
</script>

<template>
  <div class="backup-page">
    <h2>自动备份</h2>
    <section v-if="loaded" class="panel card">
      <p class="muted">服务启动时会立即生成一份完整 ZIP，之后按设定周期继续备份。保存后的设置将在下次重启服务时生效。</p>
      <p v-if="state.interval_readonly || state.retention_readonly" class="env-banner">由环境变量提供的字段为只读；修改环境变量后重启服务即可生效。</p>
      <label>备份周期</label>
      <input v-model="f.interval" :disabled="state.interval_readonly" placeholder="24h" autocomplete="off" />
      <p class="muted hint">使用 Go duration，例如 `24h`、`90m`；最小为 `1m`。</p>
      <label>保留自动备份份数</label>
      <input v-model="f.retention" :disabled="state.retention_readonly" placeholder="7" inputmode="numeric" autocomplete="off" />
      <p class="muted hint">允许 1–365。升级前快照独立保留，不受此数字清理。</p>
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
</style>
