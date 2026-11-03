import fs from 'node:fs'
import assert from 'node:assert/strict'
import path from 'node:path'
import process from 'node:process'
import console from 'node:console'
import { URL } from 'node:url'
import { createRequire } from 'node:module'
import { runPreviewChecks } from './preview-qa.mjs'
import { runDraftChecks } from './draft-qa.mjs'
import { runManagementChecks } from './management-qa.mjs'
import { runOwnListChecks } from './own-list-qa.mjs'

// The Python runner validates these local paths. This driver never installs a
// browser or package, and always creates a fresh profile for synthetic accounts.
const require = createRequire(import.meta.url)
const modulePath = process.env.OJ_PLAYWRIGHT_MODULE
const output = process.env.OJ_BROWSER_QA_OUTPUT
assert(modulePath && output && process.env.OJ_BROWSER_QA_INFO, 'Use the isolated Python browser runner')
const { chromium } = require(modulePath)
const version = require(path.join(path.dirname(require.resolve(modulePath)), 'package.json')).version
const info = JSON.parse(fs.readFileSync(process.env.OJ_BROWSER_QA_INFO, 'utf8'))
for (const [value, protocol] of [[info.origin, 'https:'], [info.http_origin, 'http:']]) {
  const url = new URL(value)
  assert(url.protocol === protocol && url.hostname === '127.0.0.1' && url.port && url.origin === value, 'Fixture origin must be loopback only')
}
const log = fs.readFileSync(info.log, 'utf8')
const ready = JSON.parse(log.match(/^BROWSER_READY (.+)$/m)?.[1] ?? 'null')
assert(ready && ready.url === info.origin, 'Fixture must be authoritatively ready')
const oldPassword = 'synthetic protocol password'
const replacement = 'synthetic browser replacement password'
const finalPassword = ' 原始 e\u0301 browser replacement password '
let phase = 'launch'
let browser, context, page
const errors = [], diagnostics = [], calls = []
const routeFailures = []
const report = { driver: 'existing Playwright ' + version, viewports: ['1280x960', '390x844'], checks: [], screenshots: [] }
const expectText = async text => page.getByText(text, { exact: true }).waitFor({ state: 'visible', timeout: 6000 })
const stats = async () => (await context.request.get(info.origin + '/__fixture/stats')).json()
const count = (s, path) => s.api_requests['POST ' + path] || 0
// Playwright invokes route callbacks outside the main workflow's try/catch.
// Keep raw network diagnostics (which may include Cookie) out of stderr.
function safeRoute(handler) {
  return async route => {
    try { await handler(route) }
    catch {
      routeFailures.push('ROUTE_CALLBACK_FAILED')
      await route.abort().catch(() => {})
    }
  }
}
function passed(name) { report.checks.push(name); console.log('PASS ' + name) }
async function login(secret, double = false) {
  await page.getByLabel('邮箱', { exact: true }).fill(ready.email)
  await page.getByLabel('密码', { exact: true }).fill(secret)
  if (double) await page.locator('form.account-form').evaluate(form => { form.requestSubmit(); form.requestSubmit() })
  else await page.getByRole('button', { name: '登录', exact: true }).click()
}
async function changePassword(current, next) {
  await page.getByLabel('当前密码', { exact: true }).fill(current)
  await page.getByLabel('新密码', { exact: true }).fill(next)
  await page.getByLabel('再次输入新密码', { exact: true }).fill(next)
  await page.getByRole('button', { name: '确认修改密码', exact: true }).click()
}
async function recovery() {
  const details = page.locator('.account-recovery details')
  if (!(await details.getAttribute('open') !== null)) await details.locator('summary').click()
}
async function shot(name, fullPage = true) {
  const image = path.join(output, name + '.png')
  await page.screenshot({ path: image, fullPage })
  report.screenshots.push(image)
}
async function checkStorage() {
  assert(await page.evaluate(() => document.cookie === '' && localStorage.length === 0 && sessionStorage.length === 0), 'No script-readable credential or storage')
}
async function injectOnce(path, fulfill, action) {
  let seen = 0
  const handler = safeRoute(async route => { seen++; await route.fulfill(fulfill) })
  await page.route('**' + path, handler)
  try {
    await action()
    assert.equal(routeFailures.length, 0, 'Route callback failed safely')
    assert.equal(seen, 1, 'One explicit request, no automatic retry')
  }
  finally { await page.unroute('**' + path, handler) }
}
const problem = (status, code) => ({ status, contentType: 'application/problem+json', body: JSON.stringify({ type: 'about:blank', status, code, title: 'untrusted title' }) })

;(async () => {
  try {
    browser = await chromium.launch({ headless: true, ...(process.env.OJ_CHROMIUM_EXECUTABLE ? { executablePath: process.env.OJ_CHROMIUM_EXECUTABLE } : {}), args: ['--disable-background-networking'] })
    report.browser = browser.version()
    context = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1280, height: 960 } })
    await context.route('**/*', safeRoute(route => {
      const url = new URL(route.request().url())
      return [info.origin, info.http_origin].includes(url.origin) ? route.continue() : route.abort()
    }))
    page = await context.newPage()
    await page.clock.install({ time: new Date() })
    page.on('pageerror', error => errors.push(error.name))
    page.on('console', message => {
      if (['error', 'warning'].includes(message.type())) {
        const text = message.text()
        diagnostics.push({ type: message.type(), expected: /Failed to load resource|redirect|Redirect/.test(text) })
      }
    })
    page.on('request', request => {
      const url = new URL(request.url())
      if (url.pathname.startsWith('/api/')) calls.push(request.method() + ' ' + url.pathname)
    })
    phase = 'initial screen'
    await page.goto(info.origin, { waitUntil: 'networkidle' })
    await page.getByRole('heading', { name: '题库', exact: true }).waitFor()
    assert.equal(page.url(), info.origin + '/')
    assert((await page.title()).includes('OnlineJudge'), 'Expected page title')
    assert((await page.locator('main').innerText()).includes('A+B Problem'), 'Meaningful preview library')
    assert.equal(await page.getByLabel('密码', { exact: true }).count(), 0, 'Home has no credential form')
    await page.getByRole('link', { name: '登录', exact: true }).click()
    await page.getByLabel('密码', { exact: true }).waitFor()
    assert.equal(page.url(), info.origin + '/login', 'Standalone login route')
    assert((await page.locator('main').innerText()).includes('使用已有账号登录'), 'Meaningful login content')
    assert.equal(await page.locator('vite-error-overlay').count(), 0)
    assert(!calls.some(call => call.startsWith('POST ')), 'Mount performs no mutation')
    await shot('desktop-anonymous')
    await page.getByLabel('邮箱', { exact: true }).focus()
    await page.keyboard.press('Tab')
    assert(await page.getByLabel('密码', { exact: true }).evaluate(el => el === document.activeElement), 'Keyboard reaches password')
    await page.keyboard.press('Tab')
    assert(await page.getByRole('button', { name: '登录', exact: true }).evaluate(el => el === document.activeElement), 'Keyboard reaches login')
    passed('page identity, meaningful screen, no overlay, keyboard flow, mount read only')

    phase = 'real login and refresh'
    const beforeLogin = await stats()
    await login(oldPassword, true)
    await expectText('登录成功，当前会话已核验。')
    await page.getByRole('link', { name: '管理个人账号', exact: true }).click()
    assert.equal(page.url(), info.origin + '/account', 'Login and account routes share one real session')
    await expectText(ready.subject_id)
    await page.locator('.connection-check summary').click()
    await page.getByRole('button', { name: '检查 API 连接', exact: true }).click()
    await expectText('API 连接成功')
    assert.equal(count(await stats(), '/api/v1/session/login/begin') - count(beforeLogin, '/api/v1/session/login/begin'), 1)
    const cookies = await context.cookies(info.origin)
    assert(cookies.some(cookie => cookie.name === '__Host-oj_session' && cookie.secure && cookie.httpOnly && cookie.sameSite === 'Lax' && cookie.path === '/'), 'Real safe session cookie')
    assert(!cookies.some(cookie => cookie.name === '__Host-oj_login'), 'Pending cookie cleared')
    await checkStorage()
    const beforeRefresh = await stats()
    await page.reload({ waitUntil: 'networkidle' })
    await expectText('已登录')
    assert.equal(count(await stats(), '/api/v1/session/activity'), count(beforeRefresh, '/api/v1/session/activity'))
    await page.getByRole('button', { name: '继续本次会话', exact: true }).click()
    await expectText('当前会话有效。检查不会延长会话期限。')
    assert.equal(count(await stats(), '/api/v1/session/activity') - count(beforeRefresh, '/api/v1/session/activity'), 1)
    passed('real TLS→mTLS login, single action, HttpOnly cookies, refresh read only, explicit activity')

    phase = 'password refusal and local confirmation'
    await changePassword('incorrect synthetic password', replacement)
    await expectText('请核对当前密码；也可能是会话已失效，可检查当前会话。')
    await expectText('已登录')
    assert.equal(await page.getByLabel('当前密码', { exact: true }).inputValue(), '')
    const beforeMismatch = await stats()
    await page.getByLabel('当前密码', { exact: true }).fill(oldPassword)
    await page.getByLabel('新密码', { exact: true }).fill(replacement)
    await page.getByLabel('再次输入新密码', { exact: true }).fill('mismatch')
    await page.getByRole('button', { name: '确认修改密码', exact: true }).click()
    assert.equal(count(await stats(), '/api/v1/account/password'), count(beforeMismatch, '/api/v1/account/password'))
    await page.getByLabel('新密码', { exact: true }).fill('mismatch')
    assert(await page.getByLabel('再次输入新密码', { exact: true }).evaluate(el => el.validity.valid), 'Changing either field clears stale confirmation validity')
    await injectOnce('/api/v1/account/password', problem(404, 'NOT_FOUND'), async () => {
      await changePassword(oldPassword, replacement)
      await expectText('此入口尚未启用修改密码。')
      await expectText('已登录')
    })
    await page.reload({ waitUntil: 'networkidle' })
    await expectText('已登录')
    passed('wrong current password preserves session, confirmation stays local, optional 404 preserves session')

    phase = 'unknown password state across routes'
    await injectOnce('/api/v1/account/password', problem(503, 'PASSWORD_CHANGE_UNAVAILABLE'), async () => {
      await changePassword(oldPassword, replacement)
      await expectText('改密结果无法确认，密码可能已经修改。请退出并重新登录核验，勿重复提交改密。')
    })
    await page.getByRole('button', { name: '检查当前会话', exact: true }).click()
    await expectText('已登录')
    await page.getByRole('link', { name: 'ECUT OJ 首页', exact: true }).click()
    await page.getByRole('link', { name: 'A+B Problem', exact: true }).click()
    await page.getByRole('textbox', { name: '代码编辑器', exact: true }).waitFor()
    await page.getByRole('link', { name: '个人账号', exact: true }).click()
    await expectText('上次改密结果仍待核验。检查会话不能证明密码未改变，请退出并重新登录核验。')
    assert(await page.getByRole('button', { name: '确认修改密码', exact: true }).isDisabled(), 'SPA preview navigation cannot reset unknown password protection')
    await page.getByRole('button', { name: '退出当前会话', exact: true }).click()
    await login(oldPassword)
    await expectText('登录成功，当前会话已核验。')
    passed('unknown password guard survives library, preview and account navigation without document reload')

    phase = 'real password change and re-login'
    await changePassword(oldPassword, replacement)
    await expectText('密码已修改，原会话已失效。请使用新密码重新登录。')
    assert(!(await context.cookies(info.origin)).some(cookie => ['__Host-oj_session', '__Host-oj_login'].includes(cookie.name)), 'Change clears both browser cookies')
    await login(oldPassword)
    await expectText('邮箱、密码或原登录尝试无效。')
    await login(replacement)
    await expectText('登录成功，当前会话已核验。')
    await shot('desktop-authenticated')
    await page.getByRole('button', { name: '退出当前会话', exact: true }).click()
    await expectText('已退出当前会话。')
    passed('real password change clears cookies, old password denied, new login and confirmed logout')

    phase = 'HttpOnly pending recovery'
    await injectOnce('/api/v1/session/login/complete', problem(503, 'LOGIN_UNAVAILABLE'), async () => {
      await login(replacement)
      await expectText('登录结果无法确认。请检查当前会话，或继续确认原尝试；勿重新提交密码。')
    })
    assert((await context.cookies(info.origin)).some(cookie => cookie.name === '__Host-oj_login' && cookie.httpOnly && cookie.secure), 'Real pending cookie remains')
    await page.reload({ waitUntil: 'networkidle' })
    await expectText('未登录')
    const beforeRecovery = await stats()
    await recovery()
    await page.getByRole('button', { name: '继续确认原尝试', exact: true }).click()
    await expectText('登录成功，当前会话已核验。')
    assert.equal(count(await stats(), '/api/v1/session/login/begin'), count(beforeRecovery, '/api/v1/session/login/begin'))
    await page.getByRole('button', { name: '退出当前会话', exact: true }).click()
    await expectText('已退出当前会话。')
    await injectOnce('/api/v1/session/login/complete', problem(503, 'LOGIN_UNAVAILABLE'), async () => {
      await login(replacement)
      await expectText('登录结果无法确认。请检查当前会话，或继续确认原尝试；勿重新提交密码。')
    })
    await recovery()
    const beforeAbandon = await stats()
    await page.getByRole('button', { name: '放弃原登录尝试', exact: true }).click()
    await expectText('原登录尝试已放弃；这不等于确认退出，请检查当前会话。')
    await expectText('状态待确认')
    const afterAbandon = await stats()
    assert.equal(count(afterAbandon, '/api/v1/session/login/abandon') - count(beforeAbandon, '/api/v1/session/login/abandon'), 1)
    assert.equal(afterAbandon.api_requests['GET /api/v1/session'], beforeAbandon.api_requests['GET /api/v1/session'], 'Abandon does not invent a fresh observation')
    assert(!(await context.cookies(info.origin)).some(cookie => cookie.name === '__Host-oj_login'), 'Confirmed Abandon clears pending Cookie')
    await page.getByRole('button', { name: '检查当前会话', exact: true }).click()
    await expectText('未登录')
    passed('HttpOnly pending recovery and real Abandon use the original candidate and require a fresh observation')

    phase = 'redirect body protection'
    const beforeRedirect = await stats()
    const clientBefore = calls.length
    await injectOnce('/api/v1/session/login/begin', { status: 307, headers: { Location: info.origin + '/__fixture/redirect_sink' } }, async () => {
      await login(replacement)
      await expectText('登录结果无法确认。请检查当前会话，或继续确认原尝试；勿重新提交密码。')
    })
    assert.equal((await stats()).redirect_sink, 0, 'Redirect target receives no password request')
    assert.deepEqual(calls.slice(clientBefore), ['POST /api/v1/session/login/begin'], 'No redirect, Complete, automatic Read or retry')
    assert.equal(count(await stats(), '/api/v1/session/login/complete'), count(beforeRedirect, '/api/v1/session/login/complete'))
    await page.getByRole('button', { name: '检查当前会话', exact: true }).click()
    await expectText('未登录')
    passed('real browser 307 redirect refused before password body reaches sink; no dependent writes')

    phase = 'malformed accepted response'
    const malformedBefore = calls.length
    await injectOnce('/api/v1/session/login/begin', { status: 202, contentType: 'text/html', body: 'unexpected response' }, async () => {
      await login(replacement)
      await expectText('登录结果无法确认。请检查当前会话，或继续确认原尝试；勿重新提交密码。')
    })
    assert.deepEqual(calls.slice(malformedBefore), ['POST /api/v1/session/login/begin'])
    await page.getByRole('button', { name: '检查当前会话', exact: true }).click()
    await expectText('未登录')
    passed('malformed successful reply remains uncertain and does not issue Complete or retry')

    phase = 'issued response followed by failed observation'
    await injectOnce('/api/v1/session', problem(503, 'SESSION_UNAVAILABLE'), async () => {
      await login(replacement)
      await expectText('登录发放已确认，但当前会话尚未核验。结果暂时无法确认，请手动检查，勿重复提交。请检查当前会话，勿重新提交登录。')
    })
    const afterIssue = await stats()
    await page.getByRole('button', { name: '检查当前会话', exact: true }).click()
    await expectText('已登录')
    assert.equal(count(await stats(), '/api/v1/session/login/complete'), count(afterIssue, '/api/v1/session/login/complete'))
    passed('confirmed issuance survives failed Read; manual check does not replay Complete')

    phase = 'unknown committed password response'
    const beforeUnknown = await stats()
    const passwordHandler = safeRoute(async route => {
      const response = await route.fetch({ maxRedirects: 0 })
      assert.equal(response.status(), 204, 'Real password commit succeeded before response substitution')
      await route.fulfill(problem(503, 'PASSWORD_CHANGE_UNAVAILABLE'))
    })
    await page.route('**/api/v1/account/password', passwordHandler)
    await changePassword(replacement, finalPassword)
    await expectText('改密结果无法确认，密码可能已经修改。请退出并重新登录核验，勿重复提交改密。')
    assert.equal(routeFailures.length, 0, 'Route callback failed safely')
    await page.unroute('**/api/v1/account/password', passwordHandler)
    await page.getByRole('button', { name: '检查当前会话', exact: true }).click()
    await expectText('未登录')
    assert.equal(count(await stats(), '/api/v1/account/password') - count(beforeUnknown, '/api/v1/account/password'), 1)
    await expectText('上次改密结果仍待核验。检查会话不能证明密码未改变，请退出并重新登录核验。')
    await login(finalPassword)
    await expectText('登录成功，当前会话已核验。')
    assert.equal(await page.getByText('上次改密结果仍待核验。检查会话不能证明密码未改变，请退出并重新登录核验。', { exact: true }).count(), 0)
    await checkStorage()
    passed('committed password with lost success stays uncertain, no retry, raw Unicode and spaces re-login')

    phase = 'preview interaction and navigation regressions'
    report.screenshots.push(...await runPreviewChecks(browser, info, output))
    passed('preview identity gate, late upload isolation, source export, history retention, global leave guard and zero code POSTs')

    phase = 'private draft rendering and account fences'
    report.screenshots.push(...await runDraftChecks(browser, info, ready.email, finalPassword, output))
    passed('real owner draft render, math safety, local fonts, exact copy, mobile layout, late response and cross-tab invalidation')

    phase = 'private draft management and durable recovery'
    report.screenshots.push(...await runManagementChecks(browser, info, ready, finalPassword, output))
    passed('real pinned import, durable IndexedDB recovery, lost response GET-only refresh, owner switch refusal and storage failure fences')

    phase = 'server own draft discovery independent of local persistence'
    report.screenshots.push(...await runOwnListChecks(browser, info, ready, finalPassword, output))
    passed('real server own-list pagination, fresh-profile denied persistence discovery, actual preview and late next-page isolation')

    phase = 'mobile visual and HTTP boundary'
    await page.setViewportSize({ width: 390, height: 844 })
    await page.locator('#account-title').scrollIntoViewIfNeeded()
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'No mobile horizontal overflow')
    await shot('mobile-authenticated')
    await shot('mobile-account-viewport', false)
    const beforeExpiry = await stats()
    await page.clock.fastForward(3 * 60 * 60 * 1000)
    await expectText('按本机时间，会话可能已到期。请检查当前会话；页面不会自动续期。')
    assert.deepEqual((await stats()).api_requests, beforeExpiry.api_requests, 'Local expiry reminder performs no background request')
    const plain = await context.newPage()
    let plainAPI = 0
    plain.on('request', request => { if (new URL(request.url()).pathname.startsWith('/api/')) plainAPI++ })
    await plain.goto(info.http_origin + '/login', { waitUntil: 'networkidle' })
    await plain.getByText('账号操作需要 HTTPS，请通过安全入口访问。', { exact: true }).waitFor()
    assert.equal(plainAPI, 0, 'HTTP account screen sends no credential or account request')
    assert(await plain.getByLabel('密码', { exact: true }).isDisabled(), 'HTTP credential fields cannot be used')
    await plain.close()
    passed('390px layout fits, local expiry reminder makes no request, HTTP blocks account transport before fetch')

    phase = 'final health and cleanup'
    assert.equal(errors.length, 0, 'No uncaught browser application errors')
    assert.equal(routeFailures.length, 0, 'Route callbacks completed without raw diagnostics')
    assert(diagnostics.every(item => item.expected), 'Every console warning/error is an expected HTTP refusal or injected redirect failure')
    report.expectedNetworkDiagnostics = diagnostics.length
    await page.getByRole('button', { name: '退出当前会话', exact: true }).click()
    await expectText('已退出当前会话。')
    report.success = true
    fs.writeFileSync(path.join(output, 'browser-qa.json'), JSON.stringify(report, null, 2) + '\n')
    console.log('Browser QA complete: ' + report.checks.length + ' groups; browser ' + report.browser)
  } catch (error) {
    report.success = false
    report.failedPhase = phase
    report.routeFailures = routeFailures.length
    if (page) {
      await page.screenshot({ path: path.join(output, 'failure.png'), fullPage: true }).catch(() => {})
      await page.locator('main').innerText().then(text => fs.writeFileSync(path.join(output, 'failure-dom.txt'), text)).catch(() => {})
    }
    fs.writeFileSync(path.join(output, 'browser-qa.json'), JSON.stringify(report, null, 2) + '\n')
    console.error('Browser QA failed at ' + phase + ' (' + error.name + ')')
    process.exitCode = 1
  } finally {
    // A failed browser assertion still needs normal fixture shutdown. The Go
    // lifecycle result is distinct from this driver's nonzero assertion result.
    if (context) await context.request.post(info.origin + '/__fixture/stop', { timeout: 5000 }).catch(() => {})
    if (browser) await browser.close()
  }
})()
