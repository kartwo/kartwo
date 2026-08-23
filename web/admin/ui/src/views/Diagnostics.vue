<!-- 本机诊断 / Local Diagnostics -->
<!-- 功能：展示数据库、媒体占用与媒体目录所在磁盘的实时状态 -->
<!-- 作者：仗键天涯(daxing) -->
<!-- 邮箱：3442535897@qq.com -->
<!-- 时间：2026-08-22 09:10:00 -->
<script setup>
import { inject, onMounted, ref } from 'vue'
import { api, APIError } from '../api.js'
import ErrorState from '../components/ErrorState.vue'

const onUnauthorized = inject('onUnauthorized', null)
const data = ref(null)
const loaded = ref(false)
const err = ref('')

async function load() {
  err.value = ''
  try {
    data.value = await api.diagnostics()
  } catch (e) {
    if (e instanceof APIError && e.status === 401 && onUnauthorized) return onUnauthorized()
    err.value = e.message
  } finally {
    loaded.value = true
  }
}

function bytes(value) {
  const n = Number(value || 0)
  if (n < 1024) return n + ' B'
  const units = ['KB', 'MB', 'GB', 'TB']
  let size = n
  let unit = -1
  do { size /= 1024; unit += 1 } while (size >= 1024 && unit < units.length - 1)
  return size.toFixed(size >= 10 ? 0 : 1) + ' ' + units[unit]
}

function formatTime(value) {
  if (!value) return '尚未生成'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '暂不可用' : date.toLocaleString()
}
</script>

<template>
  <div class="diagnostics-page">
    <div class="row page-title-row">
      <h2>诊断</h2>
      <button @click="load">刷新</button>
    </div>

    <template v-if="data">
      <section class="panel diagnostics-panel">
        <h3>数据库</h3>
        <div class="diagnostic-row">
          <span>连接</span>
          <strong class="ok">正常</strong>
        </div>
      </section>

      <section class="panel diagnostics-panel">
        <h3>媒体文件</h3>
        <dl class="diagnostic-grid">
          <div><dt>资产数量</dt><dd>{{ data.media.asset_count }}</dd></div>
          <div><dt>原图</dt><dd>{{ bytes(data.media.original_bytes) }}</dd></div>
          <div><dt>派生文件</dt><dd>{{ bytes(data.media.derivative_bytes) }}</dd></div>
          <div><dt>媒体合计</dt><dd>{{ bytes(data.media.total_bytes) }}</dd></div>
        </dl>
      </section>

      <section class="panel diagnostics-panel">
        <h3>媒体目录磁盘</h3>
        <template v-if="data.disk.available">
          <dl class="diagnostic-grid">
            <div><dt>总容量</dt><dd>{{ bytes(data.disk.total_bytes) }}</dd></div>
            <div><dt>可用空间</dt><dd class="ok">{{ bytes(data.disk.free_bytes) }}</dd></div>
            <div><dt>已用空间</dt><dd>{{ bytes(data.disk.used_bytes) }}</dd></div>
          </dl>
        </template>
        <p v-else class="err">{{ data.disk.message }}</p>
      </section>

      <section class="panel diagnostics-panel">
        <h3>本地备份与升级快照</h3>
        <dl class="diagnostic-grid">
          <div><dt>自动备份</dt><dd>{{ data.backups.automatic_count }} 份</dd></div>
          <div><dt>升级快照</dt><dd>{{ data.backups.upgrade_count }} 份</dd></div>
          <div><dt>保护点占用</dt><dd>{{ bytes(data.backups.total_bytes) }}</dd></div>
          <div><dt>最近生成</dt><dd>{{ formatTime(data.backups.latest_at) }}</dd></div>
        </dl>
        <p v-if="data.backups.message" class="err">{{ data.backups.message }}</p>
      </section>
    </template>

    <ErrorState v-else-if="loaded && err" :message="err" :retry="load" />
    <p v-else-if="loaded" class="muted">诊断暂时加载不出来，请刷新重试。</p>
    <p v-else class="muted">加载中…</p>
  </div>
</template>

<style scoped>
.diagnostics-page { display: grid; gap: var(--sp-4); }
.diagnostics-page h2, .diagnostics-panel h3 { margin: 0; }
.diagnostics-panel { display: grid; gap: var(--sp-3); }
.diagnostic-row { display: flex; justify-content: space-between; padding-top: var(--sp-2); border-top: 1px solid var(--border); }
.diagnostic-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(130px, 1fr)); gap: var(--sp-3); margin: 0; }
.diagnostic-grid div { padding-top: var(--sp-2); border-top: 1px solid var(--border); }
dt { color: var(--text-muted); font-size: var(--fs-sm); }
dd { margin: var(--sp-1) 0 0; font-size: var(--fs-lg); font-weight: 600; }
</style>
