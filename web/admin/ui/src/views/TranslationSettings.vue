<!-- 翻译服务设置 / Translation Service Settings. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<script setup>
import { onMounted, ref, inject } from 'vue'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'
const toast = useToast(); const onUnauthorized = inject('onUnauthorized'); const busy = ref(false); const state = ref({}); const f = ref({ plan: 'developer', api_key: '' })
async function load () { try { const d = await api.getTranslation(); state.value = d; f.value.plan = d.plan || 'developer' } catch (e) { toast.error(e.message) } }
async function save () { busy.value = true; try { state.value = await api.setTranslation({ ...f.value }); f.value.api_key = ''; toast.success('DeepL 配置已保存') } catch (e) { if (e instanceof APIError && e.status === 401 && onUnauthorized) return onUnauthorized(); toast.error(e.message) } finally { busy.value = false } }
onMounted(load)
</script>
<template><div class="panel card"><h2>英文内容翻译</h2><p class="muted">在商品页点击翻译按钮时，才会将中文标题、slug 或 SEO 描述发送给 DeepL 并生成英文建议；不会自动调用。API Key 加密保存且不会发送到浏览器。</p><p class="muted">还没有 API Key？<a href="https://www.deepl.com/en/developers" target="_blank" rel="noopener noreferrer">前往 DeepL 官方申请 API Key ↗</a></p><label>服务商</label><input value="DeepL" disabled /><label>套餐</label><select v-model="f.plan"><option value="developer">Developer API：累计 100 万字符免费</option><option value="growth">Growth API：月付 100 万字符 / 年付 1200 万字符</option></select><p class="muted">{{ state.price_note }}</p><label>DeepL API Key</label><input v-model="f.api_key" type="password" autocomplete="off" :placeholder="state.has_api_key ? '已保存，留空不改' : '粘贴 DeepL API Key'" /><button class="primary" :disabled="busy" @click="save">{{ busy ? '保存中…' : '保存配置' }}</button></div></template>
