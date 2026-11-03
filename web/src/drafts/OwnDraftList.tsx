import { useEffect, useState, useSyncExternalStore } from 'react'
import { Link } from 'react-router'
import { useCurrentAccount } from '../account/context'
import { subscribePrivateInvalidation } from '../account/privateInvalidation'
import { OwnDraftListController } from './listController'

export function OwnDraftList() {
  const { controller: account, state: accountState } = useCurrentAccount()
  const [controller] = useState(() => new OwnDraftListController(account))
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot)
  useEffect(() => {
    controller.connect()
    const unsubscribe = subscribePrivateInvalidation(controller.clear)
    for (const name of ['focus', 'pagehide', 'pageshow']) window.addEventListener(name, controller.clear)
    document.addEventListener('visibilitychange', controller.clear)
    return () => {
      unsubscribe(); controller.disconnect()
      for (const name of ['focus', 'pagehide', 'pageshow']) window.removeEventListener(name, controller.clear)
      document.removeEventListener('visibilitychange', controller.clear)
    }
  }, [controller])
  return <section aria-labelledby="own-draft-list-title">
    <h2 id="own-draft-list-title">服务器上的本人草稿</h2>
    <p>不依赖本机记录。按创建时间从新到旧分页；导入并发时可能需要重新检查第一页。列表不会确认或清除本机待核验请求。</p>
    <button className="button button-outline" type="button" disabled={state.busy || accountState.busy} onClick={() => void controller.first()}>检查会话并查看第一页</button>
    <p role="status">{state.notice}</p>
    {state.page && <>
      <ul className="management-list">{state.page.drafts.map(draft => <li key={draft.draft_id}>
        <div><h3>{draft.source_id}</h3><p><time dateTime={draft.created_at}>{new Date(draft.created_at).toLocaleString('zh-CN')}</time> · 私有草稿 · 尚未验证</p>
          <p>草稿 ID <code>{draft.draft_id}</code></p>
          <details><summary>查看固定版本与原请求</summary><p>文档版本 <code>{draft.version}</code></p><p>完整修订 <code>{draft.import_revision}</code></p><p>请求键 <code>{draft.request_key}</code></p></details>
        </div>
        <Link className="button button-primary" to={`/author/drafts/${draft.draft_id}`}>预览此草稿</Link>
      </li>)}</ul>
      {state.page.next_cursor && <button className="button button-outline" type="button" disabled={state.busy || accountState.busy} onClick={() => void controller.next()}>查看下一页</button>}
      {!state.page.next_cursor && state.page.drafts.length > 0 && <p>本次已读到末页。若有并发导入，请重新检查第一页。</p>}
    </>}
  </section>
}
