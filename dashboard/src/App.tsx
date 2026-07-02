// ============================================================
// App.tsx — Dashboard 根组件
//
// Phase 2: 设备列表 + 任务提交 + 终端日志
// ============================================================

import { useState } from 'react'
import { useDevices } from '@/hooks/useDevices'
import { DeviceGrid } from '@/components/DeviceGrid'
import { DeviceDetail } from '@/components/DeviceDetail'
import { JobSubmitter } from '@/components/JobSubmitter'
import { TerminalLog } from '@/components/TerminalLog'
import { ClusterTopology } from '@/components/ClusterTopology'

type Tab = 'devices' | 'jobs' | 'topology'

function App() {
  const { data: devices, isLoading, error } = useDevices()
  const [selectedDeviceId, setSelectedDeviceId] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState<Tab>('devices')
  const [trackedJobId, setTrackedJobId] = useState<string | null>(null)

  const selectedDevice = selectedDeviceId
    ? (devices || []).find((d: { id: string }) => d.id === selectedDeviceId) || null
    : null

  return (
    <div style={{ maxWidth: 1200, margin: '0 auto', padding: 24 }}>
      <header style={{ marginBottom: 24 }}>
        <h1>LocalPilot Dashboard</h1>
        <p style={{ color: '#666' }}>个人设备网格计算平台</p>
      </header>

      {/* Tab 切换 */}
      <div style={{ display: 'flex', gap: 0, marginBottom: 24, borderBottom: '2px solid #eee' }}>
        <TabButton label="设备列表" active={activeTab === 'devices'} onClick={() => setActiveTab('devices')} />
        <TabButton label="任务管理" active={activeTab === 'jobs'} onClick={() => setActiveTab('jobs')} />
        <TabButton label="集群拓扑" active={activeTab === 'topology'} onClick={() => setActiveTab('topology')} />
      </div>

      <main>
        {activeTab === 'devices' && (
          <>
            <h2>设备列表</h2>
            {isLoading && <p>加载中...</p>}
            {error && <p style={{ color: 'red' }}>加载失败: {error.message}</p>}
            {!isLoading && !error && (
              <DeviceGrid
                devices={devices || []}
                onDeviceClick={(id) => setSelectedDeviceId(id)}
              />
            )}
          </>
        )}

        {activeTab === 'jobs' && (
          <>
            <h2>任务管理</h2>
            <JobSubmitter onJobSubmitted={(id) => setTrackedJobId(id)} />
            {trackedJobId && (
              <>
                <h3 style={{ marginTop: 24 }}>任务日志</h3>
                <TerminalLog jobId={trackedJobId} />
              </>
            )}
            <QuickJobForm onJobCreated={(id) => setTrackedJobId(id)} />
          </>
        )}

        {activeTab === 'topology' && (
          <>
            <h2>集群拓扑</h2>
            <p style={{ color: '#666', fontSize: 14, marginBottom: 16 }}>
              Controller 居中，Agent 围绕分布。连线颜色 = 设备状态，绿线动画 = 在线设备。
            </p>
            {!isLoading && <ClusterTopology devices={devices || []} />}
          </>
        )}
      </main>

      {/* 设备详情面板 */}
      {selectedDevice && (
        <DeviceDetail
          device={selectedDevice}
          onClose={() => setSelectedDeviceId(null)}
        />
      )}
    </div>
  )
}

// ---- Tab 按钮 ----

function TabButton({ label, active, onClick }: { label: string; active: boolean; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      style={{
        padding: '10px 20px',
        border: 'none',
        background: 'none',
        fontSize: 15,
        fontWeight: active ? 600 : 400,
        color: active ? '#3b82f6' : '#666',
        borderBottom: active ? '2px solid #3b82f6' : '2px solid transparent',
        marginBottom: -2,
        cursor: 'pointer',
        transition: 'all 0.15s',
      }}
    >
      {label}
    </button>
  )
}

// ---- 快速任务提交 + 日志跟踪 ----

function QuickJobForm({ onJobCreated }: { onJobCreated: (id: string) => void }) {
  const [cmd, setCmd] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const handleSubmit = async () => {
    if (!cmd.trim()) return
    setSubmitting(true)
    try {
      // 动态 import 避免循环依赖
      const { submitJob } = await import('@/api/client')
      const args = cmd.trim().split(/\s+/)
      const job = await submitJob(args[0], args.slice(1))
      onJobCreated(job.id)
    } catch (_) {
      // 忽略错误——JobSubmitter 组件也有更详细的表单
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div style={{ marginTop: 16, display: 'flex', gap: 8 }}>
      <input
        value={cmd}
        onChange={(e) => setCmd(e.target.value)}
        onKeyDown={(e) => e.key === 'Enter' && handleSubmit()}
        placeholder="输入命令，如: echo hello"
        style={{
          flex: 1,
          padding: '8px 12px',
          border: '1px solid #ddd',
          borderRadius: 6,
          fontSize: 14,
          fontFamily: 'monospace',
        }}
      />
      <button
        onClick={handleSubmit}
        disabled={submitting || !cmd.trim()}
        style={{
          padding: '8px 16px',
          background: '#3b82f6',
          color: '#fff',
          border: 'none',
          borderRadius: 6,
          cursor: 'pointer',
          fontWeight: 600,
          whiteSpace: 'nowrap',
        }}
      >
        {submitting ? '...' : '执行'}
      </button>
    </div>
  )
}

export default App
