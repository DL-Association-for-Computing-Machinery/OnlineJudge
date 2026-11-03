import { useEffect, useState, useSyncExternalStore } from 'react'
import { Link } from 'react-router'
import { useCurrentAccount } from '../account/context'
import { subscribePrivateInvalidation } from '../account/privateInvalidation'
import { DraftManagementController } from './managementController'
import './management.css'

export default function DraftManagementPage() {
  const { controller: account, state: accountState } = useCurrentAccount()
  const [controller] = useState(() => new DraftManagementController(account))
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot)
  useEffect(() => {
    controller.connect()
    const unsubscribe = subscribePrivateInvalidation(controller.clear)
    const clear = controller.clear
    for (const name of ['focus', 'pagehide', 'pageshow']) window.addEventListener(name, clear)
    document.addEventListener('visibilitychange', clear)
    return () => {
      unsubscribe(); controller.disconnect()
      for (const name of ['focus', 'pagehide', 'pageshow']) window.removeEventListener(name, clear)
      document.removeEventListener('visibilitychange', clear)
    }
  }, [controller])
  const disabled = state.busy || accountState.busy
  return <div className="page-width private-draft-page management-page">
    <p className="breadcrumb"><Link to="/">题库</Link> / 本人私有草稿</p>
    <h1>导入私有草稿</h1>
    <p>从已配置来源导入题面和样例。草稿仅本人可见，尚未审核、验证或发布。</p>
    <button className="button button-primary" type="button" disabled={disabled} onClick={() => void controller.load()}>检查会话并加载来源与记录</button>
    <p role="status" className="management-notice">{state.notice}</p>
    {accountState.status !== 'authenticated' && <p>{accountState.notice.text} <Link to="/login">前往登录</Link></p>}
    {state.owner && <>
      <section aria-labelledby="import-sources"><h2 id="import-sources">可导入来源</h2>
        {state.sources.length === 0 ? <p>尚未读取来源。可重新检查，或先查询下面的原回执。</p> : <ul className="management-list">
          {state.sources.map(source => <li key={source.source_id}><div><h3>{source.title}</h3><code>{source.source_id}</code>
            <details><summary>查看所选修订</summary><p>文档版本 <code>{source.version}</code></p><p>完整导入修订 <code>{source.import_revision}</code></p></details></div>
            <button className="button button-outline" disabled={disabled} type="button" onClick={() => void controller.import(source)}>导入此修订</button></li>)}
        </ul>}
      </section>
      <section aria-labelledby="import-recovery"><h2 id="import-recovery">本机导入记录</h2>
        <p>记录按账号隔离，最多 32 条。未确认记录不会自动删除或重试；清除浏览器数据、存储回收或换设备可能使记录丢失。这里不代表服务端的完整草稿列表。</p>
        {state.entries.length === 0 ? <p>此账号在本机没有保留记录。</p> : <>
          <a className="button button-outline" download="oj-import-recovery.json" href={`data:application/json;charset=utf-8,${encodeURIComponent(JSON.stringify({ format: 'oj-import-recovery-v1', records: state.entries }, null, 2))}`}>导出本机恢复记录</a>
          <ul className="management-list">{state.entries.map(entry => { const receipt = state.receipts.get(entry.key); return <li key={entry.key}><div><h3>{entry.source}</h3>
            <p>{receipt ? '服务端回执已核验' : entry.status === 'pending' ? '结果待确认' : '已保存回执 · 待重新核验'}</p>
            <details><summary>查看原请求与修订</summary><p>请求键 <code>{entry.key}</code></p><p>完整导入修订 <code>{entry.revision}</code></p>{entry.status === 'confirmed' && <p>草稿 ID <code>{entry.draftId}</code></p>}</details>
          </div><div className="management-actions">
            <button className="button button-outline" disabled={disabled} type="button" onClick={() => void controller.recover(entry)}>查询原回执</button>
            {entry.status === 'pending' && <button className="button button-outline" disabled={disabled} type="button" onClick={() => void controller.recover(entry, true)}>重试同一请求</button>}
            {receipt && <Link className="button button-primary" to={`/author/drafts/${receipt.draft_id}`}>打开草稿预览</Link>}
            {entry.status === 'confirmed' && <button className="button button-outline" disabled={disabled} type="button" onClick={() => void controller.remove(entry)}>移除本机已确认记录</button>}
          </div></li> })}</ul>
        </>}
      </section>
    </>}
    <p><Link to="/author/drafts/open">通过已有 ID 打开草稿</Link></p>
  </div>
}
