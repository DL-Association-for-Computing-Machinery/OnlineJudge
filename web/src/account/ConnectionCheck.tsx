import { useState } from 'react'
import { getProcessHealth } from '../services/health'

export function ConnectionCheck() {
  const [status, setStatus] = useState<'idle' | 'checking' | 'connected' | 'error'>('idle')
  async function check() {
    setStatus('checking')
    try { await getProcessHealth(); setStatus('connected') }
    catch { setStatus('error') }
  }
  const labels = { idle: '尚未检查 API 连接', checking: '正在连接 API…', connected: 'API 连接成功', error: '连接失败，请确认 API 已启动后重试' }
  return <details className="connection-check"><summary>连接检查</summary><div className="disclosure-content"><p>仅检查 API 进程连通，不代表账号或判题服务可用。</p><p role="status">{labels[status]}</p><button type="button" className="button-neutral" disabled={status === 'checking'} onClick={() => void check()}>检查 API 连接</button></div></details>
}
