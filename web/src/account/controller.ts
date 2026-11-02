import { AccountError, accountRequest } from '../services/session'
import type { AccountInput, SessionView } from '../services/session'

export type AccountAction = 'check' | 'login' | 'local-login' | 'complete' | 'abandon' | 'activity' | 'logout' | 'password'
type SessionStatus = 'checking' | 'authenticated' | 'anonymous' | 'unknown' | 'unavailable' | 'insecure'
export interface AccountState {
  status: SessionStatus
  session: SessionView | null
  busy: boolean
  notice: { tone: 'neutral' | 'success' | 'error' | 'uncertain'; text: string }
  passwordUnknown: boolean
  passwordAvailable: boolean
}

// This controller owns one synchronous in-flight slot for an entire user action,
// including the dependent Begin -> Complete -> Read chain. It holds no inputs.
export class AccountController {
  private state: AccountState = {
    status: 'checking', session: null, busy: false,
    notice: { tone: 'neutral', text: '正在检查当前会话…' },
    passwordUnknown: false, passwordAvailable: true,
  }
  private listeners = new Set<() => void>()
  private connected = false
  private running = false
  private generation = 0
  private abort: AbortController | null = null
  private action: AccountAction | null = null

  getSnapshot = (): AccountState => this.state
  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener)
    return () => this.listeners.delete(listener)
  }

  connect(): void {
    this.connected = true
    void this.run('check')
  }

  disconnect(): void {
    this.connected = false
    ++this.generation
    this.abort?.abort()
    this.abort = null
    this.running = false
    this.state = { ...this.state, busy: false,
      ...(this.action && this.action !== 'check' ? {
        status: 'unknown' as const,
        notice: { tone: 'uncertain' as const, text: '上次操作已中断，结果未确认。请检查当前会话，勿重复提交。' },
      } : {}),
      ...(this.action === 'password' ? { passwordUnknown: true } : {}),
    }
    this.action = null
  }

  private publish(patch: Partial<AccountState>): void {
    this.state = { ...this.state, ...patch }
    for (const listener of this.listeners) listener()
  }

  async run(action: AccountAction, input?: AccountInput): Promise<void> {
    if (!this.connected || this.running || (action === 'password' && this.state.passwordUnknown)) return
    this.running = true
    const generation = ++this.generation
    const abort = new AbortController()
    this.abort = abort
    this.action = action
    const current = () => this.connected && this.generation === generation
    this.publish({ busy: true, notice: { tone: 'neutral', text: '正在处理，请稍候…' } })
    let issued = false
    try {
      switch (action) {
        case 'login':
        case 'local-login':
          await accountRequest(action === 'login' ? 'begin' : 'local-begin', abort.signal, action === 'login' ? input : undefined)
          if (!current()) return
          await accountRequest('complete', abort.signal)
          issued = true
          if (!current()) return
          this.acceptSession(await accountRequest('read', abort.signal), current, true)
          break
        case 'complete':
          await accountRequest('complete', abort.signal)
          issued = true
          if (!current()) return
          this.acceptSession(await accountRequest('read', abort.signal), current, true)
          break
        case 'check':
        case 'activity':
          this.acceptSession(await accountRequest(action === 'check' ? 'read' : 'activity', abort.signal), current, false)
          break
        case 'abandon':
          await accountRequest('abandon', abort.signal)
          if (current()) this.publish({ status: 'unknown', notice: { tone: 'success', text: '原登录尝试已放弃；这不等于确认退出，请检查当前会话。' } })
          break
        case 'logout':
          await accountRequest('logout', abort.signal)
          if (current()) this.publish({ status: 'anonymous', session: null, notice: { tone: 'success', text: '已退出当前会话。' } })
          break
        case 'password':
          await accountRequest('password', abort.signal, input)
          if (current()) this.publish({ status: 'anonymous', session: null, passwordUnknown: false,
            notice: { tone: 'success', text: '密码已修改，原会话已失效。请使用新密码重新登录。' } })
          break
      }
    } catch (caught) {
      if (!current()) return
      const error = caught instanceof AccountError ? caught : new AccountError('unknown', 'RESULT_UNCONFIRMED', '结果暂时无法确认。')
      const unknown = error.kind === 'unknown'
      const anonymousObservation = action === 'check' && error.code === 'SESSION_REQUIRED'
      let status = this.state.status
      let session = this.state.session
      // A password refusal can mean a wrong current password, not a dead session.
      // Optional password 404 and Abandon do not describe the ordinary session.
      if (error.kind === 'insecure') { status = 'insecure'; session = null }
      else if ((action === 'check' || action === 'activity' || action === 'logout' || issued) && error.code === 'SESSION_REQUIRED') { status = 'anonymous'; session = null }
      else if ((action === 'check' || action === 'activity') && error.code === 'NOT_FOUND') { status = 'unavailable'; session = null }
      else if (unknown || action === 'check' || action === 'activity' ||
        ['ALREADY_AUTHENTICATED', 'LOGIN_PENDING', 'SESSION_LIMIT', 'LOGIN_ATTEMPT_CONFLICT'].includes(error.code)) status = 'unknown'
      let text = error.message
      if (issued) text = `登录发放已确认，但当前会话尚未核验。${error.message}请检查当前会话，勿重新提交登录。`
      else if (unknown && action === 'password') text = '改密结果无法确认，密码可能已经修改。请退出并重新登录核验，勿重复提交改密。'
      else if (unknown && action === 'logout') text = '退出结果无法确认。请检查当前会话，暂不能视为已退出。'
      else if (action === 'logout' && error.code === 'SESSION_REQUIRED') text = '当前没有可用会话，但这不确认之前的退出操作。可检查后重新登录。'
      else if (unknown && (action === 'login' || action === 'local-login' || action === 'complete')) text = '登录结果无法确认。请检查当前会话，或继续确认原尝试；勿重新提交密码。'
      else if (action === 'abandon' && error.code === 'LOGIN_ATTEMPT_INVALID') text = '原尝试未能确认清理，可能已过期或不存在。浏览器中的受保护凭据不能由页面代为删除。'
      else if (anonymousObservation) text = '尚未登录。请使用已有账号，或继续确认原尝试。'
      this.publish({ status, session, notice: { tone: unknown ? 'uncertain' : anonymousObservation ? 'neutral' : 'error', text },
        ...(action === 'password' && unknown ? { passwordUnknown: true } : {}),
        ...(action === 'password' && error.code === 'NOT_FOUND' ? { passwordAvailable: false } : {}),
      })
    } finally {
      if (current()) {
        this.running = false
        this.abort = null
        this.action = null
        this.publish({ busy: false })
      }
    }
  }

  private acceptSession(session: SessionView, current: () => boolean, login: boolean): void {
    if (!current()) return
    this.publish({ status: 'authenticated', session,
      notice: { tone: 'success', text: login ? '登录成功，当前会话已核验。' : '当前会话有效。检查不会延长会话期限。' },
      ...(login ? { passwordUnknown: false } : {}),
    })
  }
}
