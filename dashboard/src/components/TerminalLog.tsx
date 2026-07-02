// ============================================================
// TerminalLog.tsx — 终端日志查看器
//
// 显示任务的 stdout/stderr 输出，黑色背景 + 等宽字体，
// 模拟真实终端的外观。
//
// 为什么用轮询而不是 WebSocket？
//   Phase 2 先跑通端到端链路。Dashboard 每 1 秒轮询
//   GET /api/jobs/:id 获取最新日志。
//   WebSocket 实时推送到 Phase 2 后期再加。
// ============================================================

import { useEffect, useRef, useState } from 'react'
import { fetchJob } from '@/api/client'
import type { Job } from '@/types'

interface Props {
  jobId: string
}

export function TerminalLog({ jobId }: Props) {
  const [job, setJob] = useState<Job | null>(null)
  const [error, setError] = useState<string | null>(null)
  const bottomRef = useRef<HTMLDivElement>(null)

  // 每 1 秒轮询任务状态和日志
  useEffect(() => {
    let active = true

    const poll = async () => {
      try {
        const j = await fetchJob(jobId)
        if (!active) return
        setJob(j)

        // 任务终结后停止轮询
        if (j.status === 'COMPLETED' || j.status === 'FAILED' || j.status === 'CANCELLED') {
          return
        }
      } catch (err: any) {
        if (!active) return
        setError(err.message)
      }
    }

    poll()
    const timer = setInterval(poll, 1000)

    return () => {
      active = false
      clearInterval(timer)
    }
  }, [jobId])

  // 自动滚动到底部
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [job?.logs?.length])

  const stateColor: Record<string, string> = {
    QUEUED: '#999',
    ASSIGNED: '#eab308',
    RUNNING: '#3b82f6',
    COMPLETED: '#22c55e',
    FAILED: '#ef4444',
    CANCELLED: '#f97316',
  }

  return (
    <div style={{ marginBottom: 20 }}>
      {/* 状态栏 */}
      {job && (
        <div style={{
          display: 'flex',
          justifyContent: 'space-between',
          padding: '8px 12px',
          background: '#f1f5f9',
          borderRadius: '8px 8px 0 0',
          fontSize: 13,
        }}>
          <span>任务: {job.command} {job.args?.join(' ')}</span>
          <span style={{ color: stateColor[job.status] || '#999', fontWeight: 600 }}>
            {job.status}
            {job.status === 'COMPLETED' && ` (exit: ${job.exit_code})`}
          </span>
        </div>
      )}

      {/* 错误状态 */}
      {error && (
        <div style={{ padding: 12, color: '#ef4444', fontSize: 13 }}>
          获取日志失败: {error}
        </div>
      )}

      {/* 终端输出 */}
      <div style={{
        background: '#1e1e1e',
        color: '#d4d4d4',
        fontFamily: '"Cascadia Code", "Fira Code", "Consolas", monospace',
        fontSize: 13,
        lineHeight: 1.6,
        padding: 16,
        borderRadius: job ? '0 0 8px 8px' : 8,
        minHeight: 200,
        maxHeight: 400,
        overflowY: 'auto',
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-all',
      }}>
        {(!job || job.logs?.length === 0) && (
          <span style={{ color: '#666' }}>等待任务输出...</span>
        )}

        {job?.logs?.map((line, i) => (
          <span
            key={i}
            style={{
              color: line.stream_type === 'stderr' ? '#f87171' : '#d4d4d4',
            }}
          >
            {line.data}
          </span>
        ))}

        <div ref={bottomRef} />
      </div>
    </div>
  )
}
