// ============================================================
// types/index.ts — TypeScript 类型定义
//
// 这些类型对应 proto 文件中的 message 定义。
// 为什么不用 proto 自动生成 TS 类型？
//   当前 Phase 0 阶段手写以快速迭代。
//   将来可以用 protoc-gen-ts 从 proto 文件生成。
// ============================================================

export type DeviceState = 'ONLINE' | 'UNHEALTHY' | 'OFFLINE'

export interface Device {
  id: string
  hostname: string
  os: string
  arch: string
  cpu_cores: number
  total_ram_bytes: number
  gpu_info: string
  supported_runtimes: string[]
  agent_address: string
  state: DeviceState
  cpu_percent: number
  used_ram_bytes: number
  cpu_temperature: number
  running_task_count: number
  /** 注册时间（ISO 8601 格式） */
  registered_at: string
  /** 最后心跳时间（ISO 8601 格式） */
  last_heartbeat: string
}

// ============================================================
// Phase 2: 任务相关类型
// ============================================================

export type JobState = 'QUEUED' | 'ASSIGNED' | 'RUNNING' | 'COMPLETED' | 'FAILED' | 'CANCELLED'

export interface Job {
  id: string
  name: string
  command: string
  args: string[]
  env: Record<string, string>
  status: JobState
  device_id: string
  exit_code: number
  logs?: LogLine[]
  created_at: string
}

export interface LogLine {
  stream_type: 'stdout' | 'stderr'
  data: string
  timestamp: string
}
