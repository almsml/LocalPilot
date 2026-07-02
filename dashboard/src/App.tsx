// ============================================================
// App.tsx — Dashboard 根组件
//
// Phase 0: 显示设备列表（占位 UI）
// Phase 1: 设备状态实时刷新 + 设备详情面板
// Phase 2: 任务提交 + 终端日志
// Phase 3: 集群拓扑图 + 任务甘特图
// ============================================================

import { useDevices } from '@/hooks/useDevices'
import { DeviceGrid } from '@/components/DeviceGrid'

function App() {
  const { devices, isLoading, error } = useDevices()

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
        {!isLoading && !error && <DeviceGrid devices={devices || []} />}
      </main>
    </div>
  )
}

export default App
