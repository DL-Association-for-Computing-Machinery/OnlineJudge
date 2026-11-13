import { Link } from 'react-router'

export function NotFoundPage() {
  return <section className="page-width status-page"><h1>页面不存在</h1><p>这个地址没有可用页面。</p><Link className="button button-neutral" to="/">返回题库</Link></section>
}
