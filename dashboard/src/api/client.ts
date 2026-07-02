// ============================================================
// client.ts — API 客户端
//
// 封装所有到 Controller 的 HTTP 请求。
// 为什么不用 axios？
//   fetch 是现代浏览器内置的，不需要额外依赖。
//   错误处理和 JSON 解析手动做也很简单。
// ============================================================

import type { Device } from '@/types'

const API_BASE = '/api'

// ----------------------------------------------------------
// 设备 API
// ----------------------------------------------------------

/** 获取所有设备列表 */
export async function fetchDevices(): Promise<Device[]> {
  const res = await fetch(`${API_BASE}/devices`)
  if (!res.ok) {
    throw new Error(`获取设备列表失败: ${res.status}`)
  }
  const data = await res.json()
  return data.devices || []
}

/** 获取单个设备详情 */
export async function fetchDevice(id: string): Promise<Device> {
  const res = await fetch(`${API_BASE}/devices/${id}`)
  if (!res.ok) {
    throw new Error(`获取设备详情失败: ${res.status}`)
  }
  return res.json()
}
