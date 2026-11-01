import { Link } from 'react-router'
import { useCurrentAccount } from '../account/context'

export function SubmissionsPage() {
  const { state } = useCurrentAccount()
  return <section className="page-width status-page"><h1>我的提交</h1><div className="empty-state"><h2>正式提交尚未开放</h2><p>做题页目前为界面预览，不会创建判题任务或提交记录。</p>{state.status !== 'authenticated' ? <Link className="button button-neutral" to="/login">登录已有账号</Link> : null}<Link className="text-link" to="/">返回题库预览</Link></div></section>
}

export function NotFoundPage() {
  return <section className="page-width status-page"><h1>页面不存在</h1><p>这个地址没有可用页面。</p><Link className="button button-neutral" to="/">返回题库</Link></section>
}
