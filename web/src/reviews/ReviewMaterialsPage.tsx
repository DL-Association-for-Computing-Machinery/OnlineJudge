import { useEffect, useState, useSyncExternalStore } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { useCurrentAccount } from '../account/context'
import { subscribePrivateInvalidation } from '../account/privateInvalidation'
import { validDraftID } from '../services/privateDraft'
import { Sample } from '../statement/Sample'
import { StatementMarkdown } from '../statement/StatementMarkdown'
import { ReviewMaterialsController } from './materialsController'
import { StatementConfirmation } from './StatementConfirmation'

function AssignedMaterials({ id }: { id: string }) {
  const { controller: account, state: accountState } = useCurrentAccount()
  const [controller] = useState(() => new ReviewMaterialsController(account, id))
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
  const data = state.data
  return <>
    <p>送审 ID：<code>{id}</code></p>
    <button type="button" className="button button-primary" disabled={state.busy || accountState.busy} onClick={() => void controller.load()}>检查会话并读取材料</button>
    <p role="status">{state.notice}</p>
    {accountState.status !== 'authenticated' && <p>{accountState.notice.text} <Link to="/login">前往登录</Link></p>}
    {data && <>
      <article className="problem-statement" aria-labelledby="review-title">
        <h2 id="review-title">{data.statement.title}</h2>
        <p>分配修订 {data.evidence.assignment_revision} · 正式参考结果 {data.evidence.formal_verdict}</p>
        <StatementMarkdown source={data.statement.markdown} images={data.statement.images} />
        {data.statement.examples.length > 0 && <section aria-label="原始样例"><h2>样例</h2>{data.statement.examples.map(example => <div key={example.id}>
          <h3>{example.id}</h3><div className="samples"><Sample label="输入" text={example.input} /><Sample label="输出" text={example.output} /></div>
        </div>)}</section>}
      </article>
      <section className="problem-statement" aria-labelledby="review-solution">
        <h2 id="review-solution">冻结标程</h2>
        <div className="sample"><pre><code>{data.standard_solution.source}</code></pre></div>
        {data.standard_solution.dependencies?.map(dep => <details className="sample" key={dep.name}>
          <summary>{dep.name}</summary><pre><code>{dep.source}</code></pre>
        </details>)}
      </section>
      <StatementConfirmation controller={controller} state={state} />
    </>}
  </>
}

export default function ReviewMaterialsPage() {
  const { id } = useParams()
  const [input, setInput] = useState('')
  const navigate = useNavigate()
  return <div className="page-width private-draft-page">
    <p className="breadcrumb"><Link to="/">题库</Link> / 审核材料</p>
    <h1>审核材料</h1>
    <p>使用维护者提供的送审 ID 查看冻结题面、样例和标程。材料仅向当前指定的审核人开放。</p>
    <form className="private-draft-form" onSubmit={event => { event.preventDefault(); if (validDraftID(input)) void navigate(`/review/materials/${input}`) }}>
      <label>送审 ID<input value={input} onChange={event => setInput(event.target.value)} required pattern="[a-f0-9]{32}" maxLength={32} autoComplete="off" spellCheck={false} /></label>
      <button className="button button-outline" type="submit">选择送审项</button>
    </form>
    {id && (validDraftID(id) ? <AssignedMaterials key={id} id={id} /> : <p role="alert">送审 ID 格式不正确。</p>)}
  </div>
}
