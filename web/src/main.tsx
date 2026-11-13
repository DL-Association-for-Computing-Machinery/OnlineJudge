import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router'
import { App } from './App'
import { EmailConfirmationPage } from './account/EmailConfirmationPage'
import { captureEmailConfirmation } from './account/emailConfirmation'
import './styles.css'

const root = document.getElementById('root')
if (!root) throw new Error('缺少应用挂载节点')
const confirmation = captureEmailConfirmation(window.location, path => window.history.replaceState(null, '', path))
if (confirmation) {
  document.title = (confirmation.kind === 'activation' ? '验证邮箱' : '重置密码') + ' · ECUT OJ · OnlineJudge'
}
createRoot(root).render(<StrictMode>{confirmation
  ? <EmailConfirmationPage {...confirmation} />
  : <BrowserRouter><App /></BrowserRouter>}</StrictMode>)
