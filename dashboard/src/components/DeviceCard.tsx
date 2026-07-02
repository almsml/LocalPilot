// ============================================================
// DeviceCard.tsx — 设备卡片
//
// 显示一台设备的实时状态：主机名、CPU/内存/温度、运行任务数。
// ============================================================

import type { Device } from '@/types'

/** 状态 → 颜色映射 */
const stateColor: Record<string, string> = {
  ONLINE: '#22c55e',
  UNHEALTHY: '#eab308',
  OFFLINE: '#ef4444',
}

/** 状态 → 中文标签 */
const stateLabel: Record<string, string> = {
  ONLINE: '在线',
  UNHEALTHY: '异常',
  OFFLINE: '离线',
}

/** 字节 → 人类可读 */
function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`
}

interface Props {
  device: Device
}

export function DeviceCard({ device }: Props) {
  const color = stateColor[device.state] || '#999'

  return (
    <div
      style={{
        border: `2px solid ${color}`,
        borderRadius: 12,
        padding: 16,
        background: '#fff',
        boxShadow: '0 1px 3px rgba(0,0,0,0.1)',
      }}
    >
      {/* 设备名 + 状态 */}
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          marginBottom: 12,
        }}
      >
        <h3 style={{ margin: 0, fontSize: 18 }}>{device.hostname}</h3>
        <span
          style={{
            background: color,
            color: '#fff',
            padding: '2px 8px',
            borderRadius: 8,
            fontSize: 12,
            fontWeight: 600,
          }}
        >
          {stateLabel[device.state] || device.state}
        </span>
      </div>

      {/* 系统信息 */}
      <div style={{ fontSize: 13, color: '#666', lineHeight: 1.8 }}>
        <div>系统: {device.os} / {device.arch}</div>
        <div>CPU: {device.cpu_cores} 核 | 使用率: {(device.cpu_percent * 100).toFixed(0)}%</div>
        <div>内存: {formatBytes(device.used_ram_bytes)} / {formatBytes(device.total_ram_bytes)}</div>
        {device.gpu_info && <div>GPU: {device.gpu_info}</div>}
        <div>温度: {device.cpu_temperature > 0 ? `${device.cpu_temperature.toFixed(1)}°C` : 'N/A'}</div>
        <div>运行任务: {device.running_task_count} 个</div>
      </div>
    </div>
  )
}
