import type { AccountController } from '../account/controller'
import { fetchOwnDraftPage, ImportError, type OwnDraftPage } from '../services/draftManagement'

interface ListState { owner: string | null; page: OwnDraftPage | null; pageNumber: number; busy: boolean; notice: string }
const empty = (notice: string): ListState => ({ owner: null, page: null, pageNumber: 0, busy: false, notice })

// Server discovery remains usable even when the local import journal is full,
// unavailable or cleared. List results never reconcile uncertain local writes.
export class OwnDraftListController {
  private state = empty('显式检查会话后查看服务器上的本人草稿。')
  private listeners = new Set<() => void>()
  private connected = false
  private running = false
  private generation = 0
  private intent = 0
  private abort: AbortController | null = null
  private unsubscribe: (() => void) | null = null
  private expiry: ReturnType<typeof setTimeout> | undefined
  constructor(private account: AccountController) {}
  getSnapshot = () => this.state
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => this.listeners.delete(listener) }
  private publish(state: ListState) { this.state = state; for (const listener of this.listeners) listener() }
  connect() { this.connected = true; this.unsubscribe = this.account.subscribe(() => this.discard()) }
  disconnect() { this.connected = false; this.unsubscribe?.(); this.unsubscribe = null; this.clear() }
  clear = () => { ++this.intent; this.discard() }
  private discard() {
    ++this.generation; this.abort?.abort(); this.abort = null; clearTimeout(this.expiry)
    this.publish(empty('列表已清除，请重新检查会话并查看第一页。'))
  }
  first = async () => this.load('', null, 1)
  next = async () => {
    const { page, owner, pageNumber } = this.state
    if (!owner || !page?.next_cursor) return
    await this.load(page.next_cursor, owner, pageNumber + 1)
  }
  private async load(cursor: string, expectedOwner: string | null, pageNumber: number) {
    if (this.running || !this.connected || this.account.getSnapshot().busy) return
    this.running = true; this.clear(); const intent = this.intent
    try {
      await this.account.run('check')
      const state = this.account.getSnapshot(); const session = state.session
      if (!this.connected || this.intent !== intent || state.busy || state.status !== 'authenticated' || !session) return
      if (expectedOwner && expectedOwner !== session.subject_id) { this.publish(empty('账号已变化，请重新查看当前账号的第一页。')); return }
      const deadline = Math.min(Date.parse(session.idle_expires_at), Date.parse(session.absolute_deadline))
      if (!Number.isFinite(deadline) || deadline <= Date.now() || document.visibilityState === 'hidden') return
      const generation = ++this.generation; const abort = new AbortController(); this.abort = abort
      const current = () => {
        if (!this.connected || this.generation !== generation || this.intent !== intent) return false
        if (Date.now() >= deadline || document.visibilityState === 'hidden') { this.clear(); return false }
        return true
      }
      this.expiry = setTimeout(this.clear, Math.min(deadline - Date.now(), 2_147_483_647))
      this.publish({ ...empty('正在读取本人服务器草稿…'), owner: session.subject_id, busy: true })
      try {
        const page = await fetchOwnDraftPage(session.subject_id, cursor, abort.signal)
        if (current()) this.publish({ owner: session.subject_id, page, pageNumber, busy: false,
          notice: page.drafts.length ? `当前第 ${pageNumber} 页，每页最多 20 条。` : '本次未读到本人草稿；这不证明没有仍在提交的导入。' })
      } catch (error) {
        if (current()) {
          const notice = error instanceof ImportError && error.code === 'DRAFT_SUBJECT_CHANGED' ? '浏览器账号已变化，请重新查看第一页。' :
            error instanceof ImportError && error.code === 'DRAFT_CURSOR_INVALID' ? '分页游标无效，请重新检查第一页。' :
              '无法读取列表。请检查当前账号、出题权限和服务状态，再手动重试第一页。'
          this.publish(empty(notice))
        }
      }
    } finally { this.running = false }
  }
}
