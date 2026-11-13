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
      <div className="login-heading"><h1 id="login-title">{authenticated ? '当前已登录' : '登录 ECUT OJ'}</h1><p>使用邮箱和密码登录</p></div>
      {state.status !== 'anonymous' || state.notice.tone !== 'neutral' ? <AccountNotice /> : null}
      {authenticated ? <div className="login-destinations"><Link className="button" to="/">前往题库</Link><Link className="button button-neutral" to="/account">管理个人账号</Link></div> : (
        <>
          {state.status !== 'unknown' && state.status !== 'checking' ? <LoginForm controller={controller} disabled={disabled} /> : null}
          <AccountRecovery compact />
          <nav className="login-destinations" aria-label="邮箱账号"><Link className="button button-neutral" to="/account/register">注册账号</Link><Link className="button button-neutral" to="/account/forgot-password">找回密码</Link></nav>
        </>
      )}
      {import.meta.env.VITE_OJ_LOCAL_DEV === '1' && globalThis.location.origin === 'https://localhost:8443' && !authenticated ? <section className="local-login" aria-label="本地开发测试入口"><p>本地开发环境 · 使用独立测试账号</p><button type="button" className="button-neutral" disabled={disabled} onClick={() => void controller.run('local-login')}>一键进入测试账号</button><p>复用真实登录与退出流程，无需输入测试密码。</p></section> : null}
    </section>
  )
}
