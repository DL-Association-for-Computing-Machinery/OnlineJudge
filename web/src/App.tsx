import { useState } from 'react'
import { getProcessHealth } from './services/health'

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
        <span className="badge">项目初始化预览</span>
      </header>
      <section className="intro" aria-labelledby="page-title">
        <p className="eyebrow">ONLINE JUDGE / 从一道题开始</p>
        <h1 id="page-title">把每一次练习，<br />变成向前的一步。</h1>
        <p className="description">面向日常训练与校内比赛的在线判题平台。<br />当前正在搭建基础工程，训练功能尚未开放。</p>
      </section>
      <section className="status-card" aria-labelledby="status-title">
        <div>
          <p className="eyebrow">当前进度</p>
          <h2 id="status-title">基础工程 · 连接检查</h2>
          <p>检查前端与 API 是否连通。数据库、登录和判题能力尚未接入。</p>
        </div>
        <div className="connection">
          <p role="status" className={`connection-label ${status}`}>{labels[status]}</p>
          <button type="button" onClick={() => void checkConnection()} disabled={status === 'checking'}>
            {status === 'checking' ? '检查中…' : '检查 API 连接'}<span aria-hidden="true"> ↗</span>
          </button>
        </div>
      </section>
      <footer><span>下一步：验证一条真实提交到结果的路径。</span><span>尚未开放提交</span></footer>
    </main>
  )
}
