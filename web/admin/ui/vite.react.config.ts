// React 迁移构建 / React Migration Build
// 功能：验证新后台组件包；正式入口切换时合并至唯一 Vite 配置
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 10:12:36

import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { fileURLToPath } from 'node:url'
export default defineConfig({
  plugins: [react()],
  base: '/admin/',
  build: {
    outDir: '.react-build', emptyOutDir: true,
    lib: { entry: fileURLToPath(new URL('./src/react/index.ts', import.meta.url)), formats: ['es'], fileName: 'admin' },
  },
})
