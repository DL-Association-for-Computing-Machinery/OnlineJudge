import { useCurrentAccount } from './context'

export function AccountNotice() {
  const { state } = useCurrentAccount()
  return <>
    <p className={`account-notice ${state.status === 'insecure' ? 'info' : state.notice.tone}`} role="status" aria-live="polite">{state.notice.text}</p>
    {state.passwordUnknown ? <p className="account-notice uncertain" role="alert">上次改密结果仍待核验。检查会话不能证明密码未改变，请退出并重新登录核验。</p> : null}
  </>
}
