<!-- 明文访问提示 / Insecure Connection Notice. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<!-- 功能：以普通 http 网址访问后台时，常驻顶部提示"连接未加密、属配置期临时状态"（D7-B 逃生路的附带条件） -->
<script setup>
import { computed } from 'vue'

// 何时提示：用 http 访问 **且** 不是回环地址。
// 回环地址（localhost / 127.0.0.1 / [::1]）被浏览器视为可信来源，本地开发天然是 http，
// 在那里提示纯属噪音；同时这也正是「回环地址上验不出 Secure cookie 问题」的同一条规则。
const trustworthy = ['localhost', '127.0.0.1', '[::1]', '::1']
const insecure = computed(() =>
  window.location.protocol === 'http:' && !trustworthy.includes(window.location.hostname)
)
</script>

<template>
  <div v-if="insecure" class="insecure-notice">
    <strong>当前用普通网址（http）访问后台</strong>，连接没有加密，这是配置阶段的临时状态。
    等域名和证书配好后，请改用 <strong>https</strong> 开头的网址访问，更安全。
  </div>
</template>

<style scoped>
/* 持续状态 → 常驻 inline，不用 toast（守 M4.2.3a 判据）；全部走 token，无硬编码色值。 */
.insecure-notice {
  padding: var(--sp-2) var(--sp-4);
  background: var(--warn-bg);
  color: var(--warn);
  border-bottom: 1px solid var(--warn);
  font-size: var(--fs-sm);
  line-height: 1.55;
  text-align: center;
}
</style>
