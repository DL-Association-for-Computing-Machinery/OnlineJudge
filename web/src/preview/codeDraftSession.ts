import { loadCodeDraft, writeCodeDraft, type DraftScope, type DraftVersion, type LoadedDraft, type DraftWrite } from './codeDraftStore'

type Phase = 'loading' | 'ready' | 'dirty' | 'saving' | 'saved' | 'conflict' | 'failed' | 'cleared'
interface DraftState {
  code: string
  phase: Phase
  stored?: string
  alternatives: LoadedDraft['conflicts']
}
type Timer = ReturnType<typeof setTimeout>

export class CodeDraftSession {
  private state: DraftState
  private baseline?: LoadedDraft
  private revision = 0
  private attempt = 0
  private connected = false
  private discard?: { id: string; revision: string }
  private competing?: DraftVersion
  private timer?: Timer
  private deadline?: Timer
  private writer?: string
  private saving = false
  private departing = false
  private listeners = new Set<() => void>()

  constructor(private readonly scope: DraftScope, private readonly starter: string, private readonly allowed: () => boolean) {
    this.state = { code: starter, phase: 'loading', alternatives: [] }
  }

  getSnapshot = () => this.state
  subscribe = (listener: () => void) => {
    this.listeners.add(listener)
    return () => { this.listeners.delete(listener) }
  }

  private publish(patch: Partial<DraftState>) {
    this.state = { ...this.state, ...patch }
    for (const listener of this.listeners) listener()
  }

  connect() {
    this.connected = true
    this.departing = false
    void this.load()
  }

  leave(save: boolean) {
    this.clearTimers()
    this.departing = save && this.allowed()
    if (this.departing && !this.saving && (this.state.phase === 'dirty' || this.state.phase === 'conflict')) void this.save()
    this.connected = false
    if (!this.departing) ++this.attempt
  }

  hide() {
    this.clearTimers()
    this.baseline = undefined
    ++this.attempt
    ++this.revision
    this.departing = false
    this.publish({ code: this.starter, phase: 'cleared', alternatives: [], stored: undefined })
  }

  private async load() {
    const attempt = ++this.attempt
    try {
      const result = await loadCodeDraft(this.scope)
      if (!this.connected || attempt !== this.attempt || !this.allowed()) return
      this.baseline = result
      this.publish({
        code: result.current?.code ?? this.starter,
        phase: result.current ? 'saved' : 'ready',
        alternatives: result.conflicts, stored: undefined,
      })
    } catch {
      if (this.connected && attempt === this.attempt && this.allowed()) this.publish({ phase: 'failed' })
    }
  }

  edit(code: string) {
    if (!this.allowed() || this.state.phase === 'loading' || this.state.phase === 'cleared') return false
    ++this.revision
    const conflicted = this.state.phase === 'conflict'
    this.publish({ code, phase: conflicted ? 'conflict' : this.baseline ? 'dirty' : 'failed' })
    if (this.baseline) this.schedule()
    return true
  }

  retry() {
    if (!this.allowed()) return
    if (!this.baseline) {
      // An unavailable initial read provides no version to overwrite.
      if (this.revision === 0) { this.publish({ phase: 'loading' }); void this.load() }
      else void this.refreshConflict()
    } else void this.save()
  }

  resume() {
    if (this.state.phase === 'loading' && !this.baseline) { void this.load(); return }
    if (this.state.phase === 'dirty' || this.state.phase === 'saving') {
      if (this.state.phase === 'saving') this.publish({ phase: 'dirty' })
      this.schedule()
    }
  }

  private async refreshConflict() {
    const attempt = ++this.attempt
    try {
      const result = await loadCodeDraft(this.scope)
      if (!this.connected || attempt !== this.attempt || !this.allowed()) return
      // The user may resolve the conflict after seeing the saved value.
      this.baseline = result
      this.competing = result.current
      this.publish({ phase: 'conflict', stored: result.current?.code, alternatives: result.conflicts })
    } catch {
      if (this.connected && attempt === this.attempt) this.publish({ phase: 'failed' })
    }
  }

  chooseDraft(code: string, discard?: string) {
    if (!this.baseline || !this.allowed() || this.saving) return
    if (this.state.phase === 'conflict') this.baseline = { ...this.baseline, current: this.competing }
    const alternative = this.state.alternatives.find(item => item.id === discard)
    this.discard = alternative && { id: alternative.id, revision: alternative.draft.revision }
    ++this.revision
    this.publish({ code, phase: 'dirty', stored: undefined })
    void this.save()
  }

  private schedule() {
    clearTimeout(this.timer)
    this.timer = setTimeout(() => void this.save(), 1_000)
    this.deadline ??= setTimeout(() => void this.save(), 5_000)
  }

  private clearTimers() {
    clearTimeout(this.timer)
    clearTimeout(this.deadline)
    this.timer = this.deadline = undefined
  }

  private async save() {
    if (!this.baseline || this.saving || !this.allowed() || (!this.connected && !this.departing)) return
    this.clearTimers()
    const attempt = this.attempt
    const revision = this.revision
    const snapshot = this.baseline
    const code = this.state.code
    this.saving = true
    if (!this.departing) this.publish({ phase: 'saving' })
    let result: DraftWrite | undefined
    try {
      this.writer ??= crypto.randomUUID()
      result = await writeCodeDraft({ scope: this.scope, epoch: snapshot.epoch,
        expected: snapshot.current?.revision, writer: this.writer, code, discard: this.discard },
      () => this.allowed() && this.attempt === attempt && (this.departing || this.connected))
    } catch { /* The editor keeps the current text regardless of this receipt. */ }
    finally { this.saving = false }
    if (attempt !== this.attempt) return
    if (!this.allowed()) {
      if (result?.status === 'saved' && this.baseline === snapshot) this.baseline = { ...snapshot, current: result.current }
      return
    }
    if (!this.connected) {
      // Route navigation may race an IDB open or transaction. Let that write
      // settle, then save any newer edit against its receipt or as a conflict.
      if (this.departing && result && result.status !== 'cleared' && result.status !== 'paused' && revision !== this.revision) {
        if (result.status === 'saved') this.baseline = { ...snapshot, current: result.current }
        void this.save()
      }
      return
    }
    if (!result) { this.publish({ phase: 'failed' }); return }
    if (result.status === 'cleared') { this.hide(); return }
    if (result.status === 'paused') { this.publish({ phase: 'dirty' }); this.schedule(); return }
    if (result.status === 'conflict') {
      this.competing = result.current
      this.publish({ phase: 'conflict', stored: result.current?.code })
      if (revision !== this.revision) this.schedule()
      return
    }
    this.baseline = { ...snapshot, current: result.current }
    const alternatives = snapshot.conflicts.filter(item =>
      !(item.id === this.discard?.id && item.draft.revision === this.discard.revision) && item.id !== this.writer)
    this.baseline.conflicts = alternatives
    this.discard = undefined
    this.publish({ phase: revision === this.revision ? 'saved' : 'dirty', alternatives })
    if (revision !== this.revision) this.schedule()
  }
}
