import { useState } from 'react'
import { getProcessHealth } from './services/health'
import { AccountPanel } from './account/AccountPanel'

export function App() {
  const [status, setStatus] = useState<'idle' | 'checking' | 'connected' | 'error'>('idle')

  async function checkConnection() {
    setStatus('checking')
    try {
      await getProcessHealth()
      setStatus('connected')
    } catch {
      setStatus('error')
    }
  }

  const labels = {
    idle: '尚未检查 API 连接',
    checking: '正在连接 API…',
    connected: 'API 连接成功',
    error: '连接失败，请确认 API 已启动后重试',
  }

  return (
    <main>
      <header className="masthead">
        <span className="brand">OJ<span className="brand-dot">.</span></span>
        <span>东华理工大学 · 计算机协会</span>
        <a className="account-nav" href="#account-title">个人账号</a>
        <span className="badge">训练平台 · 开发预览</span>
      </header>
      <section className="intro" aria-labelledby="page-title">
        <p className="eyebrow">ONLINE JUDGE / 从一道题开始</p>
        <h1 id="page-title">把每一次练习，<br />变成向前的一步。</h1>
        <p className="description">面向日常训练与校内比赛的在线判题平台。<br />账号能力正在完善，训练功能尚未开放。</p>
      </section>
      <AccountPanel />
      <section className="status-card" aria-labelledby="status-title">
        <div>
          <p className="eyebrow">当前进度</p>
          <h2 id="status-title">基础工程 · 连接检查</h2>
          <p>此检查仅反映 API 进程连通，不代表账号或判题服务可用。</p>
        </div>
        <div className="connection">
          <p role="status" className={`connection-label ${status}`}>{labels[status]}</p>
          <button type="button" onClick={() => void checkConnection()} disabled={status === 'checking'}>
            {status === 'checking' ? '检查中…' : '检查 API 连接'}<span aria-hidden="true"> ↗</span>
          </button>
        </div>
      </section>
      <footer><span>训练题库与提交功能仍在开发。</span><span>尚未开放提交</span></footer>
    </main>
  )
}
