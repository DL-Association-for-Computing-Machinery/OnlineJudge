import { AccountError, accountRequest, pendingDevicesRequest } from '../services/session'
import type { AccountInput, SessionDevice, SessionView } from '../services/session'
import { invalidatePrivateContent } from './privateInvalidation'

export type AccountAction = 'check' | 'login' | 'local-login' | 'complete' | 'abandon' | 'activity' | 'logout' | 'password' | 'pending-devices' | 'replace-device'
type SessionStatus = 'checking' | 'authenticated' | 'anonymous' | 'unknown' | 'unavailable' | 'insecure'
export interface AccountState {
  status: SessionStatus
  session: SessionView | null
  busy: boolean
  notice: { tone: 'neutral' | 'success' | 'error' | 'uncertain'; text: string }
  passwordUnknown: boolean
  passwordAvailable: boolean
  pendingFull?: boolean
  pendingDevices?: SessionDevice[]
  draftCleanup?: 'complete' | 'unknown'
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
  private privateGeneration = 0

  revokePrivateGeneration = () => {
    ++this.privateGeneration
    if (this.state.pendingDevices) this.publish({ pendingDevices: undefined })
  }
  readPrivateGeneration = () => this.privateGeneration

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
    this.state = { ...this.state, busy: false, pendingFull: undefined, pendingDevices: undefined,
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

  async clearDrafts(owner: string): Promise<void> {
    const { clearAccountCodeDrafts } = await import('../preview/codeDraftStore')
    if (!this.connected || this.running || this.state.status !== 'authenticated' || this.state.session?.subject_id !== owner) throw new Error('Account changed')
    const generation = this.readPrivateGeneration()
    const session = await accountRequest('read', AbortSignal.timeout(5_000))
    if (!this.connected || this.running || this.readPrivateGeneration() !== generation || session.subject_id !== owner ||
      this.state.status !== 'authenticated' || this.state.session?.subject_id !== owner) throw new Error('Account changed')
    invalidatePrivateContent()
    try { await clearAccountCodeDrafts(owner) } finally { invalidatePrivateContent() }
  }

  async run(action: AccountAction, input?: AccountInput, clearDrafts = false): Promise<void> {
    if (!this.connected || this.running || (action === 'password' && this.state.passwordUnknown)) return
    if ((action === 'pending-devices' || action === 'replace-device') && !this.state.pendingFull) return
    if (action === 'replace-device' && (!input || !('device' in input) ||
        !this.state.pendingDevices?.some(device => device.id === input.device))) return
    this.running = true
    const generation = ++this.generation
    const abort = new AbortController()
    this.abort = abort
    this.action = action
    const departing = action === 'logout' && this.state.status === 'authenticated' ? this.state.session?.subject_id : undefined
    const current = () => this.connected && this.generation === generation
    this.publish({ busy: true, draftCleanup: undefined, pendingDevices: undefined,
      pendingFull: action === 'pending-devices' ? this.state.pendingFull : undefined,
      notice: { tone: 'neutral', text: '正在处理，请稍候…' } })
    const changesCredentials = !['check', 'activity', 'pending-devices'].includes(action)
    if (changesCredentials) invalidatePrivateContent()
    const privateGeneration = this.readPrivateGeneration()
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
        case 'pending-devices': {
          const devices = await pendingDevicesRequest(abort.signal)
          if (current() && this.readPrivateGeneration() === privateGeneration) {
            this.publish({ pendingDevices: devices, notice: { tone: 'neutral', text: '选择需要退出的旧会话，再确认继续登录。' } })
          }
          break
        }
        case 'complete':
        case 'replace-device':
          await accountRequest(action, abort.signal, input)
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
          if (departing && clearDrafts) {
            const session = await accountRequest('read', abort.signal)
            if (!current()) return
            if (session.subject_id !== departing || this.readPrivateGeneration() !== privateGeneration) {
              this.publish({ status: 'unknown', session: null, notice: { tone: 'error', text: '账号已变化；未退出或清理草稿。请重新检查会话。' } })
              break
            }
          }
          await accountRequest('logout', abort.signal)
          if (current()) this.publish({ status: 'anonymous', session: null, notice: { tone: 'success', text: '已退出当前会话。' } })
          if (current() && departing && clearDrafts) {
            try {
              const { clearAccountCodeDrafts } = await import('../preview/codeDraftStore')
              await clearAccountCodeDrafts(departing)
              if (current()) this.publish({ draftCleanup: 'complete' })
            } catch {
              if (current()) this.publish({ draftCleanup: 'unknown' })
            }
          }
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
      else if (unknown && action === 'replace-device') text = '登录结果无法确认。请检查当前会话，或继续确认原尝试；勿重复撤销。'
      else if (unknown && (action === 'login' || action === 'local-login' || action === 'complete')) text = '登录结果无法确认。请检查当前会话，或继续确认原尝试；勿重新提交密码。'
      else if (action === 'pending-devices') text = '暂时无法读取旧会话。可继续确认原尝试，或从已登录设备退出后再试。'
      else if (action === 'abandon' && error.code === 'LOGIN_ATTEMPT_INVALID') text = '原尝试未能确认清理，可能已过期或不存在。浏览器中的受保护凭据不能由页面代为删除。'
      else if (anonymousObservation) text = '尚未登录。请使用已有账号，或继续确认原尝试。'
      this.publish({ status, session, notice: { tone: unknown ? 'uncertain' : anonymousObservation ? 'neutral' : 'error', text },
        ...(error.code === 'SESSION_LIMIT' ? { pendingFull: true } : {}),
        ...(action === 'password' && unknown ? { passwordUnknown: true } : {}),
        ...(action === 'password' && error.code === 'NOT_FOUND' ? { passwordAvailable: false } : {}),
      })
    } finally {
      if (changesCredentials) invalidatePrivateContent()
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
