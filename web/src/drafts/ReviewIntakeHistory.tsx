import { useEffect, useRef, useState } from 'react'
import { useCurrentAccount } from '../account/context'
import { subscribePrivateInvalidation } from '../account/privateInvalidation'
import { readAuthorReviewReturn, readAuthorSolution, requestAuthorSolutionConfirmation, requestReviewIntake } from '../services/draftManagement'
import { listReviewIntakes, type SavedReviewIntake } from './reviewIntakeJournal'
import { readSolutionBinding, reserveSolutionConfirmation, solutionJournalSlot } from './solutionConfirmationJournal'

const pageSize = 20
type Preview = { entry: SavedReviewIntake; view: Awaited<ReturnType<typeof readAuthorSolution>> }
type Confirmation = 'available' | 'pending' | 'recorded' | 'unavailable'
type Page = {
  entries: SavedReviewIntake[]; next: string; notice: string; busy: boolean
  returned?: { id: string; reason: string }; preview?: Preview; confirmations?: Record<string, Confirmation>
}
const empty: Page = { entries: [], next: '', notice: '', busy: false }
function solutionStatus(owner: string, entry: SavedReviewIntake): Confirmation {
  try {
    return readSolutionBinding(solutionJournalSlot(owner, entry.intent), entry.intent) ? 'pending' : 'available'
  } catch { return 'unavailable' }
}

type Action = { after: string } | { entry: SavedReviewIntake; operation: 'receipt' | 'return' | 'solution' | 'confirm' | 'solution-receipt' }

export function ReviewIntakeHistory({ draft, version }: { draft: string; version: string }) {
  const { controller: account } = useCurrentAccount()
  const [page, setPage] = useState(empty)
  const active = useRef<AbortController | null>(null)
  useEffect(() => {
    const clear = () => {
      active.current?.abort()
      active.current = null
      setPage(empty)
    }
    const unsubscribe = account.subscribe(clear)
    const invalidate = subscribePrivateInvalidation(clear)
    window.addEventListener('pagehide', clear)
    document.addEventListener('visibilitychange', clear)
    return () => {
      unsubscribe(); invalidate(); clear()
      window.removeEventListener('pagehide', clear)
      document.removeEventListener('visibilitychange', clear)
    }
  }, [account])

  async function run(action: Action) {
    const state = account.getSnapshot()
    const session = state.session
    if (active.current || state.busy || state.status !== 'authenticated' || !session) return
    const deadline = Math.min(Date.parse(session.idle_expires_at), Date.parse(session.absolute_deadline))
    const validSession = () => account.getSnapshot() === state && Date.now() < deadline && document.visibilityState !== 'hidden'
    if (!validSession()) return
    const abort = new AbortController()
    active.current = abort
    const current = () => active.current === abort && validSession()
    const base = { ...page, returned: undefined, preview: undefined, busy: false }
    setPage({ ...base, notice: '', busy: true })
    const confirmation = (key: string, value: Confirmation, notice: string) => {
      if (current()) setPage({ ...base, notice, confirmations: { ...base.confirmations, [key]: value } })
    }
    try {
      if ('after' in action) {
        const rows = await listReviewIntakes(session.subject_id, draft, action.after)
        if (!current()) return
        const entries = rows.slice(0, pageSize)
        if (entries.some(row => row.intent.draft_id !== draft || row.version !== version)) throw new Error('Review journal does not match draft')
        setPage({ entries, next: rows.length > pageSize ? entries.at(-1)?.intent.reference_request_id ?? '' : '',
          notice: entries.length ? '请选择要查询的原送审记录。' : '此浏览器没有该草稿的送审记录。', busy: false,
          confirmations: Object.fromEntries(entries.map(entry => [entry.intent.request_key, solutionStatus(session.subject_id, entry)])),
        })
      } else {
        const { intent } = action.entry
        if (intent.draft_id !== draft || action.entry.version !== version) throw new Error('Review journal does not match draft')
        if (action.operation === 'return') {
          const returned = await readAuthorReviewReturn(session.subject_id, intent, version, abort.signal)
          if (current()) setPage({ ...base, returned, notice: `送审 ID：${returned.id}。已退回。` })
        } else if (action.operation === 'solution') {
          const view = await readAuthorSolution(session.subject_id, intent, version, abort.signal)
          if (current()) setPage({ ...base, preview: { entry: action.entry, view }, notice: '已读取此报告对应的冻结标程。' })
        } else if (action.operation === 'confirm' || action.operation === 'solution-receipt') {
          const key = intent.request_key
          const slot = solutionJournalSlot(session.subject_id, intent)
          const write = action.operation === 'confirm'
          let binding
          try {
            if (write) {
              if (page.preview?.entry !== action.entry || page.confirmations?.[key] !== 'available') {
                throw new Error('Read the original solution before confirming')
              }
              binding = page.preview.view.binding
              if (!reserveSolutionConfirmation(slot, intent, binding)) {
                confirmation(key, 'pending', '已有原确认记录，请查询原回执。')
                return
              }
            } else binding = readSolutionBinding(slot, intent)
          } catch {
            confirmation(key, 'unavailable', '本标签页无法保存或读取原确认记录。')
            return
          }
          if (!binding) {
            confirmation(key, 'unavailable', '本标签页没有原确认记录。')
            return
          }
          try {
            await requestAuthorSolutionConfirmation(write ? 'confirm' : 'read', session.subject_id, intent, version, binding, abort.signal)
            confirmation(key, 'recorded', '已核实此报告的作者标程确认回执。')
          } catch {
            confirmation(key, 'pending', '尚未核实标程确认。请保留本标签页并查询原回执；暂时查不到不代表写入失败。')
          }
        } else {
          const id = await requestReviewIntake('read', session.subject_id, intent, version, abort.signal)
          if (current()) setPage({ ...base, notice: `送审 ID：${id}。此回执仅表示材料已送审。` })
        }
      }
    } catch {
      if (current()) setPage({ ...base, notice: 'operation' in action && action.operation === 'solution'
        ? '无法核实此报告的冻结标程。'
        : 'operation' in action && action.operation === 'return'
          ? '未能核实这份送审的退回记录。可能尚无退回，或服务暂不可用。'
          : '未能核实原送审记录。请保留本机记录，暂时查不到并不表示未送审。' })
    } finally {
      if (active.current === abort) {
        if (!current()) setPage(empty)
        active.current = null
      }
    }
  }
  return <section className="reference-requests" aria-labelledby="review-history-title">
    <h2 id="review-history-title">本机送审记录</h2>
    <button className="button button-outline" type="button" disabled={page.busy}
      onClick={() => void run({ after: '' })}>查看本机送审记录</button>
    {page.entries.length > 0 && <ul>
      {page.entries.map(entry => <li key={entry.intent.request_key}>
        <p>参考报告 <code>{entry.intent.reference_request_id}</code></p>
        <button className="button button-outline" type="button" disabled={page.busy}
          onClick={() => void run({ entry, operation: 'receipt' })}>查该报告的送审回执</button>
        {' '}
        <button className="button button-outline" type="button" disabled={page.busy}
          onClick={() => void run({ entry, operation: 'return' })}>查看该报告的退回理由</button>
        {' '}
        <button className="button button-outline" type="button" disabled={page.busy}
          onClick={() => void run({ entry, operation: 'solution' })}>查看该报告的冻结标程</button>
        {page.confirmations?.[entry.intent.request_key] === 'pending' && <>
          {' '}<button className="button button-outline" type="button" disabled={page.busy}
            onClick={() => void run({ entry, operation: 'solution-receipt' })}>查询原标程确认回执</button>
        </>}
        {page.confirmations?.[entry.intent.request_key] === 'recorded' && <p>已核实此报告的标程确认。</p>}
      </li>)}
    </ul>}
    {page.next && <button className="button button-outline" type="button" disabled={page.busy}
      onClick={() => void run({ after: page.next })}>下一页送审记录</button>}
    {page.notice && <p role="status">{page.notice}</p>}
    {page.returned && <p className="review-return-reason">退回理由：{page.returned.reason}</p>}
    {page.preview && <section className="reference-result" aria-labelledby="author-solution-title">
      <h3 id="author-solution-title">冻结标程</h3>
      <p>送审 ID：<code>{page.preview.view.id}</code></p>
      <div className="sample"><pre><code>{page.preview.view.solution.source}</code></pre></div>
      {page.preview.view.solution.dependencies?.map(dep => <details key={dep.name} className="sample">
        <summary>{dep.name}</summary><pre><code>{dep.source}</code></pre>
      </details>)}
      {page.confirmations?.[page.preview.entry.intent.request_key] === 'available' && <>
        <p>请核对上方标程及依赖。确认只记录作者已核对此报告对应的冻结标程。</p>
        <button className="button button-primary" type="button" disabled={page.busy}
          onClick={() => { if (page.preview) void run({ entry: page.preview.entry, operation: 'confirm' }) }}>确认这份冻结标程</button>
      </>}
      {page.confirmations?.[page.preview.entry.intent.request_key] === 'unavailable' &&
        <p>本标签页无法保存或读取原确认记录，暂不能提交确认。</p>}
    </section>}
  </section>
}
