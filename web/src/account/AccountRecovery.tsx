import { useCurrentAccount } from './context'

export function AccountRecovery({ compact = false }: { compact?: boolean }) {
  const { state, controller } = useCurrentAccount()
  const secure = state.status !== 'insecure'
  const disabled = state.busy || !secure || state.status === 'unavailable'
  const controls = <>
    <p>先检查当前会话；仍未登录时，可确认原尝试，无须再次输入密码。放弃会撤销原尝试，也可能使该尝试已发放的会话失效；这不等于确认退出当前会话。</p>
    <div className="account-actions">
      {compact && state.status !== 'unknown' ? <button type="button" className="button-neutral" disabled={state.busy || !secure} onClick={() => void controller.run('check')}>检查当前会话</button> : null}
      <button type="button" className="button-neutral" disabled={disabled} onClick={() => void controller.run('complete')}>继续确认原尝试</button>
      <button type="button" className="button-neutral" disabled={disabled} onClick={() => void controller.run('abandon')}>放弃原登录尝试</button>
    </div>
  </>
  return <div className="account-recovery">
    {compact && state.status !== 'unknown' ? <details><summary>登录中断？</summary><div className="disclosure-content">{controls}</div></details> : <>
      <button type="button" className="button-neutral" disabled={state.busy || !secure} onClick={() => void controller.run('check')}>检查当前会话</button>
      {state.status === 'unknown' ? <button type="button" className="button-neutral" disabled={disabled} onClick={() => void controller.run('logout')}>确认退出当前会话</button> : null}
      <details><summary>登录中断，或上次登录尚未完成？</summary><div className="disclosure-content">{controls}</div></details>
    </>}
  </div>
}
