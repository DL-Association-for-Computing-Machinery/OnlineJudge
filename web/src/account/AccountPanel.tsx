import { useState } from 'react'
import { LoginForm, PasswordForm } from './AccountForms'
import { useCurrentAccount } from './context'
import { AccountNotice } from './AccountNotice'
import { AccountRecovery } from './AccountRecovery'
import { ConnectionCheck } from './ConnectionCheck'
import { SessionExpiry } from './SessionExpiry'
import './account.css'

function date(value: string) {
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'medium' }).format(new Date(value))
}

export function AccountPanel() {
  const { state, controller } = useCurrentAccount()
  const [retainFor, setRetainFor] = useState<string | null>(null)
  const [manual, setManual] = useState<{ owner: string; text: string } | null>(null)
  const [clearing, setClearing] = useState(false)
  const secure = state.status !== 'insecure'
  const unavailable = state.status === 'unavailable'
  const authenticated = state.status === 'authenticated' && state.session !== null
  const disabled = clearing || state.busy || !secure || unavailable
  const labels = {
    checking: '检查中', authenticated: '已登录', anonymous: '未登录', unknown: '状态待确认', unavailable: '入口未开放', insecure: '需要 HTTPS',
  }

  return (
    <section className="account-panel account-page page-width" aria-labelledby="account-title" aria-busy={state.busy}>
      <div className="account-heading">
        <h1 id="account-title" tabIndex={-1}>账号与安全</h1>
        <span className={`account-badge ${state.status}`}>{labels[state.status]}</span>
      </div>
      <p className="account-intro">管理当前浏览器的会话与密码。</p>
      <AccountNotice />

      {authenticated && state.session ? (
        <div className="session-summary">
          <h3>当前会话</h3>
          <dl>
            <div><dt>账号标识</dt><dd className="subject-id">{state.session.subject_id}</dd></div>
            <div><dt>登录时间</dt><dd>{date(state.session.created_at)}</dd></div>
            <div><dt>空闲到期</dt><dd>{date(state.session.idle_expires_at)}</dd></div>
            <div><dt>最长有效至</dt><dd>{date(state.session.absolute_deadline)}</dd></div>
          </dl>
          <p>到期后需要重新登录。检查会话、打开页面或停留在此页面不会自动续期。</p>
          <SessionExpiry key={state.session.idle_expires_at} deadline={state.session.idle_expires_at} />
          <div className="account-actions">
            <button type="button" disabled={disabled} onClick={() => void controller.run('activity')}>继续本次会话</button>
            <button type="button" className="secondary" disabled={disabled} onClick={() => {
              if (!state.session) return
              const clear = retainFor !== state.session.subject_id
              setRetainFor(null)
              void controller.run('logout', undefined, clear)
            }}>退出当前会话</button>
          </div>
          <label><input type="checkbox" checked={retainFor !== state.session.subject_id} disabled={disabled}
            onChange={event => setRetainFor(event.currentTarget.checked ? null : state.session?.subject_id ?? null)} /> 清除此账号本地草稿</label>
          <p>退出前请下载需要保留的未保存代码，或取消清理选项。</p>
          <div className="account-actions">
            <button type="button" className="secondary" disabled={disabled} onClick={() => {
              const owner = state.session?.subject_id
              if (!owner || !window.confirm('清理此账号在本浏览器里的草稿？请先下载需要保留的代码。')) return
              setClearing(true)
              void controller.clearDrafts(owner).then(
                () => setManual({ owner, text: '此账号的本地代码草稿已清理。' }),
                () => setManual({ owner, text: '本地代码草稿清理未确认。' }),
              ).finally(() => setClearing(false))
            }}>立即清理本地草稿</button>
          </div>
          {manual?.owner === state.session.subject_id ? <p role="status">{manual.text}</p> : null}
          {state.passwordAvailable ? <PasswordForm controller={controller} disabled={disabled || state.passwordUnknown} /> : <p>此入口尚未启用修改密码。</p>}
        </div>
      ) : state.status === 'anonymous' ? <LoginForm controller={controller} disabled={disabled} /> : null}

      <AccountRecovery />
      <ConnectionCheck />
    </section>
  )
}
