import { Link } from 'react-router'
import { LoginForm } from '../account/AccountForms'
import { AccountRecovery } from '../account/AccountRecovery'
import { AccountNotice } from '../account/AccountNotice'
import { useCurrentAccount } from '../account/context'

export function LoginPage() {
  const { state, controller } = useCurrentAccount()
  const authenticated = state.status === 'authenticated'
  const disabled = state.busy || state.status !== 'anonymous'
  return (
    <section className="login-page" aria-labelledby="login-title" aria-busy={state.busy}>
      <div className="login-heading"><h1 id="login-title">{authenticated ? '当前已登录' : '登录 ECUT OJ'}</h1><p>使用已有账号登录</p></div>
      {state.status !== 'anonymous' || state.notice.tone !== 'neutral' ? <AccountNotice /> : null}
      {authenticated ? <div className="login-destinations"><Link className="button" to="/">前往题库</Link><Link className="button button-neutral" to="/account">管理个人账号</Link></div> : (
        <>
          {state.status !== 'unknown' && state.status !== 'checking' ? <LoginForm controller={controller} disabled={disabled} /> : null}
          <AccountRecovery compact />
        </>
      )}
    </section>
  )
}
