// React 验证配置 / React Test Config
// 功能：以浏览器 DOM 测试会话与界面，保留原有 Node 测试
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:12:36

import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
export default defineConfig({
  plugins: [react()],
  test: {
    include: ['src/react/**/*.test.{ts,tsx}'], environment: 'jsdom',
    setupFiles: ['src/react/test-setup.ts'], restoreMocks: true,
    // Each jsdom worker loads Ant Design; serialize files to avoid local/CI resource contention.
    maxWorkers: 1,
  },
})
