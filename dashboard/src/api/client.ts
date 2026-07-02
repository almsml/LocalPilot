// ============================================================
// client.ts — API 客户端
//
// 封装所有到 Controller 的 HTTP 请求。
// 为什么不用 axios？
//   fetch 是现代浏览器内置的，不需要额外依赖。
//   错误处理和 JSON 解析手动做也很简单。
// ============================================================

import type { Device, Job } from '@/types'

const API_BASE = '/api'

// ----------------------------------------------------------
// 设备 API
// ----------------------------------------------------------

/** 获取所有设备列表 */
export async function fetchDevices(): Promise<Device[]> {
  const res = await fetch(`${API_BASE}/devices`)
  if (!res.ok) throw new Error(`获取设备列表失败: ${res.status}`)
  const data = await res.json()
  return data.devices || []
}

/** 获取单个设备详情 */
export async function fetchDevice(id: string): Promise<Device> {
  const res = await fetch(`${API_BASE}/devices/${id}`)
  if (!res.ok) throw new Error(`获取设备详情失败: ${res.status}`)
  return res.json()
}

// ----------------------------------------------------------
// 任务 API (Phase 2)
// ----------------------------------------------------------

/** 提交任务 */
export async function submitJob(
  command: string,
  args: string[] = [],
  env: Record<string, string> = {},
  name?: string,
): Promise<Job> {
  const res = await fetch(`${API_BASE}/jobs`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      name: name || command,
      command,
      args,
      env,
    }),
  })
  if (!res.ok) throw new Error(`提交任务失败: ${res.status}`)
  return res.json()
}

/** 获取任务详情（含日志） */
export async function fetchJob(id: string): Promise<Job> {
  const res = await fetch(`${API_BASE}/jobs/${id}`)
  if (!res.ok) throw new Error(`获取任务失败: ${res.status}`)
  return res.json()
}

/** 列出所有任务 */
export async function fetchJobs(): Promise<Job[]> {
  const res = await fetch(`${API_BASE}/jobs`)
  if (!res.ok) throw new Error(`获取任务列表失败: ${res.status}`)
  const data = await res.json()
  return data.jobs || []
}
