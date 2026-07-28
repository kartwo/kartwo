<!-- 统一确认弹窗 / Confirm Dialog. 作者：仗键天涯(daxing) 3442535897@qq.com -->
<!-- 功能：视口居中模态卡 + 半透明遮罩；破坏性确认=红实心(D4)；Esc/点遮罩=取消；打开时聚焦确认键。常驻挂 App.vue -->
<script setup>
import { ref, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useConfirmState, confirmYes, confirmNo } from '../confirm.js'

const s = useConfirmState()
const confirmBtn = ref(null)

// 打开时把焦点移到确认键（键盘可用 + 无障碍）。
watch(() => s.open, async (open) => {
  if (open) { await nextTick(); confirmBtn.value && confirmBtn.value.focus() }
})

// 全局 Esc = 取消（遮罩未必持有焦点，挂 window 更稳）。
function onKey(e) { if (s.open && e.key === 'Escape') confirmNo() }
onMounted(() => window.addEventListener('keydown', onKey))
onUnmounted(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <div v-if="s.open" class="cd-overlay" @click.self="confirmNo">
    <div class="cd-card" role="dialog" aria-modal="true" :aria-label="s.title">
      <h3 class="cd-title">{{ s.title }}</h3>
      <p v-if="s.message" class="cd-msg">{{ s.message }}</p>
      <div class="cd-actions">
        <button class="cd-cancel" @click="confirmNo">{{ s.cancelText }}</button>
        <button ref="confirmBtn" :class="s.danger ? 'cd-confirm-danger' : 'primary'" @click="confirmYes">
          {{ s.confirmText }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.cd-overlay {
  position: fixed; inset: 0; z-index: 1000;
  background: rgba(0, 0, 0, .4);
  display: grid; place-items: center; padding: 1rem;
}
.cd-card {
  width: 400px; max-width: 92vw;
  background: var(--surface); border: 1px solid var(--border);
  border-radius: var(--radius-lg); box-shadow: var(--shadow-md);
  padding: 1.3rem 1.4rem;
}
.cd-title { margin: 0 0 .5rem; font-size: var(--fs-md); font-weight: 600; }
.cd-msg { margin: 0 0 1.2rem; color: var(--text-muted); line-height: 1.5; }
.cd-actions { display: flex; justify-content: flex-end; gap: .6rem; }
.cd-actions button { flex: 0 0 auto; }
/* D4：破坏性确认=红实心 */
.cd-confirm-danger {
  background: var(--danger); color: #fff; border-color: var(--danger); font-weight: 600;
}
.cd-confirm-danger:hover { background: #c01739; border-color: #c01739; }
</style>
