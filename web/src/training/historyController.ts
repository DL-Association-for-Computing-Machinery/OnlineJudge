import type { AccountController } from '../account/controller'
import { fetchTrainingHistory, HistoryError, type HistoryPage } from '../services/trainingHistory'

interface HistoryState { page: HistoryPage | null; number: number; busy: boolean; notice: string }
const initial: HistoryState = { page: null, number: 0, busy: false, notice: '检查会话后可读取本人的训练提交。' }
const pageHidden = () => document.visibilityState === 'hidden'

export class HistoryController {
  private state = initial
  private listeners = new Set<() => void>()
  private unsubscribe: (() => void) | null = null
  private request: AbortController | null = null
  private expiry: ReturnType<typeof setTimeout> | undefined
  private subject: string | null = null

  constructor(private account: AccountController) {}
  getSnapshot = () => this.state
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => this.listeners.delete(listener) }

  connect() { this.unsubscribe = this.account.subscribe(this.clear) }
  disconnect() { this.unsubscribe?.(); this.unsubscribe = null; this.clear() }

  private publish(state: HistoryState) {
    this.state = state
    for (const listener of this.listeners) listener()
  }

  clear = () => {
    this.request?.abort()
    this.request = null
    this.subject = null
    clearTimeout(this.expiry)
    this.publish(initial)
  }

  first = () => this.load('', 1, null)
  next = () => {
    if (this.subject && this.state.page?.next_cursor) {
      return this.load(this.state.page.next_cursor, this.state.number + 1, this.subject)
    }
  }

  private async load(cursor: string, number: number, priorSubject: string | null) {
    const account = this.account.getSnapshot()
    if (!this.unsubscribe || this.request || account.busy || pageHidden()) return
    // The account controller already checked the session. The history request
    // pins that observation; the server independently reads the live cookie.
    if (account.status !== 'authenticated' || !account.session) {
      await this.account.run('check')
      return
    }
    const session = account.session
    if (priorSubject && priorSubject !== session.subject_id) { this.clear(); return }
    const deadline = Math.min(Date.parse(session.idle_expires_at), Date.parse(session.absolute_deadline))
    const remaining = deadline - Date.now()
    if (!Number.isFinite(remaining) || remaining <= 0) {
      this.clear()
      await this.account.run('check')
      return
    }
    this.clear()
    const request = new AbortController()
    this.request = request
    this.expiry = setTimeout(this.clear, Math.min(remaining, 2_147_483_647))
    this.publish({ ...initial, busy: true, notice: '正在读取训练提交…' })
    try {
      const page = await fetchTrainingHistory(session.subject_id, cursor, request.signal)
      if (request.signal.aborted) return
      if (pageHidden() || Date.now() >= deadline) {
        this.clear()
        return
      }
      this.subject = session.subject_id
      this.publish({ page, number, busy: false, notice: page.submissions.length ?
        `第 ${number} 页，共 ${page.submissions.length} 条。${page.next_cursor ? '' : '已到末页。'}` : '没有可显示的训练提交。' })
    } catch (error) {
      if (request.signal.aborted) return
      const status = error instanceof HistoryError ? error.status : 0
      if (status === 401 || status === 409) {
        // The old subject must not be reused if the browser cookie changed.
        await this.account.run('check')
        return
      }
      const notice = status === 403 ? '只有已验证邮箱的个人账号可以查看训练提交。' :
        status === 404 ? '此入口尚未开放训练提交历史。' : '暂时无法读取训练提交，请重试第一页。'
      this.publish({ ...initial, notice })
    } finally {
      if (this.request === request) this.request = null
    }
  }
}
