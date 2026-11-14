import type { AccountController } from '../account/controller'
import { fetchReviewAssignments, type ReviewAssignmentPage } from '../services/reviewMaterials'

type State = { page: ReviewAssignmentPage | null; busy: boolean; notice: string }
const empty = (notice: string): State => ({ page: null, busy: false, notice })

export class ReviewAssignmentController {
  private state = empty('检查会话后查看分配给当前账号的审核项。')
  private listeners = new Set<() => void>()
  private connected = false
  private running = false
  private intent = 0
  private generation = 0
  private abort: AbortController | null = null
  private expiry: ReturnType<typeof setTimeout> | undefined
  private unsubscribe: (() => void) | undefined
  private pageOwner: string | undefined

  constructor(private account: AccountController) {}
  getSnapshot = () => this.state
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => this.listeners.delete(listener) }
  private publish(state: State) { this.state = state; for (const listener of this.listeners) listener() }
  connect() { this.connected = true; this.unsubscribe = this.account.subscribe(() => this.discard()) }
  disconnect() { this.connected = false; this.unsubscribe?.(); this.unsubscribe = undefined; this.clear() }
  clear = () => { ++this.intent; this.discard() }
  private discard() {
    ++this.generation
    this.abort?.abort()
    this.abort = null
    this.pageOwner = undefined
    clearTimeout(this.expiry)
    this.publish(empty('请重新检查会话并读取审核分配。'))
  }
  async load(next = false) {
    if (!this.connected || this.running || this.account.getSnapshot().busy) return
    const cursor = next ? this.state.page?.next_cursor : ''
    const previousOwner = this.pageOwner
    if (cursor === undefined || (next && (!cursor || !previousOwner))) return
    this.running = true
    this.clear()
    const intent = this.intent
    try {
      await this.account.run('check')
      const account = this.account.getSnapshot()
      const session = account.session
      if (!this.connected || this.intent !== intent || account.busy || account.status !== 'authenticated' || !session) return
      if (next && previousOwner !== session.subject_id) return
      const deadline = Math.min(Date.parse(session.idle_expires_at), Date.parse(session.absolute_deadline))
      if (!Number.isFinite(deadline) || deadline <= Date.now() || document.visibilityState === 'hidden') return
      const generation = ++this.generation
      const abort = new AbortController()
      this.abort = abort
      const current = () => {
        if (!this.connected || this.intent !== intent || this.generation !== generation) return false
        if (deadline <= Date.now() || document.visibilityState === 'hidden') { this.clear(); return false }
        return true
      }
      this.expiry = setTimeout(this.clear, Math.min(deadline - Date.now(), 2_147_483_647))
      this.publish({ ...empty('正在读取审核分配…'), busy: true })
      try {
        const page = await fetchReviewAssignments(session.subject_id, cursor, abort.signal)
        if (current()) {
          this.pageOwner = session.subject_id
          this.publish({ page, busy: false, notice: page.assignments.length > 0 ? '已读取本页分配。'
            : page.next_cursor ? '本页没有当前分配，可继续查看下一页。' : '没有可查看的审核分配。' })
        }
      } catch {
        if (current()) this.publish(empty('无法读取审核分配。请检查当前账号和权限，再从第一页重试。'))
      }
    } finally { this.running = false }
  }
}
