import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Link } from 'react-router'
import { requestEmail, type EmailRequestKind, type EmailRequestResult } from '../account/emailRequest'

export function EmailRequestPage({ kind }: { kind: EmailRequestKind }) {
  const registration = kind === 'registration'
  const secure = globalThis.location.protocol === 'https:'
  const [state, setState] = useState<EmailRequestResult | 'ready' | 'pending'>('ready')
  const active = useRef<AbortController | null>(null)
  useEffect(() => () => {
    active.current?.abort()
    active.current = null
  }, [])

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!secure || active.current || (state !== 'ready' && state !== 'invalid')) return
    const form = event.currentTarget
    const email = new FormData(form).get('email')
    if (typeof email !== 'string') return
    const controller = new AbortController()
    active.current = controller
    setState('pending')
    const result = await requestEmail(kind, email, AbortSignal.any([controller.signal, AbortSignal.timeout(12_000)]))
    if (active.current !== controller) return
    active.current = null
    if (result !== 'invalid') form.reset()
    setState(result)
  }

  return (
    <section className="login-page email-request-page" aria-labelledby="email-request-title">
      <div className="login-heading">
        <h1 id="email-request-title">{registration ? '注册账号' : '找回密码'}</h1>
        <p>{registration ? '申请邮箱验证链接，也可以重新申请激活邮件。' : '申请密码重置链接。收到邮件后再设置新密码。'}</p>
      </div>
      {!secure ? <p className="account-notice info" role="status">请通过 HTTPS 访问此页面后再申请。</p> : null}
      {state === 'pending' ? <p className="account-notice info" role="status">正在提交申请…</p> : null}
      {state === 'accepted' ? <p className="account-notice success" role="status">申请已受理。如果符合条件，请留意收件箱和垃圾邮件。若未收到，请稍后再申请。</p> : null}
      {state === 'invalid' ? <p className="account-notice error" role="alert">请输入有效的邮箱地址。</p> : null}
      {state === 'closed' ? <p className="account-notice info" role="status">邮件申请暂未开放，请稍后再试。</p> : null}
      {state === 'unknown' ? <p className="account-notice uncertain" role="alert">无法确认申请是否受理。请先检查邮箱，稍后再决定是否重新申请。</p> : null}
      {secure && (state === 'ready' || state === 'pending' || state === 'invalid') ? (
        <form className="account-form" method="post" aria-busy={state === 'pending'} onSubmit={event => void submit(event)}>
          <fieldset disabled={state === 'pending'}>
            <legend>填写邮箱</legend>
            <label htmlFor="email-request-address">邮箱</label>
            <input id="email-request-address" name="email" type="email" autoComplete="email" spellCheck={false} maxLength={254} required />
            <button type="submit">{registration ? '申请验证邮件' : '申请找回邮件'}</button>
          </fieldset>
        </form>
      ) : null}
      <p className="email-request-return"><Link to="/login">返回登录</Link></p>
    </section>
  )
}
