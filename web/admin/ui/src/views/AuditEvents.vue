<!-- 审计日志页 / Audit Events View. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<!-- 功能：展示最近后台关键操作；只读且不显示口令、密钥、会话令牌或请求正文 -->
<script setup>
import { inject, onMounted, ref } from 'vue'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'

const onUnauthorized = inject('onUnauthorized', null)
const toast = useToast()
const loading = ref(true)
const events = ref([])

function describe(event) {
  const labels = {
    'admin.login': '登录后台',
    'admin.logout': '退出后台',
    'product.create': '新建商品',
    'product.update': '更新商品',
    'product.delete': '删除商品',
    'variant.inventory_update': '更新库存',
    'variant.price_update': '更新价格',
    'import.execute': '确认导入商品',
    'backup.settings_update': '更新自动备份设置',
    'payment.settings_update': '更新收款设置',
    'domain.settings_update': '更新域名设置',
    'market.settings_update': '更新市场设置',
    'smtp.settings_update': '更新邮件设置',
    'order.refund': '退款订单',
  }
  return labels[event.action] || event.action
}

async function load() {
  loading.value = true
  try {
    const data = await api.auditEvents()
    events.value = data.events || []
  } catch (e) {
    if (e instanceof APIError && e.status === 401 && onUnauthorized) return onUnauthorized()
    toast.error(e.message)
  } finally { loading.value = false }
}

onMounted(load)
</script>

<template>
  <div class="audit-page">
    <div class="page-title-row">
      <div>
        <h2>审计日志</h2>
        <p class="muted">最近 100 条关键后台操作；不记录口令、密钥、会话令牌或请求内容。</p>
      </div>
      <button :disabled="loading" @click="load">刷新</button>
    </div>
    <section class="panel card">
      <p v-if="loading" class="muted">加载中…</p>
      <p v-else-if="!events.length" class="muted">尚无审计记录。</p>
      <div v-else class="audit-table-wrap">
        <table class="audit-table">
          <thead><tr><th>时间</th><th>操作人</th><th>操作</th><th>对象</th></tr></thead>
          <tbody>
            <tr v-for="event in events" :key="event.public_id">
              <td>{{ event.created_at }}</td>
              <td>{{ event.admin_username }}</td>
              <td>{{ describe(event) }}</td>
              <td>{{ event.target_public_id || '—' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>

<style scoped>
.audit-page { display: grid; gap: var(--sp-4); }
.audit-page h2 { margin: 0; }
.page-title-row { display: flex; align-items: start; justify-content: space-between; gap: var(--sp-3); }
.page-title-row p { margin: var(--sp-1) 0 0; }
.audit-table-wrap { overflow-x: auto; }
.audit-table { width: 100%; min-width: 680px; }
.audit-table th, .audit-table td { text-align: left; }
</style>
