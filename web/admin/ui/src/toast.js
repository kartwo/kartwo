// 轻量 Toast 通知 / Lightweight Toast (Admin SPA)
// 功能：全局 toast 队列 + success/error；悬浮不随滚动、可堆叠、可手动关闭；error 停留更久(6s)让非技术用户读完
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-07-07 11:12:18
import { reactive } from 'vue'

// 全局单例队列（跨组件共享），无外部依赖。
const state = reactive({ items: [] })
let seq = 0

function remove(id) {
  const i = state.items.findIndex(t => t.id === id)
  if (i !== -1) state.items.splice(i, 1)
}

// push 入队一条 toast，ms>0 时自动消失；返回 id 供手动关闭。
function push(type, message, ms) {
  const id = ++seq
  state.items.push({ id, type, message })
  if (ms > 0) setTimeout(() => remove(id), ms)
  return id
}

export function useToast() {
  return {
    items: state.items,
    remove,
    // error 停留 6s（Derek 要求"别太短"，非技术用户来得及读），且始终带手动关闭按钮；success 3s。
    error: (m) => push('error', m, 6000),
    success: (m) => push('success', m, 3000),
  }
}
