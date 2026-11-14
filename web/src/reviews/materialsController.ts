import type { AccountController } from '../account/controller'
import { validKey } from '../services/draftManagement'
import { fetchReviewEligibility, fetchReviewMaterials, ReviewResponseError, type ReviewEligibility, type ReviewMaterials } from '../services/reviewMaterials'
import { requestReviewReturn, type ReviewReturnKey } from '../services/reviewReturn'
import { decodeApprovalKey, requestReviewApproval, type ApprovalKey } from '../services/reviewApproval'
import { validReturnReason } from '../services/reviewReturnReason'
import { requestStatementConfirmation } from '../services/statementConfirmation'
import { originalReturn, reserveReturn, returnSlot } from './returnJournal'

type Confirmation = 'ready' | 'pending' | 'recorded' | 'unavailable'
type ReturnStatus = 'ready' | 'pending' | 'recorded' | 'unavailable'
interface MaterialsState {
  data: ReviewMaterials | null; busy: boolean; notice: string; confirmation: Confirmation
  eligibility: ReviewEligibility | null; returnStatus: ReturnStatus; approval: Confirmation
}
const empty = (notice: string): MaterialsState => ({
  data: null, busy: false, notice, confirmation: 'unavailable', eligibility: null, returnStatus: 'unavailable', approval: 'unavailable',
})

function journalSlot(owner: string, evidence: ReviewMaterials['evidence']) {
  return ['oj.statement-confirmation.v1', owner, evidence.review_id, evidence.assignment_revision,
    evidence.import_revision, evidence.evidence_sha256].join(':')
}

function confirmationStatus(slot: string): Confirmation {
  try {
    const key = sessionStorage.getItem(slot)
    return key === null ? 'ready' : validKey(key) ? 'pending' : 'unavailable'
  } catch { return 'unavailable' }
}
function returnStatus(slot: string): ReturnStatus {
  try { return originalReturn(slot) === null ? 'ready' : 'pending' } catch { return 'unavailable' }
}
const authorityExpired = (deadline: number) => Date.now() >= deadline || document.visibilityState === 'hidden'

function approvalSlot(owner: string, evidence: ReviewMaterials['evidence']) {
  return ['oj.review-approval.v1', owner, evidence.review_id, evidence.assignment_revision].join(':')
}

function savedApproval(slot: string): ApprovalKey | null {
  const raw = sessionStorage.getItem(slot)
  return raw === null ? null : decodeApprovalKey(JSON.parse(raw) as unknown)
}

function approvalStatus(slot: string): Confirmation {
  try { return savedApproval(slot) === null ? 'ready' : 'pending' } catch { return 'unavailable' }
}

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
  private authority: { owner: string; deadline: number } | null = null

  constructor(private account: AccountController, private id: string) {}
  getSnapshot = () => this.state
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => this.listeners.delete(listener) }
  private publish(state: MaterialsState) { this.state = state; for (const listener of this.listeners) listener() }
  connect() { this.connected = true; this.unsubscribe = this.account.subscribe(() => this.discard()) }
  disconnect() { this.connected = false; this.unsubscribe?.(); this.unsubscribe = null; this.clear() }
  clear = () => { ++this.intent; this.discard() }
  private discard() {
    ++this.generation; this.abort?.abort(); this.abort = null; clearTimeout(this.expiry)
    this.authority = null
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
        if (current()) {
          this.authority = { owner: session.subject_id, deadline }
          this.publish({ ...empty('已读取当前分配的冻结材料。'), data,
            confirmation: confirmationStatus(journalSlot(session.subject_id, data.evidence)),
            returnStatus: returnStatus(returnSlot(session.subject_id, data.evidence)),
            approval: approvalStatus(approvalSlot(session.subject_id, data.evidence)) })
        }
      } catch {
        if (current()) this.publish(empty('无法读取材料。请核对送审 ID、当前分配及服务状态，再手动重试。'))
      }
    } finally { this.running = false }
  }

  async readEligibility() {
    const { data, busy } = this.state
    const authority = this.authority
    if (!this.connected || this.running || busy || this.account.getSnapshot().busy || !data || !authority) return
    const { owner, deadline } = authority
    if (authorityExpired(deadline)) { this.clear(); return }
    const generation = this.generation
    const abort = new AbortController()
    this.abort = abort
    this.running = true
    this.publish({ ...this.state, busy: true, eligibility: null, notice: '正在读取批准准入状态…' })
    try {
      const eligibility = await fetchReviewEligibility(data.evidence, owner, abort.signal)
      if (generation !== this.generation) return
      if (authorityExpired(deadline)) { this.clear(); return }
      this.publish({ ...this.state, busy: false, eligibility, notice: '已读取本次准入状态。' })
    } catch {
      if (generation === this.generation) {
        this.clear()
        this.publish(empty('无法核实批准准入状态，请重新检查会话并读取材料。'))
      }
    } finally {
      this.running = false
      if (this.abort === abort) this.abort = null
    }
  }

  async confirm(action: 'confirm' | 'read', allChecked = false) {
    const { data, confirmation, busy, returnStatus, eligibility } = this.state
    const authority = this.authority
    if (!this.connected || this.running || busy || !data || !authority || returnStatus === 'recorded' ||
      returnStatus === 'pending' || eligibility?.blockers.includes('materials_returned') ||
      (action === 'confirm' ? !allChecked || confirmation !== 'ready' : confirmation !== 'pending')) return
    const { owner, deadline } = authority
    if (authorityExpired(deadline)) { this.clear(); return }
    const generation = this.generation
    const slot = journalSlot(owner, data.evidence)
    let key: string
    try {
      const saved = sessionStorage.getItem(slot)
      if (action === 'confirm' && saved === null) {
        key = crypto.randomUUID().replaceAll('-', '')
        // Save before POST so a lost response or refresh only needs a GET.
        sessionStorage.setItem(slot, key)
      } else if (validKey(saved)) {
        if (action === 'confirm') { this.publish({ ...this.state, confirmation: 'pending' }); return }
        key = saved
      } else { throw new Error('Original request key unavailable') }
    } catch {
      this.publish({ ...this.state, confirmation: 'unavailable' })
      return
    }
    this.running = true
    const abort = new AbortController()
    this.abort = abort
    this.publish({ ...this.state, busy: true, confirmation: 'pending', eligibility: null })
    let recorded = false
    try {
      await requestStatementConfirmation(action, owner, data.evidence, key, abort.signal)
      recorded = true
    } catch {
      // A missing receipt can mean the write is still in flight. Never resend it.
    } finally {
      this.running = false
      if (generation === this.generation) {
        if (authorityExpired(deadline)) this.clear()
        else {
          this.abort = null
          this.publish({ ...this.state, busy: false, confirmation: recorded ? 'recorded' : 'pending' })
        }
      }
    }
  }

  async returnReview(reason?: string) {
    const { data, busy, returnStatus, eligibility, approval } = this.state
    const authority = this.authority
    const write = reason !== undefined
    if (!this.connected || this.running || busy || this.account.getSnapshot().busy || !data || !authority ||
      (write ? approval === 'pending' || approval === 'recorded' || returnStatus !== 'ready' || !validReturnReason(reason) ||
        eligibility?.blockers.includes('materials_returned') : returnStatus !== 'pending')) return
    const { owner, deadline } = authority
    if (authorityExpired(deadline)) { this.clear(); return }
    const slot = returnSlot(owner, data.evidence)
    let original: ReviewReturnKey
    try {
      const saved = originalReturn(slot)
      if (write && saved === null) original = reserveReturn(slot, data.evidence.assignment_revision)
      else if (saved && !write) original = saved
      else {
        this.publish({ ...this.state, returnStatus: saved ? 'pending' : 'unavailable' })
        return
      }
    } catch {
      this.publish({ ...this.state, returnStatus: 'unavailable' })
      return
    }
    const generation = this.generation
    const abort = new AbortController()
    this.abort = abort
    this.running = true
    this.publish({ ...this.state, busy: true, returnStatus: 'pending', eligibility: null,
      notice: write ? '正在提交这份材料的退回决定…' : '正在查询原退回回执…' })
    let recorded = false
    try {
      await requestReviewReturn(owner, data.evidence, original, abort.signal, reason)
      recorded = true
    } catch {
      // Absence or an invalid response cannot establish that no decision was committed.
    } finally {
      this.running = false
      if (generation === this.generation) {
        if (authorityExpired(deadline)) this.clear()
        else {
          this.abort = null
          this.publish({ ...this.state, busy: false, returnStatus: recorded ? 'recorded' : 'pending',
            notice: recorded ? '已核实这份材料的退回回执。' : '尚未核实本次退回。' })
        }
      }
    }
  }

  async approve(reason?: string) {
    const { data, busy, approval, eligibility, returnStatus } = this.state
    const authority = this.authority
    if (!this.connected || this.running || busy || this.account.getSnapshot().busy || !data || !authority) return
    const write = reason !== undefined
    if (write ? approval !== 'ready' || !eligibility?.approvalEnabled || !eligibility.eligible ||
      returnStatus === 'pending' || returnStatus === 'recorded' || !validReturnReason(reason) : approval !== 'pending') return
    const { owner, deadline } = authority
    if (authorityExpired(deadline)) { this.clear(); return }
    const slot = approvalSlot(owner, data.evidence)
    let original: ApprovalKey
    try {
      const saved = savedApproval(slot)
      if (saved && write) { this.publish({ ...this.state, approval: 'pending' }); return }
      if (saved) original = saved
      else if (write && eligibility) {
        original = { key: crypto.randomUUID().replaceAll('-', ''), snapshot: eligibility.snapshot }
        sessionStorage.setItem(slot, JSON.stringify(original))
      } else throw new Error('Original approval key unavailable')
    } catch {
      this.publish({ ...this.state, approval: 'unavailable' })
      return
    }
    const generation = this.generation
    const abort = new AbortController()
    this.abort = abort
    this.running = true
    this.publish({ ...this.state, busy: true, approval: 'pending', eligibility: null,
      notice: write ? '正在提交终审批准…' : '正在查询原批准回执…' })
    let status: Confirmation = 'pending'
    let notice = '尚未核实本次批准，请查询原回执。'
    try {
      await requestReviewApproval(owner, data.evidence.review_id, data.evidence.assignment_revision, original, abort.signal, reason)
      status = 'recorded'
      notice = '已核实批准回执。'
    } catch (err) {
      if (err instanceof ReviewResponseError && generation === this.generation) {
        if ((err.status === 401 && err.code === 'SESSION_REQUIRED') ||
          (err.status === 403 && err.code === 'PROBLEM_REVIEWER_REQUIRED') ||
          (err.status === 409 && err.code === 'REVIEW_SUBJECT_CHANGED')) {
          this.clear()
          this.publish(empty('无法核实当前审核权限，请重新检查会话并读取材料。'))
        } else if (write && err.status === 409 && (err.code === 'REVIEW_CHANGED' || err.code === 'REVIEW_APPROVAL_BLOCKED')) {
          // Only these responses to the first POST prove this key did not write.
          // A failed GET, even 404, cannot settle an earlier unknown result.
          try {
            const saved = savedApproval(slot)
            if (saved?.key === original.key && saved.snapshot === original.snapshot) sessionStorage.removeItem(slot)
            this.clear()
            this.publish(empty('本次批准未写入，请重新读取材料和准入状态。'))
          } catch { /* Leave a potentially outstanding journal entry alone. */ }
        }
      }
    } finally {
      this.running = false
      if (generation === this.generation) {
        if (authorityExpired(deadline)) this.clear()
        else {
          this.abort = null
          this.publish({ ...this.state, busy: false, approval: status, notice })
        }
      }
    }
  }
}
