<!-- CSV 商品导入 / CSV Product Import. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<script setup>
import { ref, inject } from 'vue'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'

const file = ref(null)
const preview = ref(null)
const busy = ref(false)
const err = ref('')
const toast = useToast()
const onUnauthorized = inject('onUnauthorized')

function choose(event) {
  file.value = event.target.files?.[0] || null
  preview.value = null
  err.value = ''
}
async function dryRun() {
  if (!file.value) { err.value = '请先选择 CSV 文件'; return }
  busy.value = true; err.value = ''
  try { preview.value = await api.previewImportCSV(file.value) }
  catch (e) { if (e instanceof APIError && e.status === 401) return onUnauthorized(); err.value = e.message }
  finally { busy.value = false }
}
async function execute() {
  if (!preview.value || preview.value.errors?.length) return
  busy.value = true; err.value = ''
  try {
    preview.value = await api.executeImport(preview.value.public_id)
    toast.success('商品已导入')
  } catch (e) { if (e instanceof APIError && e.status === 401) return onUnauthorized(); err.value = e.message }
  finally { busy.value = false }
}
</script>

<template>
  <div class="row page-title-row">
    <div><h2>导入商品</h2><p class="muted">先干跑检查，确认无错误后才会写入商品。</p></div>
    <RouterLink to="/products">返回商品</RouterLink>
  </div>
  <section class="panel import-panel">
    <label>通用 CSV 文件</label>
    <input type="file" accept=".csv,text/csv" @change="choose" />
    <p class="muted">列：title、slug、status、description、option1_name、option1_value、option2_name、option2_value、sku、price_cents、quantity。</p>
    <button class="primary" :disabled="busy || !file" @click="dryRun">{{ busy ? '处理中…' : '预览导入' }}</button>
    <p v-if="err" class="err">{{ err }}</p>
  </section>
  <section v-if="preview" class="panel import-panel">
    <h3>预览结果</h3>
    <div class="import-summary"><span>行数 {{ preview.total_rows }}</span><span>商品 {{ preview.product_count }}</span><span>变体 {{ preview.variant_count }}</span></div>
    <p v-if="preview.status === 'succeeded'" class="ok">该文件已成功导入；重复确认不会再创建商品。</p>
    <template v-else-if="preview.errors?.length">
      <p class="err">发现 {{ preview.errors.length }} 行错误，修正 CSV 后请重新预览。</p>
      <table><thead><tr><th>行</th><th>问题</th></tr></thead><tbody><tr v-for="e in preview.errors" :key="e.row"><td>{{ e.row }}</td><td>{{ e.message }}</td></tr></tbody></table>
    </template>
    <button v-else class="primary" :disabled="busy" @click="execute">{{ busy ? '导入中…' : '确认导入' }}</button>
  </section>
</template>
