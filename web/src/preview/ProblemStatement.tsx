import { Link } from 'react-router'
import { Sample } from '../statement/Sample'
import { StatementMarkdown } from '../statement/StatementMarkdown'
import type { PreviewProblem } from './problems'

export function ProblemStatement({ problem }: { problem: PreviewProblem }) {
  return <article className="problem-statement" aria-labelledby="problem-title">
    <p className="breadcrumb"><Link to="/">题库</Link><span>/</span>{problem.id}</p>
    <h1 id="problem-title">{problem.title}</h1>
    <div className="problem-meta"><span className={problem.difficulty === '简单' ? 'difficulty-easy' : 'difficulty-medium'}>{problem.difficulty}</span><span>示例时间限制 1 s</span><span>示例内存限制 128 MB</span></div>
    <StatementMarkdown source={`## 题目描述\n\n${problem.description}\n\n## 输入格式\n\n${problem.input}\n\n## 输出格式\n\n${problem.output}`} />
    <section><h2>样例</h2><div className="samples"><Sample label="输入" text={problem.sampleInput} /><Sample label="输出" text={problem.sampleOutput} /></div></section>
    <section><h2>数据范围</h2><p className="sample-range">{problem.range}</p></section>
  </article>
}
