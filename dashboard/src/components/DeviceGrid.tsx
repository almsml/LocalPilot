// ============================================================
// DeviceGrid.tsx — 设备网格/列表
//
// 显示所有已注册设备的卡片网格。
// 在线设备绿色边框、离线设备红色边框、不健康设备黄色边框。
// ============================================================

import type { Device } from '@/types'
import { DeviceCard } from './DeviceCard'

interface Props {
  devices: Device[]
}

export function DeviceGrid({ devices }: Props) {
  if (devices.length === 0) {
    return (
      <div style={{ padding: 48, textAlign: 'center', color: '#999' }}>
        <p>暂无设备连接</p>
        <p style={{ fontSize: 14 }}>
          启动一台设备的 Agent 后，这里会自动显示
        </p>
      </div>
    )
  }

  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))',
        gap: 16,
      }}
    >
      {devices.map((device) => (
        <DeviceCard key={device.id} device={device} />
      ))}
    </div>
  )
}
