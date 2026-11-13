import { lazy, Suspense } from 'react'
import { Route, Routes } from 'react-router'
import { AccountProvider } from './account/AccountProvider'
import { AccountPanel } from './account/AccountPanel'
import { SiteHeader } from './layout/SiteHeader'
import { SiteFooter } from './layout/SiteFooter'
import { RouteFocus } from './layout/RouteFocus'
import { LibraryPage } from './pages/LibraryPage'
import { LoginPage } from './pages/LoginPage'
import { EmailRequestPage } from './pages/EmailRequestPage'
import { NotFoundPage } from './pages/StatusPages'
import { SubmissionsPage } from './training/SubmissionsPage'
import { PreviewDraftProvider } from './preview/PreviewDraftProvider'
import './account/account.css'

const ProblemPage = lazy(() => import('./preview/ProblemPage'))
const DraftPreviewPage = lazy(() => import('./drafts/DraftPreviewPage'))
const DraftManagementPage = lazy(() => import('./drafts/DraftManagementPage'))
const ReviewMaterialsPage = lazy(() => import('./reviews/ReviewMaterialsPage'))

export function App() {
  return (
    <AccountProvider>
      <PreviewDraftProvider>
      <a className="skip-link" href="#main-content">跳到主要内容</a>
      <SiteHeader />
      <main id="main-content" tabIndex={-1}>
        <RouteFocus />
        <Routes>
          <Route path="/" element={<LibraryPage />} />
          <Route path="/login" element={<LoginPage />} />
          <Route path="/account/register" element={<EmailRequestPage key="registration" kind="registration" />} />
          <Route path="/account/forgot-password" element={<EmailRequestPage key="recovery" kind="recovery" />} />
          <Route path="/account" element={<AccountPanel />} />
          <Route path="/submissions" element={<SubmissionsPage />} />
          <Route path="/author/drafts" element={<Suspense fallback={<p className="page-width loading-state" role="status">正在加载导入管理…</p>}><DraftManagementPage /></Suspense>} />
          <Route path="/author/drafts/open" element={<Suspense fallback={<p className="page-width loading-state" role="status">正在加载草稿预览…</p>}><DraftPreviewPage /></Suspense>} />
          <Route path="/author/drafts/:id" element={<Suspense fallback={<p className="page-width loading-state" role="status">正在加载草稿预览…</p>}><DraftPreviewPage /></Suspense>} />
          <Route path="/review/materials/:id?" element={<Suspense fallback={<p className="page-width loading-state" role="status">正在加载审核材料页…</p>}><ReviewMaterialsPage /></Suspense>} />
          <Route path="/preview/problems/:id" element={<Suspense fallback={<p className="page-width loading-state" role="status">正在加载编辑器…</p>}><ProblemPage /></Suspense>} />
          <Route path="*" element={<NotFoundPage />} />
        </Routes>
      </main>
      <SiteFooter />
      </PreviewDraftProvider>
    </AccountProvider>
  )
}
