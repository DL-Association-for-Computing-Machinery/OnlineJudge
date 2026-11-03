import { useEffect, useState, useSyncExternalStore } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { useCurrentAccount } from '../account/context'
import { subscribePrivateInvalidation } from '../account/privateInvalidation'
import { validDraftID } from '../services/privateDraft'
import { Sample } from '../statement/Sample'
import { StatementMarkdown } from '../statement/StatementMarkdown'
import { DraftPreviewController } from './controller'

function Preview({ id }: { id: string }) {
  const { controller: account, state: accountState } = useCurrentAccount()
  const [controller] = useState(() => new DraftPreviewController(account, id))
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot)
  useEffect(() => {
    controller.connect()
    const unsubscribe = subscribePrivateInvalidation(controller.clear)
    // Restoring or refocusing a page first drops its private result. Reload is
    // explicit, so an invalidation can never silently re-authorize old data.
    const clear = () => controller.clear()
    window.addEventListener('focus', clear)
    window.addEventListener('pagehide', clear)
    window.addEventListener('pageshow', clear)
    document.addEventListener('visibilitychange', clear)
    return () => {
      unsubscribe(); controller.disconnect()
      window.removeEventListener('focus', clear); window.removeEventListener('pagehide', clear); window.removeEventListener('pageshow', clear)
      document.removeEventListener('visibilitychange', clear)
    }
  }, [controller])
  const data = state.data
  return <>
    <p><code>{id}</code></p>
    <button type="button" className="button button-primary" disabled={state.busy || accountState.busy} onClick={() => void controller.load()}>检查会话并打开草稿</button>
    <p role="status">{state.notice}</p>
    {accountState.status !== 'authenticated' && <p>{accountState.notice.text} <Link to="/login">前往登录</Link></p>}
    {data && <article className="problem-statement" aria-labelledby="draft-title">
      <h2 id="draft-title">{data.title}</h2><p>版本 {data.version} · 英文题面 · 私有草稿</p>
      <StatementMarkdown source={data.markdown} />
      <section aria-label="原始样例"><h2>样例</h2>{data.examples.map(example => <div key={example.id}><h3>{example.id}</h3><div className="samples"><Sample label="输入" text={example.input} /><Sample label="输出" text={example.output} /></div></div>)}</section>
    </article>}
  </>
}

export default function DraftPreviewPage() {
  const { id } = useParams()
  const [input, setInput] = useState('')
  const navigate = useNavigate()
  return <div className="page-width private-draft-page">
    <p className="breadcrumb"><Link to="/">题库</Link> / 本人私有草稿</p>
    <h1>私有草稿预览</h1>
    <p>输入已导入草稿的 ID。仅作者本人可访问；导入、审核和发布管理尚未开放。图片资源暂不可用。</p>
    <form className="private-draft-form" onSubmit={event => { event.preventDefault(); if (validDraftID(input)) void navigate(`/author/drafts/${input}`) }}>
      <label>草稿 ID<input value={input} onChange={event => setInput(event.target.value)} required pattern="[a-f0-9]{32}" maxLength={32} autoComplete="off" spellCheck={false} placeholder="32 位小写十六进制 ID" /></label>
      <button className="button button-outline" type="submit">选择草稿</button>
    </form>
    {id && (validDraftID(id) ? <Preview key={id} id={id} /> : <p role="alert">草稿 ID 格式不正确。</p>)}
  </div>
}
