// 后台 Vite 配置 / Admin Vite Configuration
// 功能：构建唯一 React/TypeScript 后台至 Go embed 目录，开发模式代理本机 API 与媒体
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 12:56:12

import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
const target = process.env.KARTWO_DEV_PROXY || 'http://127.0.0.1:8080'
export default defineConfig({
  plugins: [react()],
  base: '/admin/',
  server: { host: '127.0.0.1', proxy: { '/admin/api': { target }, '/media': { target } } },
  build: {
    outDir: '../dist', emptyOutDir: true,
    rollupOptions: { output: { manualChunks: id => ['/react/', '/react-dom/', '/scheduler/'].some(name => id.includes(`/node_modules${name}`)) ? 'react-core' : undefined } },
  },
})
