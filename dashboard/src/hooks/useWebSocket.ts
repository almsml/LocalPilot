// ============================================================
// useWebSocket.ts — WebSocket 连接 Hook
//
// Phase 2 实现。用于接收 Controller 推送的：
//   - 设备上下线事件（device_state_change）
//   - 实时任务日志流（task_log）
//
// 为什么 Phase 1 只有类型定义和骨架？
//   Phase 1 用 HTTP 轮询（refetchInterval: 2s）获取设备状态足够了。
//   WebSocket 实时推送主要用于两件事：
//     1) 设备上下线即时通知（比轮询快 2 秒，但用户体验差异不大）
//     2) 任务日志 streaming（Phase 2 的核心功能）
//   Phase 1 先定义接口契约，Phase 2 实现完整连接。
//
// WebSocket 连接架构（Phase 2 计划）：
//   - Dashboard 通过 vite proxy (/ws → ws://localhost:8080/ws) 连接 Controller
//   - Controller 维护 WebSocket hub，广播设备状态变化和任务日志
//   - 前端通过 React context 或 zustand 分发事件到各组件
//   - 自动重连机制：指数退避（1s → 2s → 4s → max 30s）
// ============================================================

// ---- 事件类型定义 ----

/** 设备状态变化事件 */
export interface DeviceStateEvent {
  type: 'device_state_change'
  device_id: string
  old_state: string
  new_state: string
  timestamp: number
}

/** 任务日志流事件 */
export interface TaskLogEvent {
  type: 'task_log'
  task_id: string
  stream_type: 'stdout' | 'stderr'
  data: string
  seq_num: number
}

/** WebSocket 消息联合类型 */
export type WSEvent = DeviceStateEvent | TaskLogEvent

// ---- Hook ----

/** useWebSocket — Phase 1 骨架，Phase 2 实现完整连接 */
export function useWebSocket() {
  // TODO(Phase 2):
  //   1. 用 new WebSocket('ws://localhost:8080/ws') 建立连接
  //   2. 监听 onmessage 事件解析 JSON (WSEvent 类型)
  //   3. 自动重连（指数退避: 1s → 2s → 4s → max 30s）
  //   4. 通过 React context 或 event emitter 分发事件到各组件
  //   5. 连接状态管理: isConnected, reconnectAttempts
  return {
    /** 是否已连接到 Controller 的 WebSocket */
    isConnected: false,
    /** 最近收到的事件（null 表示尚未收到任何事件） */
    lastEvent: null as WSEvent | null,
  }
}
