// ============================================================
// main.tsx — Dashboard 入口
// ============================================================

import React from 'react'
import ReactDOM from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Toaster } from 'react-hot-toast'
import App from './App'

// React Query 客户端
// staleTime: 2 秒内不重新请求（设备列表每 2 秒才刷新一次）
// gcTime: 5 分钟垃圾回收
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 2000,
      gcTime: 5 * 60 * 1000,
    },
  },
})

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
      <Toaster position="top-right" />
    </QueryClientProvider>
  </React.StrictMode>,
)
