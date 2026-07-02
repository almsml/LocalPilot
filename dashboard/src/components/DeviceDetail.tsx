// ============================================================
// DeviceDetail.tsx — 设备详情面板
//
// 点击设备卡片后从右侧滑出的详情面板，展示：
//   - 设备基本信息（主机名、OS、架构、CPU 核心数）
//   - 实时资源使用（CPU 使用率进度条、内存使用进度条、温度）
//   - 设备能力（GPU、支持的运行时列表）
//   - 连接信息（Agent 地址、注册时间、最后心跳时间）
//
// 为什么用右侧滑出面板而不是弹窗（Modal）？
//   滑出面板不遮挡设备列表——用户可以一边看设备网格一边看详情，
//   对比不同设备的状态。Modal 会完全覆盖背景，打断用户的视线流。
//
// 为什么不用 React Router 管理详情路由？
//   Phase 1 的设备详情是快速查看面板，不需要深层链接。
//   用组件状态管理更简单，不需要引入 React Router。
// ============================================================

import type { Device } from '@/types'

/** 字节 → 人类可读 */
function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`
}

/** 格式化时间戳为可读的相对时间 */
function timeAgo(dateStr: string): string {
  const date = new Date(dateStr)
  const now = Date.now()
  const diffMs = now - date.getTime()

  if (diffMs < 0) return '刚刚'

  const seconds = Math.floor(diffMs / 1000)
  if (seconds < 5) return '刚刚'
  if (seconds < 60) return `${seconds} 秒前`

  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes} 分钟前`

  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} 小时前`

  const days = Math.floor(hours / 24)
  return `${days} 天前`
}

/** 状态 → 颜色映射 */
const stateColor: Record<string, string> = {
  ONLINE: '#22c55e',
  UNHEALTHY: '#eab308',
  OFFLINE: '#ef4444',
}

interface Props {
  device: Device
  /** 关闭面板回调 */
  onClose: () => void
}

export function DeviceDetail({ device, onClose }: Props) {
  const color = stateColor[device.state] || '#999'
  const cpuPct = (device.cpu_percent * 100).toFixed(1)
  const memPct = device.total_ram_bytes > 0
    ? ((device.used_ram_bytes / device.total_ram_bytes) * 100).toFixed(1)
    : '0'

  return (
    <>
      {/* 半透明遮罩层——点击关闭面板 */}
      {/* 为什么需要遮罩？
          遮罩让用户明确感知到"当前焦点在详情面板"。
          点击遮罩关闭面板是移动端和桌面端通用的交互模式。
        */}
      <div
        onClick={onClose}
        style={{
          position: 'fixed',
          inset: 0,
          background: 'rgba(0,0,0,0.2)',
          zIndex: 999,
        }}
      />

      {/* 详情面板 */}
      <div
        style={{
          position: 'fixed',
          right: 0,
          top: 0,
          height: '100vh',
          width: 400,
          background: '#fff',
          boxShadow: '-4px 0 16px rgba(0,0,0,0.15)',
          padding: '24px 24px 24px 24px',
          overflowY: 'auto',
          zIndex: 1000,
          animation: 'slideIn 0.2s ease-out',
        }}
      >
        {/* ---- 头部：设备名 + 关闭按钮 ---- */}
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginBottom: 20,
          }}
        >
          <h2 style={{ margin: 0, fontSize: 22 }}>{device.hostname}</h2>
          <button
            onClick={onClose}
            style={{
              border: 'none',
              background: 'none',
              fontSize: 24,
              cursor: 'pointer',
              color: '#999',
              padding: '4px 8px',
              borderRadius: 4,
            }}
            title="关闭"
          >
            ✕
          </button>
        </div>

        {/* 状态标签 */}
        <div style={{ marginBottom: 20 }}>
          <span
            style={{
              display: 'inline-block',
              background: color,
              color: '#fff',
              padding: '4px 12px',
              borderRadius: 12,
              fontSize: 14,
              fontWeight: 600,
            }}
          >
            {device.state === 'ONLINE' ? '在线' :
             device.state === 'UNHEALTHY' ? '异常' : '离线'}
          </span>
        </div>

        {/* ---- 分隔线 ---- */}
        <Divider />

        {/* ---- 系统信息 ---- */}
        <Section title="系统信息">
          <InfoRow label="操作系统" value={`${device.os} / ${device.arch}`} />
          <InfoRow label="CPU 核心" value={`${device.cpu_cores} 核`} />
          <InfoRow label="总内存" value={formatBytes(device.total_ram_bytes)} />
        </Section>

        {/* ---- 分隔线 ---- */}
        <Divider />

        {/* ---- 资源使用 ---- */}
        <Section title="资源使用">
          {/* CPU 使用率进度条 */}
          <div style={{ marginBottom: 12 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 13, marginBottom: 4 }}>
              <span>CPU 使用率</span>
              <span style={{ fontWeight: 600 }}>{cpuPct}%</span>
            </div>
            <ProgressBar pct={device.cpu_percent * 100} color="#3b82f6" />
          </div>

          {/* 内存使用进度条 */}
          <div style={{ marginBottom: 12 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 13, marginBottom: 4 }}>
              <span>内存使用</span>
              <span style={{ fontWeight: 600 }}>
                {formatBytes(device.used_ram_bytes)} / {formatBytes(device.total_ram_bytes)} ({memPct}%)
              </span>
            </div>
            <ProgressBar pct={Number(memPct)} color="#8b5cf6" />
          </div>

          {/* 温度 */}
          <InfoRow
            label="CPU 温度"
            value={device.cpu_temperature > 0
              ? `${device.cpu_temperature.toFixed(1)} °C`
              : 'N/A'}
          />

          {/* 运行中任务数 */}
          <InfoRow label="运行中任务" value={`${device.running_task_count} 个`} />
        </Section>

        {/* ---- 分隔线 ---- */}
        <Divider />

        {/* ---- 设备能力 ---- */}
        <Section title="设备能力">
          {device.gpu_info && <InfoRow label="GPU" value={device.gpu_info} />}
          {!device.gpu_info && <InfoRow label="GPU" value="无" />}
          <InfoRow
            label="支持的运行时"
            value={device.supported_runtimes?.length
              ? device.supported_runtimes.join(', ')
              : '未知'}
          />
        </Section>

        {/* ---- 分隔线 ---- */}
        <Divider />

        {/* ---- 连接信息 ---- */}
        <Section title="连接信息">
          <InfoRow label="Agent 地址" value={device.agent_address} mono />
          <InfoRow label="注册时间" value={new Date(device.registered_at).toLocaleString()} />
          <InfoRow label="最后心跳" value={timeAgo(String(device.last_heartbeat))} />
        </Section>
      </div>

      {/* 滑入动画的 keyframes */}
      <style>{`
        @keyframes slideIn {
          from { transform: translateX(100%); }
          to { transform: translateX(0); }
        }
      `}</style>
    </>
  )
}

// ============================================================
// 内部子组件
// ============================================================

/** 分区标题 */
function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div style={{ marginBottom: 16 }}>
      <h4 style={{
        margin: '0 0 8px 0',
        fontSize: 13,
        color: '#999',
        textTransform: 'uppercase',
        letterSpacing: '0.05em',
      }}>
        {title}
      </h4>
      {children}
    </div>
  )
}

/** 分隔线 */
function Divider() {
  return (
    <hr style={{
      border: 'none',
      borderTop: '1px solid #eee',
      margin: '16px 0',
    }} />
  )
}

/** 键值行 */
function InfoRow({
  label,
  value,
  mono,
}: {
  label: string
  value: string
  mono?: boolean
}) {
  return (
    <div style={{
      display: 'flex',
      justifyContent: 'space-between',
      fontSize: 13,
      lineHeight: 2,
    }}>
      <span style={{ color: '#888' }}>{label}</span>
      <span style={{
        fontWeight: 500,
        fontFamily: mono ? 'monospace' : undefined,
        color: mono ? '#6366f1' : '#333',
      }}>
        {value}
      </span>
    </div>
  )
}

/** 进度条 */
function ProgressBar({ pct, color }: { pct: number; color: string }) {
  // 限制在 0-100 之间，防止异常数据显示越界
  const clampedPct = Math.max(0, Math.min(100, pct))

  return (
    <div style={{
      height: 8,
      background: '#f1f5f9',
      borderRadius: 4,
      overflow: 'hidden',
    }}>
      <div style={{
        height: '100%',
        width: `${clampedPct}%`,
        background: color,
        borderRadius: 4,
        transition: 'width 0.5s ease',
      }} />
    </div>
  )
}
