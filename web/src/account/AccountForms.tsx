import type { FormEvent } from 'react'
import type { AccountController } from './controller'

export function LoginForm({ controller, disabled }: { controller: AccountController; disabled: boolean }) {
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = event.currentTarget
    const fields = new FormData(form)
    const email = fields.get('email')
    const password = fields.get('password')
    const remember = fields.has('remember')
    if (typeof email !== 'string' || typeof password !== 'string') return
    // Passwords stay out of React state and browser storage. Reset immediately;
    // the browser/password manager still owns its own autofill behavior.
    form.reset()
    void controller.run('login', { email, password, ...(remember ? { remember: true } : {}) })
  }
  return (
    <form onSubmit={submit} className="account-form">
      <fieldset disabled={disabled}>
        <legend>使用已有账号登录</legend>
        <label htmlFor="login-email">邮箱</label>
        <input id="login-email" name="email" type="email" autoComplete="username" required spellCheck={false} />
        <label htmlFor="login-password">密码</label>
        <input id="login-password" name="password" type="password" autoComplete="current-password" required />
        <label><input type="checkbox" name="remember" /> 在此设备上保持登录（最长 7 天）</label>
        <button type="submit">登录</button>
      </fieldset>
    </form>
  )
}

export function PasswordForm({ controller, disabled }: { controller: AccountController; disabled: boolean }) {
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = event.currentTarget
    const fields = new FormData(form)
    const current = fields.get('current_password')
    const replacement = fields.get('new_password')
    const confirmation = fields.get('confirmation')
    const confirmInput = form.elements.namedItem('confirmation')
    if (!(confirmInput instanceof HTMLInputElement)) return
    confirmInput.setCustomValidity(replacement === confirmation ? '' : '两次输入的新密码不一致。')
    if (!form.reportValidity()) return
    if (typeof current !== 'string' || typeof replacement !== 'string') return
    form.reset()
    void controller.run('password', { current_password: current, new_password: replacement })
  }
  return (
    <form onSubmit={submit} className="account-form password-form" onInput={event => {
      const confirmation = event.currentTarget.elements.namedItem('confirmation')
      if (confirmation instanceof HTMLInputElement) confirmation.setCustomValidity('')
    }}>
      <fieldset disabled={disabled}>
        <legend>修改密码</legend>
        <p id="password-help">至少 15 个字符，本站可能采用更严格的长度与弱密码规则。成功后需要重新登录。</p>
        <p>若上次改密中断或结果未确认，请先退出并重新登录核验，勿重复提交。</p>
        <label htmlFor="current-password">当前密码</label>
        <input id="current-password" name="current_password" type="password" autoComplete="current-password" required />
        <label htmlFor="new-password">新密码</label>
        <input id="new-password" name="new_password" type="password" autoComplete="new-password" aria-describedby="password-help" required />
        <label htmlFor="confirm-password">再次输入新密码</label>
        <input id="confirm-password" name="confirmation" type="password" autoComplete="new-password" required onInput={event => event.currentTarget.setCustomValidity('')} />
        <button type="submit">确认修改密码</button>
      </fieldset>
    </form>
  )
}
