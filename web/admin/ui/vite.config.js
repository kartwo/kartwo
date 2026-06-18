// Vite 配置 / Vite Config
// 功能：构建 Vue3 Admin SPA 到 ../dist，供 Go embed 托管
// 作者：仗键天涯(daxing) ｜ 邮箱：3442535897@qq.com ｜ 时间：2026-06-18 10:20:00
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  base: '/',
  build: {
    outDir: '../dist',
    emptyOutDir: true,
  },
})
