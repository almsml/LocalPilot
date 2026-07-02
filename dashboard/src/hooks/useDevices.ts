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

/** 获取设备列表，自动每 2 秒刷新 */
export function useDevices() {
  return useQuery({
    queryKey: ['devices'],
    queryFn: fetchDevices,
    // refetchInterval: 2000,  // Phase 1: 启用自动刷新
  })
}
