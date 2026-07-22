// 统一确认弹窗 / Unified Confirm Dialog
// 功能：promise 化 confirm(opts)——替换原生 window.confirm；单例状态，配 ConfirmDialog.vue 常驻渲染
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-07-22 12:00:00
import { reactive } from 'vue'

// 单例状态（同一时刻只一个模态；模态阻塞交互，够用）。
const state = reactive({
  open: false, title: '', message: '',
  confirmText: '确认', cancelText: '取消', danger: false,
  _resolve: null,
})

// confirm 弹出确认框，返回 Promise<boolean>（确认=true，取消/Esc/点遮罩=false）。
export function confirm(opts = {}) {
  return new Promise((resolve) => {
    state.title = opts.title || '确认操作'
    state.message = opts.message || ''
    state.confirmText = opts.confirmText || '确认'
    state.cancelText = opts.cancelText || '取消'
    state.danger = !!opts.danger
    state._resolve = resolve
    state.open = true
  })
}

function settle(v) {
  if (!state.open) return
  state.open = false
  const r = state._resolve
  state._resolve = null
  if (r) r(v)
}

export function useConfirmState() { return state }
export function confirmYes() { settle(true) }
export function confirmNo() { settle(false) }
