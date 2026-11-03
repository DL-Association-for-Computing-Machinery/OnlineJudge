import { Link, NavLink, useLocation } from 'react-router'
import { useCurrentAccount } from '../account/context'
import { Icon } from '../ui/Icon'

export function SiteHeader() {
  const { pathname } = useLocation()
  const { state } = useCurrentAccount()
  const loginPage = pathname === '/login'
  return (
    <header className="site-header">
      <div className="header-inner">
        <Link className="brand" to="/" aria-label="ECUT OJ 首页">ECUT OJ</Link>
        {loginPage ? <Link className="return-link" to="/"><Icon name="back" />返回题库</Link> : (
          <>
            <nav className="main-nav" aria-label="主导航">
              <Link to="/" aria-current={pathname === '/' ? 'page' : pathname.startsWith('/preview/problems/') ? 'location' : undefined}>题库</Link>
              <NavLink to="/submissions">我的提交</NavLink>
              {state.status === 'authenticated' && <NavLink to="/author/drafts">私有草稿</NavLink>}
            </nav>
            <Link className="button button-outline account-nav" to={state.status === 'authenticated' ? '/account' : '/login'}>
              {state.status === 'authenticated' ? '个人账号' : '登录'}
            </Link>
          </>
        )}
      </div>
    </header>
  )
}
