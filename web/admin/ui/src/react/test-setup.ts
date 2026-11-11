// 界面测试环境 / UI Test Setup
// 功能：补齐 jsdom 媒体查询、滚动条探测并清理组件和请求桩
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:12:36

import { afterEach, vi } from 'vitest'
import { cleanup } from '@testing-library/react'
Object.defineProperty(window, 'matchMedia', { writable: true, value: (query: string) => ({
  matches: false, media: query, onchange: null,
  addListener() {}, removeListener() {}, addEventListener() {}, removeEventListener() {}, dispatchEvent() { return true },
}) })
// jsdom has no layout engine; layout measurements belong to real-browser acceptance.
Object.defineProperty(globalThis, 'ResizeObserver', { writable: true, value: class {
  observe() {}
  unobserve() {}
  disconnect() {}
} })
// Ant's scrollbar probe requests this pseudo-element on every table/modal.
// jsdom 30 warns and then returns the element's style anyway (Window.js).
// Preserve that same result without repeated unsupported-API console I/O;
// other pseudo-elements/errors stay untouched. Native browsers verify layout.
const elementComputedStyle = window.getComputedStyle.bind(window)
window.getComputedStyle = (element, pseudo) => elementComputedStyle(element, pseudo === '::-webkit-scrollbar' ? undefined : pseudo)
globalThis.getComputedStyle = window.getComputedStyle
afterEach(() => { cleanup(); vi.unstubAllGlobals() })
