import type { AccountController } from '../account/controller'
import { fetchImportReceipt, fetchImportSources, ImportError, postImport, type ImportSource, type ImportReceipt } from '../services/draftManagement'
import { ImportJournal, JournalError, type JournalEntry } from './importJournal'

interface ManagementState {
  owner: string | null; sources: ImportSource[]; entries: JournalEntry[]
  receipts: ReadonlyMap<string, ImportReceipt>; busy: boolean; notice: string
}
const empty = (notice: string): ManagementState => ({ owner: null, sources: [], entries: [], receipts: new Map(), busy: false, notice })
type Action = { owner: string; signal: AbortSignal; current: () => boolean }
const messages: Record<string, string> = {
  SESSION_REQUIRED: '会话已失效，请重新登录后查询原回执。',
  PROBLEM_AUTHOR_REQUIRED: '当前账号没有出题权限。恢复记录保留在本机。',
  DRAFT_SUBJECT_CHANGED: '浏览器账号已变化。已清除显示，请重新检查当前账号。',
  DRAFT_NOT_FOUND: '此刻尚未查到回执，不代表原请求已取消。可手动重试同一请求。',
  DRAFT_SOURCE_CHANGED: '来源修订已变化。原记录仍保留；请查询回执，勿更换请求键或修订。',
  DRAFT_REQUEST_CONFLICT: '请求键与服务端内容不一致。请保留记录并联系维护者。',
}
export class DraftManagementController {
  private state = empty('检查会话后可选择来源，或恢复本机保留的导入。')
  private listeners = new Set<() => void>()
  private connected = false
  private running = false
  private generation = 0
  private intent = 0
  private abort: AbortController | null = null
  private unsubscribe: (() => void) | null = null
  private expiry: ReturnType<typeof setTimeout> | undefined
  constructor(private account: AccountController, private journal = new ImportJournal()) {}
  getSnapshot = () => this.state
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => this.listeners.delete(listener) }
  private publish(state: ManagementState) { this.state = state; for (const listener of this.listeners) listener() }
  connect() { this.connected = true; this.unsubscribe = this.account.subscribe(() => this.discard()); this.journal.connect(this.clear) }
  disconnect() { this.connected = false; this.unsubscribe?.(); this.unsubscribe = null; this.clear(); this.journal.close() }
  clear = () => { ++this.intent; this.discard() }
  private discard() {
    ++this.generation; this.abort?.abort(); this.abort = null; clearTimeout(this.expiry)
    this.publish(empty('显示已清除。本地恢复记录仍保留，请重新检查会话。'))
  }
  private async run(expectedOwner: string | null, work: (action: Action) => Promise<void>) {
    if (this.running || !this.connected || this.account.getSnapshot().busy) return
    this.running = true; this.clear(); const intent = this.intent
    try {
      await this.account.run('check')
      const state = this.account.getSnapshot(); const session = state.session
      if (!this.connected || this.intent !== intent || state.busy || state.status !== 'authenticated' || !session) return
      if (expectedOwner && session.subject_id !== expectedOwner) { this.publish(empty('账号已变化，请重新加载当前账号的来源和记录。')); return }
      const deadline = Math.min(Date.parse(session.idle_expires_at), Date.parse(session.absolute_deadline))
      if (!Number.isFinite(deadline) || deadline <= Date.now() || document.visibilityState === 'hidden') return
      const generation = ++this.generation; const abort = new AbortController(); this.abort = abort
      const current = () => {
        if (!this.connected || this.generation !== generation || this.intent !== intent) return false
        // Timers can be delayed while a page is suspended. Observing expiry
        // also revokes records already shown before the outstanding request.
        if (Date.now() >= deadline || document.visibilityState === 'hidden') { this.clear(); return false }
        return true
      }
      this.expiry = setTimeout(this.clear, Math.min(deadline - Date.now(), 2_147_483_647))
      this.publish({ ...empty('正在核验本人导入记录…'), owner: session.subject_id, busy: true })
      try { await work({ owner: session.subject_id, signal: abort.signal, current }) }
      catch (error) {
        if (current()) {
          const notice = error instanceof JournalError ? error.message : error instanceof ImportError && messages[error.code] || '结果尚未确认。请重新加载本地记录，再手动查询原回执；不要创建替代请求。'
          const clear = error instanceof JournalError || error instanceof ImportError && ['DRAFT_SUBJECT_CHANGED', 'SESSION_REQUIRED'].includes(error.code)
          this.publish(clear ? empty(notice) : { ...this.state, busy: false, notice })
        }
      }
    } finally { this.running = false }
  }
  private async show(action: Action, notice: string, receipt?: ImportReceipt, busy = false) {
    const entries = await this.journal.list(action.owner)
    if (action.current()) this.publish({ ...this.state, owner: action.owner, entries, busy, notice,
      receipts: receipt ? new Map(this.state.receipts).set(receipt.request_key, receipt) : this.state.receipts })
  }
  load = async () => this.run(null, async action => {
    // Recoverable tuples remain available even if the configured catalog is now
    // missing/changed. A catalog failure never erases an uncertain operation.
    await this.show(action, '本机记录已读取；回执需要逐条手动核验。', undefined, true)
    if (!action.current()) return
    const sources = await fetchImportSources(action.owner, action.signal)
    if (action.current()) this.publish({ ...this.state, sources, busy: false, notice: '来源已核验。导入只创建本人私有草稿，尚不审核或发布。' })
  })
  import = async (source: ImportSource) => {
    const owner = this.state.owner
    if (!owner || !this.state.sources.some(s => s.source_id === source.source_id && s.import_revision === source.import_revision)) return
    await this.run(owner, async action => {
      await this.journal.list(owner)
      if (!action.current()) return
      let verified = await this.journal.active(owner, source.source_id)
      if (!action.current()) return
      if (verified?.status === 'confirmed') {
        const receipt = await fetchImportReceipt(verified, action.signal)
        if (!action.current() || receipt.draft_id !== verified.draftId) throw new ImportError()
        verified = await this.journal.confirm(verified, receipt)
        if (!action.current()) return
        if (verified.revision === source.import_revision) { await this.show(action, '此修订已导入，回执已核验。', receipt); return }
      }
      const { entry, created } = await this.journal.begin(owner, source, verified)
      // An account/visibility change during commit leaves a pending tuple but
      // cannot cause a late POST. A competing tab's tuple never triggers POST.
      if (!action.current()) return
      if (!created) { await this.show(action, '已有原导入记录。请先查询回执，或手动重试同一请求。'); return }
      await this.show(action, '恢复记录已保存，正在导入…', undefined, true)
      await this.journal.require(entry)
      if (!action.current()) return
      const receipt = await postImport(entry, action.signal)
      if (!action.current()) return
      await this.journal.confirm(entry, receipt)
      if (action.current()) await this.show(action, '导入已确认，可打开本人草稿预览。', receipt)
    })
  }
  recover = async (entry: JournalEntry, retry = false) => this.run(entry.owner, async action => {
    // Storage is re-read and compared by confirm; server evidence is never
    // inferred from the browser's confirmed flag or a missing receipt.
    await this.journal.list(action.owner)
    await this.journal.require(entry)
    if (!action.current()) return
    let receipt: ImportReceipt
    try { receipt = await fetchImportReceipt(entry, action.signal) }
    catch (error) {
      if (!action.current() || !retry || entry.status !== 'pending' || !(error instanceof ImportError) || error.code !== 'DRAFT_NOT_FOUND') throw error
      const sources = await fetchImportSources(action.owner, action.signal)
      if (!action.current()) return
      if (!sources.some(s => s.source_id === entry.source && s.import_revision === entry.revision)) throw new ImportError('DRAFT_SOURCE_CHANGED')
      await this.journal.require(entry)
      if (!action.current()) return
      receipt = await postImport(entry, action.signal)
    }
    if (!action.current() || entry.status === 'confirmed' && entry.draftId !== receipt.draft_id) throw new ImportError()
    await this.journal.confirm(entry, receipt)
    if (action.current()) await this.show(action, '服务端回执已核验，可打开本人草稿预览。', receipt)
  })
  remove = async (entry: JournalEntry) => this.run(entry.owner, async action => {
    const receipt = await fetchImportReceipt(entry, action.signal)
    if (!action.current() || entry.status !== 'confirmed' || receipt.draft_id !== entry.draftId) throw new ImportError()
    await this.journal.removeConfirmed(entry)
    if (action.current()) await this.show(action, '已移除此条本机记录；服务端草稿仍保留。')
  })
}
