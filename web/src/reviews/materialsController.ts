import type { AccountController } from '../account/controller'
import { fetchReviewMaterials, type ReviewMaterials } from '../services/reviewMaterials'

interface MaterialsState { data: ReviewMaterials | null; busy: boolean; notice: string }
const empty = (notice: string): MaterialsState => ({ data: null, busy: false, notice })

export class ReviewMaterialsController {
  private state = empty('请检查会话并读取分配给当前账号的审核材料。')
  private listeners = new Set<() => void>()
  private connected = false
  private running = false
  private intent = 0
  private generation = 0
  private abort: AbortController | null = null
  private unsubscribe: (() => void) | null = null
  private expiry: ReturnType<typeof setTimeout> | undefined

  constructor(private account: AccountController, private id: string) {}
  getSnapshot = () => this.state
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => this.listeners.delete(listener) }
  private publish(state: MaterialsState) { this.state = state; for (const listener of this.listeners) listener() }
  connect() { this.connected = true; this.unsubscribe = this.account.subscribe(() => this.discard()) }
  disconnect() { this.connected = false; this.unsubscribe?.(); this.unsubscribe = null; this.clear() }
  clear = () => { ++this.intent; this.discard() }
  private discard() {
    ++this.generation; this.abort?.abort(); this.abort = null; clearTimeout(this.expiry)
    this.publish(empty('材料已清除，请重新检查会话。'))
  }

  async load() {
    if (this.running || !this.connected || this.account.getSnapshot().busy) return
    this.running = true; this.clear(); const intent = this.intent
    try {
      await this.account.run('check')
      const account = this.account.getSnapshot(); const session = account.session
      if (!this.connected || this.intent !== intent || account.busy || account.status !== 'authenticated' || !session) return
      const deadline = Math.min(Date.parse(session.idle_expires_at), Date.parse(session.absolute_deadline))
      if (!Number.isFinite(deadline) || deadline <= Date.now() || document.visibilityState === 'hidden') return
      const generation = ++this.generation; const abort = new AbortController(); this.abort = abort
      const current = () => {
        if (!this.connected || this.generation !== generation || this.intent !== intent) return false
        if (Date.now() >= deadline || document.visibilityState === 'hidden') { this.clear(); return false }
        return true
      }
      this.expiry = setTimeout(this.clear, Math.min(deadline - Date.now(), 2_147_483_647))
      this.publish({ ...empty('正在读取审核材料…'), busy: true })
      try {
        const data = await fetchReviewMaterials(this.id, session.subject_id, abort.signal)
        if (current()) this.publish({ data, busy: false, notice: '已读取当前分配的冻结材料。' })
      } catch {
        if (current()) this.publish(empty('无法读取材料。请核对送审 ID、当前分配及服务状态，再手动重试。'))
      }
    } finally { this.running = false }
  }
}
