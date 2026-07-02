// ============================================================
// JobSubmitter.tsx — 任务提交表单
//
// 让用户输入命令、参数、环境变量并提交到 Controller。
// Phase 2: 基础文本输入 + 提交按钮。
// ============================================================

import { useState } from 'react'
import { submitJob } from '@/api/client'

interface Props {
  onJobSubmitted?: (jobId: string) => void
}

export function JobSubmitter({ onJobSubmitted }: Props) {
  const [command, setCommand] = useState('echo')
  const [argsStr, setArgsStr] = useState('hello localpilot')
  const [envStr, setEnvStr] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [result, setResult] = useState<string | null>(null)

  const handleSubmit = async () => {
    setSubmitting(true)
    setResult(null)
    try {
      const args = argsStr.trim() ? argsStr.split(/\s+/) : []
      const env: Record<string, string> = {}
      if (envStr.trim()) {
        envStr.split(',').forEach((pair) => {
          const [k, v] = pair.split('=')
          if (k && v) env[k.trim()] = v.trim()
        })
      }

      const job = await submitJob(command, args, env)
      setResult(`任务已提交: ${job.id.slice(0, 8)}... → 状态: ${job.status}`)
      onJobSubmitted?.(job.id)
    } catch (err: any) {
      setResult(`提交失败: ${err.message}`)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div style={{ background: '#f8f9fa', borderRadius: 12, padding: 20, marginBottom: 20 }}>
      <h3 style={{ margin: '0 0 16px 0' }}>提交任务</h3>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
        <div>
          <label style={{ fontSize: 13, color: '#666', display: 'block', marginBottom: 4 }}>命令</label>
          <input
            value={command}
            onChange={(e) => setCommand(e.target.value)}
            placeholder="echo"
            style={inputStyle}
          />
        </div>

        <div>
          <label style={{ fontSize: 13, color: '#666', display: 'block', marginBottom: 4 }}>参数（空格分隔）</label>
          <input
            value={argsStr}
            onChange={(e) => setArgsStr(e.target.value)}
            placeholder="hello world"
            style={inputStyle}
          />
        </div>

        <div>
          <label style={{ fontSize: 13, color: '#666', display: 'block', marginBottom: 4 }}>环境变量（逗号分隔，如 KEY=VALUE）</label>
          <input
            value={envStr}
            onChange={(e) => setEnvStr(e.target.value)}
            placeholder="FOO=bar,DEBUG=1"
            style={inputStyle}
          />
        </div>

        <button
          onClick={handleSubmit}
          disabled={submitting || !command}
          style={{
            padding: '10px 20px',
            background: command ? '#3b82f6' : '#ccc',
            color: '#fff',
            border: 'none',
            borderRadius: 8,
            cursor: command ? 'pointer' : 'not-allowed',
            fontSize: 14,
            fontWeight: 600,
          }}
        >
          {submitting ? '提交中...' : '提交任务'}
        </button>

        {result && (
          <div style={{
            fontSize: 13,
            color: result.startsWith('提交失败') ? '#ef4444' : '#22c55e',
            padding: '8px 12px',
            background: result.startsWith('提交失败') ? '#fef2f2' : '#f0fdf4',
            borderRadius: 6,
          }}>
            {result}
          </div>
        )}
      </div>
    </div>
  )
}

const inputStyle: React.CSSProperties = {
  width: '100%',
  padding: '8px 12px',
  border: '1px solid #ddd',
  borderRadius: 6,
  fontSize: 14,
  boxSizing: 'border-box',
  fontFamily: 'monospace',
}
