<!-- 向导·配置收款 / Wizard Payment Step. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<script setup>
import { ref, inject } from 'vue'
import { api, APIError } from '../api.js'
import PaymentSettings from './PaymentSettings.vue'

const emit = defineEmits(['done'])
const onUnauthorized = inject('onUnauthorized')
const busy = ref(false)
const err = ref('')

async function skip() {
  if (busy.value) return
  busy.value = true; err.value = ''
  try {
    await api.wizardPaymentSkip()
    emit('done')
  } catch (e) {
    if (e instanceof APIError && e.status === 401) return onUnauthorized()
    err.value = e.message
  } finally { busy.value = false }
}
function enter() { emit('done') }
</script>

<template>
  <div class="container" style="max-width:760px">
    <h2>配置收款方式</h2>
    <p class="muted" style="max-width:64ch">
      想让顾客<strong>真正付钱</strong>，得先接上收款。下面填好任意一种（Stripe 收信用卡、或 PayPal）即可，
      <strong>默认沙箱/测试</strong>，可放心用测试卡先跑通。<strong>不急也能稍后再配</strong>——但没配好之前，顾客只能下单、付不了款。
    </p>
    <p v-if="err" class="err">{{ err }}</p>

    <PaymentSettings />

    <div class="row" style="gap:.8rem;margin-top:1.2rem">
      <button class="primary" @click="enter">配好了，进入后台 →</button>
      <button :disabled="busy" @click="skip">稍后再配</button>
    </div>
    <p class="muted" style="font-size:.82rem;margin-top:.5rem">随时可在后台「收款」页修改。</p>
  </div>
</template>
