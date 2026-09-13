<!-- 店铺设置 / Shop Settings. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<!-- 功能：后台保存店铺名称并上传 Logo，实时作用于店面品牌展示与 SEO -->
<script setup>
import { inject, onMounted, ref } from 'vue'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'
import ErrorState from '../components/ErrorState.vue'

const onUnauthorized = inject('onUnauthorized')
const toast = useToast()
const err = ref('')
const busy = ref(false)
const logoBusy = ref(false)
const shop = ref({ name: '', source: 'db', readonly: false, logo_url: '' })

async function load() {
  err.value = ''
  try { Object.assign(shop.value, await api.getShop()) } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    err.value = e.message
  }
}
async function save() {
	if (busy.value) return
  busy.value = true
  try {
    Object.assign(shop.value, await api.setShop(shop.value.name.trim()))
    toast.success('店铺名称已保存，店面会立即使用新名称。')
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    toast.error(e.message)
  } finally { busy.value = false }
}
async function uploadLogo(event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  if (!file || logoBusy.value) return
  logoBusy.value = true
  try {
    Object.assign(shop.value, await api.uploadShopLogo(file))
    toast.success('Logo 已上传，店面页眉现在只显示 Logo。')
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    toast.error(e.message)
  } finally { logoBusy.value = false }
}
async function removeLogo() {
  if (logoBusy.value) return
  logoBusy.value = true
  try {
    Object.assign(shop.value, await api.deleteShopLogo())
    toast.success('Logo 已删除，店面页眉恢复显示店铺名称。')
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    toast.error(e.message)
  } finally { logoBusy.value = false }
}
onMounted(load)
</script>

<template>
  <div class="container shop-page">
    <h2>店铺设置</h2>
    <p class="muted">店铺名称用于页脚、浏览器标题与 SEO；上传 Logo 后，店面左上角只显示 Logo。</p>
    <ErrorState v-if="err" :message="err" :retry="load" />
    <section v-else class="panel card">
      <label for="shop-name">店铺名称</label>
      <input id="shop-name" v-model="shop.name" maxlength="120" @keyup.enter="save" />
      <p class="muted hint">最多 120 个字符。</p>
      <button class="primary" :disabled="busy" @click="save">{{ busy ? '正在保存…' : '保存店铺名称' }}</button>
      <div class="logo-section">
        <h3>店铺 Logo</h3>
        <p class="muted hint">支持 JPG、PNG 或 WebP，建议使用透明背景横版图；系统会压缩并去除图片元数据。</p>
        <div v-if="shop.logo_url" class="logo-preview"><img :src="shop.logo_url" :alt="shop.name" /></div>
        <div class="logo-actions">
          <label class="upload-button" :class="{ disabled: logoBusy }">
            {{ logoBusy ? '处理中…' : (shop.logo_url ? '更换 Logo' : '上传 Logo') }}
            <input type="file" accept="image/jpeg,image/png,image/webp" :disabled="logoBusy" @change="uploadLogo" />
          </label>
          <button v-if="shop.logo_url" class="danger" :disabled="logoBusy" @click="removeLogo">删除 Logo</button>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.shop-page{max-width:720px}
.panel label{display:block;margin-bottom:var(--sp-2)}
.panel input{width:100%}
.hint{font-size:var(--fs-sm)}
.logo-section{margin-top:var(--sp-6);padding-top:var(--sp-4);border-top:1px solid var(--border)}
.logo-section h3{margin-top:0}.logo-preview{display:flex;align-items:center;min-height:100px;margin:var(--sp-3) 0;padding:var(--sp-3);border:1px solid var(--border);border-radius:var(--radius-md);background:linear-gradient(45deg,#f3f4f6 25%,transparent 25%),linear-gradient(-45deg,#f3f4f6 25%,transparent 25%),linear-gradient(45deg,transparent 75%,#f3f4f6 75%),linear-gradient(-45deg,transparent 75%,#f3f4f6 75%);background-size:20px 20px;background-position:0 0,0 10px,10px -10px,-10px 0}
.logo-preview img{display:block;max-width:320px;max-height:80px}.logo-actions{display:flex;gap:var(--sp-2);align-items:center}.upload-button{display:inline-flex;align-items:center;padding:.72rem 1rem;border-radius:var(--radius-md);background:var(--accent);color:var(--on-accent);font-weight:700;cursor:pointer}.upload-button:hover{background:var(--accent-hover)}.upload-button.disabled{opacity:.55;cursor:not-allowed}.upload-button input{position:absolute;width:1px;height:1px;opacity:0;pointer-events:none}.danger{color:var(--danger)}
</style>
