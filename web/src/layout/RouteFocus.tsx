import { useEffect } from 'react'
import { useLocation, useNavigate } from 'react-router'

export function RouteFocus() {
  const { pathname, hash } = useLocation()
  const navigate = useNavigate()
  useEffect(() => {
    if (pathname === '/' && hash === '#account-title') {
      void navigate('/account', { replace: true })
      return
    }
    document.getElementById('main-content')?.focus({ preventScroll: true })
    window.scrollTo(0, 0)
    const title = pathname === '/login' ? '登录' : pathname === '/account' ? '个人账号'
      : pathname === '/account/register' ? '注册账号' : pathname === '/account/forgot-password' ? '找回密码'
      : pathname === '/submissions' ? '我的提交' : pathname.startsWith('/preview/') ? '做题页预览' : '题库'
    document.title = `${title} · ECUT OJ · OnlineJudge`
  }, [pathname, hash, navigate])
  return null
}
