<!-- Toast 宿主 / Toast Host. 悬浮固定于视口顶部居中，不随页面滚动。作者：仗键天涯(daxing) 3442535897@qq.com -->
<script setup>
import { useToast } from '../toast.js'
const { items, remove } = useToast()
</script>

<template>
  <div class="toast-host" aria-live="polite">
    <div v-for="t in items" :key="t.id" class="toast" :class="t.type" role="alert">
      <span class="toast-icon" aria-hidden="true">{{ t.type === 'success' ? '✓' : '✕' }}</span>
      <span class="toast-msg">{{ t.message }}</span>
      <button class="toast-close" aria-label="关闭" @click="remove(t.id)">×</button>
    </div>
  </div>
</template>

<style scoped>
/* 固定视口正中：无论页面滚到哪都稳定居中显示（Derek 要求，最醒目、最难忽略）；
   不加遮罩、host 本身不拦点击（pointer-events:none），仅每条 toast 可交互，故不阻断页面操作。
   多条在正中垂直有序堆叠、居中对齐，整块随条数增长仍以视口中心为中点。 */
.toast-host {
  position: fixed; top: 50%; left: 50%; transform: translate(-50%, -50%);
  z-index: 1000; display: flex; flex-direction: column; gap: .5rem;
  width: 420px; max-width: 92vw; pointer-events: none;
}
.toast {
  pointer-events: auto; display: flex; align-items: flex-start; gap: .6rem;
  background: var(--panel); border: 1px solid var(--line); border-left-width: 4px;
  border-radius: 8px; padding: .7rem .8rem; color: var(--text);
  box-shadow: 0 6px 24px rgba(0, 0, 0, .35);
}
.toast.success { border-left-color: var(--ok); }
.toast.error { border-left-color: var(--danger); }
.toast-icon { font-weight: 700; line-height: 1.4; }
.toast.success .toast-icon { color: var(--ok); }
.toast.error .toast-icon { color: var(--danger); }
.toast-msg { flex: 1; line-height: 1.4; font-size: .9rem; word-break: break-word; }
/* 覆盖全局 button 默认样式，做成朴素关闭按钮 */
.toast-close {
  flex: 0; background: transparent; border: none; color: var(--muted);
  font-size: 1.15rem; line-height: 1; cursor: pointer; padding: 0 .1rem;
}
.toast-close:hover { color: var(--text); }
</style>
