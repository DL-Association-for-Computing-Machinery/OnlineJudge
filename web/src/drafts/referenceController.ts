import type { AccountController } from '../account/controller'
import { ReferenceRequestError, sendReferenceRequest, sendReferencePreparation, type ReferenceAction, type ReferencePreparationAction, type ReferenceReceipt } from '../services/referenceRequests'
import { referenceJournal, type ReferenceJournal } from './referenceJournal'

interface ReferenceState { receipt: ReferenceReceipt | null; ready: boolean | null; busy: boolean; notice: string }
const initial: ReferenceState = { receipt: null, ready: null, busy: false, notice: '可提交新的参考验证申请，也可查询此浏览器保留的申请。' }
const notices: Record<string, string> = {
  REFERENCE_NOT_FOUND: '当前未查到回执。请保留原申请，可用同一请求再次提交。',
  NOT_FOUND: '参考验证入口尚未启用，请保留原申请。',
  REFERENCE_QUEUE_FULL: '参考验证队列已满，可稍后使用同一请求重试。',
  REFERENCE_SOURCE_CHANGED: '当前草稿版本不可申请参考验证。',
  REFERENCE_SUBJECT_CHANGED: '当前会话用户已变化，请重新打开本人草稿。',
  REFERENCE_KEY_CONFLICT: '申请与本地记录不一致，请保留原记录。',
  SESSION_REQUIRED: '会话无效或已过期，请重新登录。',
  PROBLEM_AUTHOR_REQUIRED: '当前会话没有出题权限。',
  REFERENCE_PREPARATION_BUSY: '材料准备繁忙，请稍后重试。',
  REFERENCE_PREPARATION_DENIED: '当前申请无法准备材料，请重新查看回执。',
}

export class ReferenceController {
  private state = initial
  private listeners = new Set<() => void>()
  private unsubscribe?: () => void
  private active?: AbortController
  constructor(private account: AccountController, private draft: string, private journal: ReferenceJournal = referenceJournal,
    private send: typeof sendReferenceRequest = sendReferenceRequest, private preparation: typeof sendReferencePreparation = sendReferencePreparation) {}
  getSnapshot = () => this.state
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => this.listeners.delete(listener) }
  private publish(state: ReferenceState) { this.state = state; for (const listener of this.listeners) listener() }
  connect() { this.unsubscribe = this.account.subscribe(this.clear) }
  clear = () => { this.active?.abort(); this.active = undefined; this.publish(initial) }
  disconnect() { this.unsubscribe?.(); this.unsubscribe = undefined; this.clear() }

  async run(action: ReferenceAction | ReferencePreparationAction) {
    const account = this.account.getSnapshot()
    const previous = this.state.receipt
    if (!this.unsubscribe || this.active || account.busy || account.status !== 'authenticated' || !account.session ||
      (action !== 'read' && action !== 'create' && previous?.state !== 'queued') || (action === 'create' && previous?.state === 'queued')) return
    const owner = account.session.subject_id
    const deadline = Math.min(Date.parse(account.session.idle_expires_at), Date.parse(account.session.absolute_deadline))
    if (!(Date.now() < deadline)) { this.clear(); return }
    const abort = new AbortController()
    this.active = abort
    const current = () => this.active === abort && Date.now() < deadline && this.account.getSnapshot() === account
    this.publish({ ...this.state, busy: true, notice: '正在读取或更新本人申请…' })
    let key: string | undefined
    try {
      key = action === 'create' ? await this.journal.select(owner, this.draft, previous?.request_key) : await this.journal.read(owner, this.draft)
      if (!current()) return
      if (!key || (action !== 'create' && action !== 'read' && key !== previous?.request_key)) {
        this.publish({ ...initial, notice: key ? '本地申请已变化，请重新查看当前回执。' : '此浏览器没有为这份草稿保留申请。' })
        return
      }
      if (action === 'prepare' || action === 'plan') {
        const ready = await this.preparation(action, owner, key, abort.signal)
        if (current()) this.publish({ receipt: previous, ready, busy: false, notice: ready ? '材料计划已准备。'
          : action === 'prepare' ? '已受理准备请求，请稍后查看材料状态。' : '尚未确认材料计划，可稍后查看或用原申请重新准备。' })
        return
      }
      const receipt = await this.send(action, owner, this.draft, key, abort.signal)
      if (current()) this.publish({ receipt, ready: null, busy: false, notice: '已取得本人申请回执。' })
    } catch (err) {
      const code = err instanceof ReferenceRequestError ? err.code : ''
      const keepReceipt = (action === 'prepare' || action === 'plan') && key !== undefined &&
        !['REFERENCE_SUBJECT_CHANGED', 'SESSION_REQUIRED', 'PROBLEM_AUTHOR_REQUIRED', 'REFERENCE_NOT_FOUND'].includes(code)
      if (current()) this.publish({ ...initial, notice: !key
        ? '本地申请记录不可用，未发送请求。'
        : notices[code] || (action === 'prepare'
          ? '准备结果未确认，请用原申请查看材料状态。' : '结果未确认，请保留原申请并查看回执。'),
      ...(keepReceipt ? { receipt: previous } : {}) })
    } finally {
      if (this.active === abort) {
        if (!current()) this.clear()
        else this.active = undefined
      }
    }
  }
}
