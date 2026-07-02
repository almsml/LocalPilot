// ============================================================
// ClusterTopology.tsx — 集群拓扑图（Phase 5）
//
// 用 React Flow 可视化设备网格：Controller 是中心节点，
// 各 Agent 设备围绕 Controller 分布，连线表示心跳连接。
// 节点颜色反映设备状态（绿色 ONLINE / 黄色 UNHEALTHY / 红色 OFFLINE）。
//
// 为什么用 React Flow 而不是手写 Canvas？
//   React Flow 提供开箱即用的节点拖拽、缩放、连线动画。
//   手写 Canvas 在这类需求上要花大量时间在交互细节上。
// ============================================================

import { useMemo } from 'react'
import {
  ReactFlow,
  Background,
  Controls,
  type Node,
  type Edge,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import type { Device } from '@/types'

interface Props {
  devices: Device[]
}

/** 设备状态 → 节点颜色 */
const stateColor: Record<string, string> = {
  ONLINE: '#22c55e',
  UNHEALTHY: '#eab308',
  OFFLINE: '#ef4444',
}

export function ClusterTopology({ devices }: Props) {
  // 每次设备列表变化时重新计算节点和边
  const { nodes, edges } = useMemo(() => buildGraph(devices), [devices])

  if (devices.length === 0) {
    return (
      <div style={{ padding: 48, textAlign: 'center', color: '#999', height: 400 }}>
        <p>暂无设备连接</p>
        <p style={{ fontSize: 14 }}>启动 Agent 后拓扑图会自动显示</p>
      </div>
    )
  }

  return (
    <div style={{ height: 500, border: '1px solid #eee', borderRadius: 12, overflow: 'hidden' }}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        fitView
        nodesDraggable={false}
        nodesConnectable={false}
        elementsSelectable={false}
        proOptions={{ hideAttribution: true }}
      >
        <Background color="#f1f5f9" gap={20} />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  )
}

// ---- 构建图数据 ----

function buildGraph(devices: Device[]): { nodes: Node[]; edges: Edge[] } {
  const nodes: Node[] = []
  const edges: Edge[] = []

  // Controller 中心节点
  const centerX = 400
  const centerY = 250

  nodes.push({
    id: 'controller',
    type: 'default',
    position: { x: centerX - 60, y: centerY - 25 },
    data: { label: '🖥️ Controller' },
    style: {
      background: '#1e293b',
      color: '#fff',
      border: '2px solid #334155',
      borderRadius: 12,
      padding: 12,
      fontSize: 14,
      fontWeight: 600,
      width: 160,
    },
  })

  // Agent 设备节点——围绕 Controller 均匀分布
  const radius = 200
  devices.forEach((device, i) => {
    const angle = (2 * Math.PI * i) / devices.length - Math.PI / 2
    const x = centerX + radius * Math.cos(angle) - 70
    const y = centerY + radius * Math.sin(angle) - 25

    const color = stateColor[device.state] || '#999'
    const cpuPct = (device.cpu_percent * 100).toFixed(0)
    const memGB = (device.used_ram_bytes / (1024 * 1024 * 1024)).toFixed(1)
    const totalGB = (device.total_ram_bytes / (1024 * 1024 * 1024)).toFixed(1)

    nodes.push({
      id: device.id,
      type: 'default',
      position: { x, y },
      data: {
        label: `${device.hostname}\nCPU: ${cpuPct}% | RAM: ${memGB}/${totalGB}GB`,
      },
      style: {
        background: '#fff',
        color: '#333',
        border: `2px solid ${color}`,
        borderRadius: 10,
        padding: '8px 12px',
        fontSize: 12,
        width: 180,
      },
    })

    // Controller → Agent 连线
    edges.push({
      id: `e-controller-${device.id}`,
      source: 'controller',
      target: device.id,
      animated: device.state === 'ONLINE',
      style: {
        stroke: color,
        strokeWidth: 2,
      },
    })
  })

  return { nodes, edges }
}
