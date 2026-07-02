// ============================================================
// App.tsx — Dashboard 根组件
//
// Phase 1: 设备状态实时刷新 + 设备详情面板
// Phase 2: 任务提交 + 终端日志
// Phase 3: 集群拓扑图 + 任务甘特图
//
// 为什么用 useState 管理选中的设备而不是 React Router？
//   Phase 1 的设备详情是一个快速查看侧面板，不需要深层链接。
//   用户点击任意卡片 → 右侧滑出详情 → 点击遮罩/关闭按钮 → 面板消失。
//   这个交互天然是状态驱动的——组件内部管理 selectedDeviceId 足够。
// ============================================================

import { useState } from 'react'
import { useDevices } from '@/hooks/useDevices'
import { DeviceGrid } from '@/components/DeviceGrid'
import { DeviceDetail } from '@/components/DeviceDetail'

function App() {
  // React Query 返回 { data, isLoading, error }
  // data 就是设备列表（fetchDevices 的返回值）
  const { data: devices, isLoading, error } = useDevices()

  // 当前选中的设备 ID——null 表示没有选中任何设备
  const [selectedDeviceId, setSelectedDeviceId] = useState<string | null>(null)

  // 根据 ID 查找选中的设备对象
  const selectedDevice = selectedDeviceId
    ? (devices || []).find((d: { id: string }) => d.id === selectedDeviceId) || null
    : null

  return (
    <div style={{ maxWidth: 1200, margin: '0 auto', padding: 24 }}>
      <header style={{ marginBottom: 32 }}>
        <h1>LocalPilot Dashboard</h1>
        <p style={{ color: '#666' }}>
          个人设备网格计算平台
        </p>
      </header>

      <main>
        <h2>设备列表</h2>
        {isLoading && <p>加载中...</p>}
        {error && <p style={{ color: 'red' }}>加载失败: {error.message}</p>}
        {!isLoading && !error && (
          <DeviceGrid
            devices={devices || []}
            onDeviceClick={(id) => setSelectedDeviceId(id)}
          />
        )}
      </main>

      {/* 设备详情面板——选中设备时显示 */}
      {selectedDevice && (
        <DeviceDetail
          device={selectedDevice}
          onClose={() => setSelectedDeviceId(null)}
        />
      )}
    </div>
  )
}

export default App
