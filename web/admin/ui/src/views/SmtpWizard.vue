<!-- 向导·配置邮件 / Wizard SMTP Step. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<!-- 功能：开店向导第 4 步——录入 SMTP(复用 SmtpSettings)、可「暂不配」跳过、可「上一步」回域名步 -->
<script setup>
import { ref, inject } from 'vue'
import { api, APIError } from '../api.js'
import { useToast } from '../toast.js'
import SmtpSettings from './SmtpSettings.vue'

const emit = defineEmits(['done', 'back'])
const onUnauthorized = inject('onUnauthorized')
const toast = useToast()
const busy = ref(false)

async function skip() {
  if (busy.value) return
  busy.value = true
  try {
    await api.wizardSmtpSkip()
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
    <h2>配置发信邮箱</h2>
    <p class="muted" style="max-width:64ch">
      配好发信邮箱，顾客付款后会自动收到<strong>订单确认信</strong>。
      <strong>还没准备好也没关系</strong>——可以先跳过，之后在后台「邮件」页随时配；没配好之前只是暂时不发确认信，<strong>不影响下单和收款</strong>。
    </p>

    <SmtpSettings />

    <div class="row wiz-actions" style="flex:0;gap:.8rem;margin-top:1.4rem">
      <button @click="back">← 上一步</button>
      <button :disabled="busy" @click="skip">暂不配邮件，稍后再说</button>
      <button class="primary" @click="enter">完成，进入后台 →</button>
    </div>
    <p class="muted" style="font-size:.82rem;margin-top:.5rem">随时可在后台「邮件」页修改。</p>
  </div>
</template>

<style scoped>
.wiz-actions { flex-wrap: wrap; }
</style>
