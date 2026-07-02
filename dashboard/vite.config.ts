// ============================================================
// vite.config.ts — Vite 构建配置
//
// 开发时 Dashboard 跑在 localhost:5173，
// 通过 proxy 把 /api 和 /ws 请求转发到 Controller（localhost:8080），
// 避免跨域问题。
// ============================================================

import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'node:path'

export default defineConfig({
  plugins: [react()],

  // 开发服务器配置
  server: {
    port: 5173,
    strictPort: true,

    // API 代理：/api/* → Controller:8080
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/ws': {
        target: 'ws://localhost:8080',
        ws: true, // WebSocket 代理
      },
    },
  },

  // 路径别名：@ → src/
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src'),
    },
  },

  build: {
    outDir: 'dist',
    sourcemap: true,
  },
})
