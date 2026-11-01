import { useState } from 'react'
import { Link } from 'react-router'
import { Icon } from '../ui/Icon'
import type { PreviewProblem } from './problems'

function Sample({ label, text }: { label: string; text: string }) {
  const [notice, setNotice] = useState('')
  async function copy() {
    try { await navigator.clipboard.writeText(text); setNotice('已复制') }
    catch { setNotice('无法复制，请手动选择样例。') }
  }
  return <div className="sample"><div className="sample-heading"><h3>{label}</h3><button type="button" className="text-button" aria-label={`复制样例${label}`} onClick={() => void copy()}><Icon name={notice === '已复制' ? 'check' : 'copy'} />{notice === '已复制' ? notice : '复制'}</button></div><pre>{text}</pre><span className="sr-only" role="status">{notice}</span></div>
}

export function ProblemStatement({ problem }: { problem: PreviewProblem }) {
  return <article className="problem-statement" aria-labelledby="problem-title">
    <p className="breadcrumb"><Link to="/">题库</Link><span>/</span>{problem.id}</p>
    <h1 id="problem-title">{problem.title}</h1>
    <div className="problem-meta"><span className={problem.difficulty === '简单' ? 'difficulty-easy' : 'difficulty-medium'}>{problem.difficulty}</span><span>示例时间限制 1 s</span><span>示例内存限制 128 MB</span></div>
    <section><h2>题目描述</h2><p>{problem.description}</p></section>
    <section><h2>输入格式</h2><p>{problem.input}</p></section>
    <section><h2>输出格式</h2><p>{problem.output}</p></section>
    <section><h2>样例</h2><div className="samples"><Sample label="输入" text={problem.sampleInput} /><Sample label="输出" text={problem.sampleOutput} /></div></section>
    <section><h2>数据范围</h2><p className="sample-range">{problem.range}</p></section>
  </article>
}
