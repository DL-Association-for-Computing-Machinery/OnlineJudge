import type { AccountController } from '../account/controller'
import { requestReviewIntake } from '../services/draftManagement'
import type { ReferenceReport } from '../services/referenceRequests'
import { referenceJournal } from './referenceJournal'
import { hasReviewIntake, reserveReviewIntake } from './reviewIntakeJournal'

interface State {
  status: 'ready' | 'busy' | 'pending' | 'recorded' | 'unavailable'
  id?: string
}

export class ReviewIntakeController {
  private state: State = { status: 'unavailable' }
  private listeners = new Set<() => void>()
  private unsubscribe?: () => void
  private active?: AbortController
  private expiry?: ReturnType<typeof setTimeout>

  constructor(private account: AccountController, private report: ReferenceReport & { import_revision: string }) {}
  getSnapshot = () => this.state
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => this.listeners.delete(listener) }
  private publish(state: State) { this.state = state; for (const listener of this.listeners) listener() }
  connect() {
    this.unsubscribe = this.account.subscribe(this.clear)
    void this.run('prepare')
  }
  disconnect() { this.unsubscribe?.(); this.unsubscribe = undefined; this.clear() }
  clear = () => {
    clearTimeout(this.expiry)
    this.active?.abort()
    this.active = undefined
    this.publish({ status: 'unavailable' })
  }

  async run(action: 'prepare' | 'submit' | 'read') {
    const snapshot = this.account.getSnapshot()
    const session = snapshot.session
    if (!this.unsubscribe || this.active || snapshot.busy || snapshot.status !== 'authenticated' || !session ||
      (action === 'submit' && this.state.status !== 'ready') || (action === 'read' && this.state.status !== 'pending')) return
    const deadline = Math.min(Date.parse(session.idle_expires_at), Date.parse(session.absolute_deadline))
    const currentAccount = () => this.account.getSnapshot() === snapshot && Date.now() < deadline && document.visibilityState !== 'hidden'
    if (!currentAccount()) { this.clear(); return }
    clearTimeout(this.expiry)
    this.expiry = setTimeout(this.clear, Math.min(deadline - Date.now(), 2_147_483_647))
    const abort = new AbortController()
    this.active = abort
    const current = () => this.active === abort && currentAccount()
    const owner = session.subject_id
    const report = this.report
    const intent = { request_key: `web-review-v1-${report.request_id}`, draft_id: report.draft_id,
      import_revision: report.import_revision, reference_request_id: report.request_id }
    this.publish({ status: 'busy' })
    let mayHaveSent = action === 'read'
    try {
      if (await referenceJournal.read(owner, report.draft_id) !== report.request_key) {
        if (current()) this.publish({ status: 'unavailable' })
        return
      }
      if (!current()) return
      if (action === 'prepare') {
        const sent = await hasReviewIntake(owner, intent)
        if (current()) this.publish({ status: sent ? 'pending' : 'ready' })
        return
      }
      if (action === 'submit') {
        const first = await reserveReviewIntake(owner, { intent, version: report.content_version })
        mayHaveSent = true
        if (!current()) return
        if (!first) { this.publish({ status: 'pending' }); return }
      }
      const id = await requestReviewIntake(action, owner, intent, report.content_version, abort.signal)
      if (current()) this.publish({ status: 'recorded', id })
    } catch {
      if (current()) this.publish({ status: mayHaveSent ? 'pending' : 'unavailable' })
    } finally {
      if (this.active === abort) {
        if (!current()) this.clear()
        else this.active = undefined
      }
    }
  }
}
