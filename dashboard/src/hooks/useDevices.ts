// ============================================================
// useDevices.ts — 设备列表 React Query Hook
//
// 为什么用 React Query 而不是 useEffect + fetch？
//   React Query 自动管理缓存、后台刷新、loading/error 状态。
//   设备列表每 2 秒自动刷新一次（staleTime: 2000），
//   不需要手写 setInterval。
// ============================================================

import { useQuery } from '@tanstack/react-query'
import { fetchDevices } from '@/api/client'

/**
 * 获取设备列表，自动每 2 秒刷新
 *
 * 为什么 refetchInterval 是 2 秒而不是 5 秒（心跳间隔）？
 *   心跳间隔是 5 秒。如果 Dashboard 也是 5 秒刷新一次，
 *   可能刚好错过心跳——用户看到的是 10 秒前的数据。
 *   2 秒刷新保证数据延迟不超过 2 秒，用户体验更好。
 *   这个轮询间隔对 Controller 的压力很小——
 *   GET /api/devices 只是内存中 RWMutex 读锁操作。
 */
export function useDevices() {
  return useQuery({
    queryKey: ['devices'],
    queryFn: fetchDevices,
    refetchInterval: 2000,      // 每 2 秒自动刷新
    refetchOnWindowFocus: true,  // 窗口重新获得焦点时立即刷新
  })
}
