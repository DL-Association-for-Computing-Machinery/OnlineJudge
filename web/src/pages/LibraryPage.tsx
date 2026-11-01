import { Link, useSearchParams } from 'react-router'
import { previewProblems, selectPreviewProblems } from '../preview/problems'
import { Icon } from '../ui/Icon'

const pageSize = 6
const topics = [...new Set(previewProblems.map(problem => problem.topic))]

export function LibraryPage() {
  const [params, setParams] = useSearchParams()
  const query = (params.get('q') ?? '').slice(0, 80)
  const difficulty = params.get('difficulty') ?? ''
  const topic = params.get('topic') ?? ''
  const problems = selectPreviewProblems(query, difficulty, topic)
  const pages = Math.max(1, Math.ceil(problems.length / pageSize))
  const candidate = Number(params.get('page') ?? 1)
  const page = Number.isSafeInteger(candidate) ? Math.max(1, Math.min(pages, candidate)) : 1
  const update = (name: string, value: string) => {
    setParams(previous => {
      const next = new URLSearchParams(previous)
      if (value) next.set(name, value)
      else next.delete(name)
      if (name !== 'page') next.delete('page')
      return next
    }, { replace: true })
  }
  return (
    <section className="library-page page-width" aria-labelledby="library-title">
      <h1 id="library-title">题库</h1>
      <div className="library-tools">
        <div className="search-field"><Icon name="search" /><input type="search" aria-label="搜索示例题目" placeholder="搜索题号或题目名称" maxLength={80} value={query} onChange={event => update('q', event.target.value)} /></div>
        <details className="filters">
          <summary className="button button-neutral"><Icon name="filter" />筛选{difficulty || topic ? <span className="filter-dot" aria-label="已应用筛选" /> : null}</summary>
          <div className="filter-fields">
            <label>难度<select value={difficulty} onChange={event => update('difficulty', event.target.value)}><option value="">全部难度</option><option>简单</option><option>中等</option></select></label>
            <label>标签<select value={topic} onChange={event => update('topic', event.target.value)}><option value="">全部标签</option>{topics.map(item => <option key={item}>{item}</option>)}</select></label>
            <button className="text-button" type="button" onClick={() => setParams({})}>清除筛选</button>
          </div>
        </details>
      </div>
      <p className="sr-only" role="status">找到 {problems.length} 道示例题目，当前第 {page} 页。</p>
      <table className="problem-table">
        <caption className="sr-only">界面预览：示例题目，尚未开放正式题库与提交</caption>
        <thead><tr><th scope="col">题号</th><th scope="col">题目</th><th scope="col">难度</th></tr></thead>
        <tbody>{problems.slice((page - 1) * pageSize, page * pageSize).map(problem => (
          <tr key={problem.id}><td className="problem-id">{problem.id}</td><td><Link to={`/preview/problems/${problem.id}`}>{problem.title}</Link></td><td><span className={problem.difficulty === '简单' ? 'difficulty-easy' : 'difficulty-medium'}>{problem.difficulty}</span></td></tr>
        ))}</tbody>
      </table>
      {problems.length === 0 ? <div className="empty-state"><p>没有匹配的示例题目</p><button type="button" className="text-button" onClick={() => setParams({})}>清除搜索与筛选</button></div> : null}
      <nav className="pagination" aria-label="示例题目分页">
        <button className="button-neutral" type="button" disabled={page === 1} onClick={() => update('page', String(page - 1))}><Icon name="back" />上一页</button>
        {Array.from({ length: pages }, (_, index) => index + 1).map(number => <button type="button" key={number} className={page === number ? 'page-current' : 'button-neutral'} aria-current={page === number ? 'page' : undefined} aria-label={`第 ${number} 页`} onClick={() => update('page', String(number))}>{number}</button>)}
        <button className="button-neutral" type="button" disabled={page === pages} onClick={() => update('page', String(page + 1))}>下一页<Icon name="chevron" /></button>
      </nav>
    </section>
  )
}
