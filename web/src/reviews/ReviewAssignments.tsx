import { useEffect, useState, useSyncExternalStore } from 'react'
import { Link } from 'react-router'
import { useCurrentAccount } from '../account/context'
import { subscribePrivateInvalidation } from '../account/privateInvalidation'
import { ReviewAssignmentController } from './assignmentController'

export function ReviewAssignments() {
  const { controller: account, state: accountState } = useCurrentAccount()
  const [controller] = useState(() => new ReviewAssignmentController(account))
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot)
  useEffect(() => {
    controller.connect()
    const unsubscribe = subscribePrivateInvalidation(controller.clear)
    window.addEventListener('focus', controller.clear)
    window.addEventListener('pagehide', controller.clear)
    window.addEventListener('pageshow', controller.clear)
    document.addEventListener('visibilitychange', controller.clear)
    return () => {
      unsubscribe(); controller.disconnect()
      window.removeEventListener('focus', controller.clear)
      window.removeEventListener('pagehide', controller.clear)
      window.removeEventListener('pageshow', controller.clear)
      document.removeEventListener('visibilitychange', controller.clear)
    }
  }, [controller])
  const busy = state.busy || accountState.busy
  return <section className="problem-statement" aria-labelledby="review-assignments">
    <h2 id="review-assignments">分配给我的审核项</h2>
    <button type="button" className="button button-primary" disabled={busy} onClick={() => void controller.load()}>检查会话并查看分配</button>
    <p role="status">{state.notice}</p>
    {accountState.status !== 'authenticated' && <p><Link to="/login">前往登录</Link></p>}
    {state.page && state.page.assignments.length > 0 && <ul>
      {state.page.assignments.map(item => <li key={item.review_id}>
        <Link to={`/review/materials/${item.review_id}`} aria-label={`选择 ${item.source_id}：${item.review_id}`}>{item.source_id}</Link>
        {' · 分配修订 '}{item.assignment_revision}{' · 材料 '}<code title={item.import_revision}>{item.import_revision.slice(0, 10)}</code>
      </li>)}
    </ul>}
    {state.page?.next_cursor && <button type="button" className="button button-outline" disabled={busy}
      onClick={() => void controller.load(true)}>下一页</button>}
  </section>
}
