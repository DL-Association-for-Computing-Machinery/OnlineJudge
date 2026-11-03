import type { AccountController } from '../account/controller'
import { fetchDraftPreview, type DraftPreview } from '../services/privateDraft'

interface PreviewState { data: DraftPreview | null; busy: boolean; notice: string }
export class DraftPreviewController {
  private state: PreviewState = { data: null, busy: false, notice: '请检查会话并打开本人草稿。' }
  private listeners = new Set<() => void>()
  private generation = 0
  private intent = 0
  private abort: AbortController | null = null
  private unsubscribe: (() => void) | null = null
  private expiry: ReturnType<typeof setTimeout> | undefined
  private connected = false
  constructor(private account: AccountController, private id: string) {}
  getSnapshot = () => this.state
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => this.listeners.delete(listener) }
  private publish(state: PreviewState) { this.state = state; for (const listener of this.listeners) listener() }
  connect() {
    this.connected = true
    // Synchronous subscription fences a mutation before its first HTTP call.
    this.unsubscribe = this.account.subscribe(() => this.discard())
  }
  clear = () => {
    ++this.intent
    this.discard()
  }
  private discard() {
    ++this.generation
    this.abort?.abort()
    this.abort = null
    clearTimeout(this.expiry)
    this.publish({ data: null, busy: false, notice: '预览已清除，请重新检查会话并打开草稿。' })
  }
  disconnect() { this.connected = false; this.unsubscribe?.(); this.unsubscribe = null; this.clear() }
  async load() {
    this.clear()
    const intent = this.intent
    if (!this.connected || this.account.getSnapshot().busy) return
    // Check is read-only and never renews the session. Account publications
    // clear any old preview before a new request generation is allocated.
    await this.account.run('check')
    const account = this.account.getSnapshot()
    if (!this.connected || this.intent !== intent || account.busy || account.status !== 'authenticated' || !account.session) return
    const deadline = Math.min(Date.parse(account.session.idle_expires_at), Date.parse(account.session.absolute_deadline))
    const remaining = deadline - Date.now()
    if (remaining <= 0) return
    const generation = ++this.generation
    const abort = new AbortController()
    this.abort = abort
    this.expiry = setTimeout(this.clear, Math.min(remaining, 2_147_483_647))
    this.publish({ data: null, busy: true, notice: '正在读取本人草稿…' })
    try {
      const data = await fetchDraftPreview(this.id, abort.signal)
      if (this.connected && this.generation === generation) {
        if (Date.now() >= deadline) this.clear()
        else this.publish({ data, busy: false, notice: '当前版本预览。此草稿尚未发布。' })
      }
    } catch {
      if (this.connected && this.generation === generation) this.publish({ data: null, busy: false, notice: '无法读取草稿。请确认草稿 ID、本人权限及私有草稿入口已启用。' })
    }
  }
}
