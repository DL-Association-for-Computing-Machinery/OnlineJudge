import { useEffect, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router'
import { listCatalog, readCatalog } from '../services/catalog'
import { Sample } from '../statement/Sample'
import { StatementMarkdown } from '../statement/StatementMarkdown'

type Page = Awaited<ReturnType<typeof listCatalog>>
type Detail = NonNullable<Awaited<ReturnType<typeof readCatalog>>>
type Lookup<T> = { status: 'loading' | 'error' | 'missing' } | { status: 'ready'; value: T }

function CatalogRows({ query, after }: { query: string; after: string }) {
  const [result, setResult] = useState<Lookup<Page>>({ status: 'loading' })
  useEffect(() => {
    const abort = new AbortController()
    void listCatalog(query, after, abort.signal).then(value => {
      if (!abort.signal.aborted) setResult({ status: 'ready', value })
    }, () => { if (!abort.signal.aborted) setResult({ status: 'error' }) })
    return () => abort.abort()
  }, [query, after])
  if (result.status !== 'ready') return <p role="status">{result.status === 'loading' ? '正在加载题库…' : '题库暂不可用，请稍后重试。'}</p>
  return <>
    {result.value.entries.length === 0 ? <p role="status">没有匹配的已发布题目。</p> :
      <ul>{result.value.entries.map(entry => <li key={entry.id}><Link to={`/problems/${entry.id}`}>{entry.title}</Link></li>)}</ul>}
    {result.value.next && <Link to={'?' + new URLSearchParams({ q: query, after: result.value.next }).toString()}>下一页</Link>}
    {after && <p><Link to={'?' + new URLSearchParams({ q: query }).toString()}>返回首页</Link></p>}
  </>
}

export function CatalogPage() {
  const [params, setParams] = useSearchParams()
  const query = params.get('q') ?? ''
  const after = params.get('after') ?? ''
  return <section className="library-page page-width">
    <h1>已发布题库</h1>
    <CatalogSearch key={query} query={query} onSearch={value => setParams(value ? { q: value } : {})} />
    <CatalogRows key={JSON.stringify([query, after])} query={query} after={after} />
  </section>
}

function CatalogSearch({ query, onSearch }: { query: string; onSearch: (value: string) => void }) {
  const [search, setSearch] = useState(query)
  const exceedsLimit = new TextEncoder().encode(search.trim()).length > 128
  return <form className="library-tools" onSubmit={event => {
    event.preventDefault()
    if (!exceedsLimit) onSearch(search.trim())
  }}>
    <label>题目名称 <input type="search" maxLength={128} value={search} onChange={event => setSearch(event.target.value)} /></label>
    <button type="submit" className="button button-neutral" disabled={exceedsLimit}>搜索</button>
    {exceedsLimit && <p role="status">搜索词太长。</p>}
  </form>
}

function Statement({ id }: { id: string }) {
  const [result, setResult] = useState<Lookup<Detail>>({ status: 'loading' })
  useEffect(() => {
    const abort = new AbortController()
    void readCatalog(id, abort.signal).then(value => {
      if (!abort.signal.aborted) setResult(value ? { status: 'ready', value } : { status: 'missing' })
    }, () => { if (!abort.signal.aborted) setResult({ status: 'error' }) })
    return () => abort.abort()
  }, [id])
  if (result.status !== 'ready') return <p role="status">{result.status === 'loading' ? '正在加载题面…' :
    result.status === 'missing' ? '找不到这道已发布题目。' : '题面暂不可用，请稍后重试。'}</p>
  const data = result.value
  return <>
    <h1>{data.title}</h1>
    <StatementMarkdown source={data.markdown} images={data.images} />
    {data.examples.length > 0 && <section aria-label="样例"><h2>样例</h2>{data.examples.map(example =>
      <div key={example.id} className="samples"><Sample label="输入" text={example.input} /><Sample label="输出" text={example.output} /></div>
    )}</section>}
  </>
}

export function ReleasedProblemPage() {
  const { id = '' } = useParams()
  return <article className="page-width private-draft-page"><p><Link to="/problems">返回已发布题库</Link></p><Statement key={id} id={id} /></article>
}
