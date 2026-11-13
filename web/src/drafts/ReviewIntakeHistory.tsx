import { useEffect, useRef, useState } from 'react'
import { useCurrentAccount } from '../account/context'
import { subscribePrivateInvalidation } from '../account/privateInvalidation'
import { requestReviewIntake } from '../services/draftManagement'
import { listReviewIntakes, type SavedReviewIntake } from './reviewIntakeJournal'

const pageSize = 20
type Page = { entries: SavedReviewIntake[]; next: string; notice: string; busy: boolean }
const empty: Page = { entries: [], next: '', notice: '', busy: false }

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

  async function run(action: { after: string } | { entry: SavedReviewIntake }) {
    const state = account.getSnapshot()
    const session = state.session
    if (active.current || state.busy || state.status !== 'authenticated' || !session) return
    const deadline = Math.min(Date.parse(session.idle_expires_at), Date.parse(session.absolute_deadline))
    const validSession = () => account.getSnapshot() === state && Date.now() < deadline && document.visibilityState !== 'hidden'
    if (!validSession()) return
    const abort = new AbortController()
    active.current = abort
    const current = () => active.current === abort && validSession()
    setPage({ ...page, notice: '', busy: true })
    try {
      if ('after' in action) {
        const rows = await listReviewIntakes(session.subject_id, draft, action.after)
        if (!current()) return
        const entries = rows.slice(0, pageSize)
        if (entries.some(row => row.intent.draft_id !== draft || row.version !== version)) throw new Error('Review journal does not match draft')
        setPage({ entries, next: rows.length > pageSize ? entries.at(-1)?.intent.reference_request_id ?? '' : '',
          notice: entries.length ? '请选择要查询的原送审记录。' : '此浏览器没有该草稿的送审记录。', busy: false })
      } else {
        const { intent } = action.entry
        if (intent.draft_id !== draft || action.entry.version !== version) throw new Error('Review journal does not match draft')
        const id = await requestReviewIntake('read', session.subject_id, intent, version, abort.signal)
        if (current()) setPage({ ...page, notice: `送审 ID：${id}。此回执仅表示材料已送审。`, busy: false })
      }
    } catch {
      if (current()) setPage({ ...page, notice: '未能核实原送审记录。请保留本机记录，暂时查不到并不表示未送审。', busy: false })
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
          onClick={() => void run({ entry })}>查该报告的送审回执</button>
      </li>)}
    </ul>}
    {page.next && <button className="button button-outline" type="button" disabled={page.busy}
      onClick={() => void run({ after: page.next })}>下一页送审记录</button>}
    {page.notice && <p role="status">{page.notice}</p>}
  </section>
}
