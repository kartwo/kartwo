<!-- 向导·配置域名 / Wizard Domain Step. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<!-- 功能：开店向导第 3 步——录入域名(复用 DomainSettings)、可「暂不配先用 http」跳过、可「上一步」回收款步 -->
<script setup>
import { ref, inject } from 'vue'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'
import DomainSettings from './DomainSettings.vue'

const emit = defineEmits(['done', 'back'])
const onUnauthorized = inject('onUnauthorized')
const toast = useToast()
const busy = ref(false)

async function skip() {
  if (busy.value) return
  busy.value = true
  try {
    await api.wizardDomainSkip()
    emit('done')
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    toast.error(e.message)
  } finally { busy.value = false }
}
function enter() { emit('done') }
function back() { emit('back') }
</script>

<template>
  <div class="container" style="max-width:760px">
    <h2>配置你的店铺域名</h2>
    <p class="muted" style="max-width:64ch">
      有自己的域名（如 <code>shop.example.com</code>）就填在这里，Kartwo 会自动申请免费 HTTPS，让顾客访问时带安全绿锁。
      <strong>还没有域名也没关系</strong>——先跳过，用普通网址把店开起来，等买了域名再回来配。
    </p>

    <DomainSettings />

    <div class="row wiz-actions" style="flex:0;gap:.8rem;margin-top:1.4rem">
      <button @click="back">← 上一步</button>
      <button :disabled="busy" @click="skip">暂不配域名，先用 http 开店</button>
      <button class="primary" @click="enter">完成，进入后台 →</button>
    </div>
    <p class="muted" style="font-size:.82rem;margin-top:.5rem">随时可在后台「域名」页修改。</p>
  </div>
</template>

<style scoped>
code { background: rgba(148,163,184,.18); padding: 0 .3rem; border-radius: 4px; font-size: .9em; }
.wiz-actions { flex-wrap: wrap; }
</style>
