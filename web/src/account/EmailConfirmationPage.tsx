import { useRef, useState, type FormEvent } from 'react'
import type { ConfirmationResult, EmailConfirmationPageInput } from './emailConfirmation'
import './account.css'

const retryMessages = {
  'invalid-password': '密码长度不符合站点要求，请重新输入。',
  'weak-password': '请换一个不常见的密码。',
  limited: '请求过于频繁，请稍后再试。',
}

export function EmailConfirmationPage({ kind, confirm }: EmailConfirmationPageInput) {
  const activation = kind === 'activation'
  const title = activation ? '验证邮箱' : '重置密码'
  const [state, setState] = useState<ConfirmationResult | 'ready' | 'pending'>(confirm ? 'ready' : 'invalid-link')
  const busy = useRef(false)
  const retry = state === 'invalid-password' || state === 'weak-password' || state === 'limited'
  const showForm = state === 'ready' || state === 'pending' || retry

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!confirm || busy.current || !showForm) return
    const form = event.currentTarget
    const fields = new FormData(form)
    const password = fields.get('new_password')
    const confirmInput = form.elements.namedItem('confirmation')
    if (typeof password !== 'string' || !(confirmInput instanceof HTMLInputElement)) return
    confirmInput.setCustomValidity(password === confirmInput.value ? '' : '两次输入的密码不一致。')
    if (!form.reportValidity()) return
    busy.current = true
    form.reset()
    setState('pending')
    try {
      setState(await confirm(password))
    } finally {
      busy.current = false
    }
  }

  return (
    <main className="login-page email-confirmation-page" aria-labelledby="email-confirmation-title">
      <div className="login-heading">
        <h1 id="email-confirmation-title">{title}</h1>
        <p>{activation ? '为你的账号设置密码' : '为你的账号设置新密码'}</p>
      </div>
      {state === 'done' ? <p className="account-notice success" role="status">{activation ? '邮箱已验证。请用新密码登录。' : '密码已重置。请用新密码重新登录。'}</p> : null}
      {state === 'unknown' ? <p className="account-notice uncertain" role="alert">无法确认操作是否成功。请先尝试用新密码登录，勿重复提交。</p> : null}
      {state === 'invalid-link' ? <p className="account-notice error" role="alert">链接无效或已过期。请从邮箱打开最新链接，或重新申请。</p> : null}
      {state === 'closed' ? <p className="account-notice info" role="status">邮箱确认暂未开放，请稍后再试。</p> : null}
      {state === 'pending' ? <p className="account-notice info" role="status">正在确认，请稍候…</p> : null}
      {retry ? <p className="account-notice error" role="alert">{retryMessages[state]}</p> : null}
      {showForm ? (
        <form className="account-form" method="post" onSubmit={event => void submit(event)} onInput={event => {
          const input = event.currentTarget.elements.namedItem('confirmation')
          if (input instanceof HTMLInputElement) input.setCustomValidity('')
        }}>
          <fieldset disabled={state === 'pending'}>
            <legend>设置新密码</legend>
            <label htmlFor="email-confirmation-password">新密码</label>
            <input id="email-confirmation-password" name="new_password" type="password" autoComplete="new-password" aria-describedby="email-confirmation-password-help" required />
            <p id="email-confirmation-password-help">至少 15 个字符，避免使用常见密码。</p>
            <label htmlFor="email-confirmation-repeat">再次输入新密码</label>
            <input id="email-confirmation-repeat" name="confirmation" type="password" autoComplete="new-password" required />
            <button type="submit">{activation ? '验证并设置密码' : '确认重置密码'}</button>
          </fieldset>
        </form>
      ) : null}
      <p><a className="email-confirmation-login" href="/login">前往登录</a></p>
    </main>
  )
}
