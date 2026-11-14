import assert from 'node:assert/strict'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import process from 'node:process'
import { createRequire } from 'node:module'

const require = createRequire(import.meta.url)
const { chromium } = require(process.env.OJ_PLAYWRIGHT_MODULE)
const output = process.env.OJ_BROWSER_QA_OUTPUT
const info = JSON.parse(fs.readFileSync(process.env.OJ_BROWSER_QA_INFO, 'utf8'))
const origin = new URL(info.origin)
assert(origin.protocol === 'https:' && origin.hostname === '127.0.0.1' && origin.port && origin.origin === info.origin)
const report = { checks: [], success: false }
const scratch = fs.mkdtempSync(path.join(os.tmpdir(), 'oj-remembered-profile-'))
const sessionName = '__Host-oj_session'
let context
let phase = 'launch'
let writes = 0
const passed = name => report.checks.push(name)

async function launch(profile, viewport) {
  assert(!context)
  context = await chromium.launchPersistentContext(path.join(scratch, profile), {
    headless: true, ignoreHTTPSErrors: true, viewport,
    ...(process.env.OJ_CHROMIUM_EXECUTABLE ? { executablePath: process.env.OJ_CHROMIUM_EXECUTABLE } : {}),
    args: ['--disable-background-networking'],
  })
  await context.route('**/*', route => {
    if (new URL(route.request().url()).origin !== origin.origin) return route.abort()
    return route.continue()
  })
  context.on('request', request => {
    if (request.method() === 'POST' && new URL(request.url()).pathname.startsWith('/api/')) writes++
  })
  return context.pages()[0] ?? await context.newPage()
}

async function close() {
  if (!context) return
  await context.close()
  context = undefined
}

async function waitText(page, text) {
  await page.getByText(text, { exact: true }).waitFor({ state: 'visible', timeout: 8000 })
}

async function openAccount(page, loggedIn) {
  const before = writes
  await page.goto(origin.origin + '/account')
  await waitText(page, loggedIn ? '已登录' : '未登录')
  if (loggedIn) await waitText(page, info.subject_id)
  assert.equal(writes, before, 'Opening account performs no API write')
  assert(await page.evaluate(() => document.cookie === '' && localStorage.length === 0 && sessionStorage.length === 0), 'No script-readable credential')
}

async function login(page, remember) {
  await page.goto(origin.origin + '/login')
  const checkbox = page.getByRole('checkbox', { name: /在此设备上保持登录/ })
  assert.equal(await checkbox.isChecked(), false)
  if (remember) await checkbox.check()
  await page.getByLabel('邮箱', { exact: true }).fill(info.email)
  await page.getByLabel('密码', { exact: true }).fill('synthetic protocol password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await waitText(page, '登录成功，当前会话已核验。')
  const cookie = await readCookie()
  assert(cookie && cookie.httpOnly && cookie.secure && cookie.path === '/' && cookie.sameSite === 'Lax', 'Session cookie policy')
  assert(remember ? cookie.expires > Date.now() / 1000 : cookie.expires === -1, 'Expected cookie lifetime')
  return cookie.expires
}

async function readCookie() {
  const cookies = await context.cookies(origin.origin)
  assert(!cookies.some(cookie => cookie.name === '__Host-oj_login'), 'Pending cookie cleared')
  return cookies.find(cookie => cookie.name === sessionName)
}

try {
  const desktop = { width: 1280, height: 960 }
  const narrow = { width: 390, height: 844 }
  phase = 'default login'
  let page = await launch('default', desktop)
  await login(page, false)
  await openAccount(page, true)
  await close()
  page = await launch('default', desktop)
  assert(!(await readCookie()), 'Clean Chromium restart does not retain the ordinary session cookie')
  await openAccount(page, false)
  await close()
  passed('default cookie is session-only across a clean Chromium restart')

  phase = 'remembered login'
  page = await launch('remembered', narrow)
  const expires = await login(page, true)
  await close()
  page = await launch('remembered', narrow)
  const cookie = await readCookie()
  assert(cookie && cookie.expires === expires, 'Original persistent deadline is retained')
  await openAccount(page, true)
  assert((await readCookie())?.expires === expires, 'Read did not change the cookie deadline')
  passed('opt-in login survives Chromium restart with its original deadline')

  phase = 'logout and restart'
  await page.getByRole('button', { name: '退出当前会话', exact: true }).click()
  await waitText(page, '未登录')
  assert(!(await readCookie()), 'Logout cleared the persistent cookie')
  await close()
  page = await launch('remembered', narrow)
  assert(!(await readCookie()), 'Logout remained cleared after restart')
  await openAccount(page, false)
  await close()
  passed('logout remains effective after restarting the same Chromium profile')
  report.success = true
} catch {
  // Do not print assertions or browser errors that could contain credentials.
  report.failed_phase = phase
  process.exitCode = 1
} finally {
  await close()
  fs.rmSync(scratch, { recursive: true })
  fs.writeFileSync(path.join(output, 'browser-qa.json'), JSON.stringify(report, null, 2) + '\n')
}
