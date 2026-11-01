import { useLocation } from 'react-router'

export function SiteFooter() {
  const { pathname } = useLocation()
  return <footer className="site-footer"><span>东华理工大学计算机协会</span><span>{pathname === '/' || pathname.startsWith('/preview/') ? '开发预览 · 示例题目，暂不接受正式提交' : '开发预览'}</span></footer>
}
