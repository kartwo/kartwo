// 界面测试环境 / UI Test Setup
// 功能：补齐 jsdom 不提供的媒体查询并清理组件和请求桩
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
afterEach(() => { cleanup(); vi.unstubAllGlobals() })
