<!-- 页级错误状态块 / Page-level Error State. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<!-- 功能：把页级加载失败/404 的裸红字升级为协调的常驻状态块（图标+说明+可选重试）；持续状态、不自动消失、非 toast -->
<script setup>
defineProps({
  message: { type: String, required: true },
  retry: { type: Function, default: null },
})
</script>

<template>
  <div class="err-state">
    <span class="err-ic" aria-hidden="true">⚠️</span>
    <p class="err-msg">{{ message }}</p>
    <button v-if="retry" class="err-retry" @click="retry">重试</button>
  </div>
</template>

<style scoped>
.err-state {
  display: flex; flex-direction: column; align-items: center; gap: .6rem; text-align: center;
  padding: 2.2rem 1.2rem; margin: 1rem 0;
  background: var(--surface); border: 1px solid var(--border);
  border-radius: var(--radius-lg); box-shadow: var(--shadow-sm);
}
.err-ic { font-size: 1.7rem; line-height: 1; }
.err-msg { margin: 0; color: var(--text); font-weight: 500; }
.err-retry { flex: 0 0 auto; }
</style>
