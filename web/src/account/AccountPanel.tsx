import { LoginForm, PasswordForm } from './AccountForms'
import { useAccount } from './useAccount'
import { SessionExpiry } from './SessionExpiry'
import './account.css'

function date(value: string) {
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'medium' }).format(new Date(value))
}

export function AccountPanel() {
  const { state, controller } = useAccount()
  const secure = state.status !== 'insecure'
  const unavailable = state.status === 'unavailable'
  const authenticated = state.status === 'authenticated' && state.session !== null
  const disabled = state.busy || !secure || unavailable
  const labels = {
    checking: '检查中', authenticated: '已登录', anonymous: '未登录', unknown: '状态待确认', unavailable: '入口未开放', insecure: '需要 HTTPS',
  }

  return (
    <section className="account-panel" aria-labelledby="account-title" aria-busy={state.busy}>
      <div className="account-heading">
        <div><p className="eyebrow">个人账号</p><h2 id="account-title" tabIndex={-1}>账号与安全</h2></div>
        <span className={`account-badge ${state.status}`}>{labels[state.status]}</span>
      </div>
      <p className="account-intro">管理当前浏览器的会话与密码。训练、注册和邮箱找回尚未开放。</p>
      <p className={`account-notice ${state.notice.tone}`} role="status" aria-live="polite">{state.notice.text}</p>
      {state.passwordUnknown ? <p className="account-notice uncertain" role="alert">上次改密结果仍待核验。检查会话不能证明密码未改变，请退出并重新登录核验。</p> : null}

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
            <button type="button" className="secondary" disabled={disabled} onClick={() => void controller.run('logout')}>退出当前会话</button>
          </div>
          {state.passwordAvailable ? <PasswordForm controller={controller} disabled={disabled || state.passwordUnknown} /> : <p>此入口尚未启用修改密码。</p>}
        </div>
      ) : state.status === 'anonymous' ? <LoginForm controller={controller} disabled={disabled} /> : null}

      <div className="account-recovery">
        <button type="button" className="secondary" disabled={state.busy || !secure} onClick={() => void controller.run('check')}>检查当前会话</button>
        {state.status === 'unknown' ? <button type="button" className="secondary" disabled={disabled} onClick={() => void controller.run('logout')}>确认退出当前会话</button> : null}
        <details>
          <summary>登录中断，或上次登录尚未完成？</summary>
          <p>先检查当前会话；仍未登录时，可确认原尝试，无须再次输入密码。放弃会撤销原尝试，也可能使该尝试已发放的会话失效；这不等于确认退出当前会话。</p>
          <div className="account-actions">
            <button type="button" className="secondary" disabled={disabled} onClick={() => void controller.run('complete')}>继续确认原尝试</button>
            <button type="button" className="secondary" disabled={disabled} onClick={() => void controller.run('abandon')}>放弃原登录尝试</button>
          </div>
        </details>
      </div>
    </section>
  )
}
