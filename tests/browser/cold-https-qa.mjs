import assert from 'node:assert/strict'
import fs from 'node:fs/promises'
import path from 'node:path'
import { createRequire } from 'node:module'
import { setTimeout as delay } from 'node:timers/promises'
import { firefoxTrust } from './firefox-trust.mjs'

const require = createRequire(import.meta.url)
const { firefox } = require(process.env.OJ_PLAYWRIGHT_MODULE)
const output = process.env.OJ_BROWSER_QA_OUTPUT
const info = JSON.parse(await fs.readFile(process.env.OJ_BROWSER_QA_INFO, 'utf8'))
const origin = new URL(info.origin)
assert(origin.protocol === 'https:' && origin.hostname === '127.0.0.1' && origin.port && origin.origin === info.origin)
assert(/^[a-zA-Z0-9._+-]+@example\.invalid$/.test(info.email))
const report = { success: false, checks: [], screenshots: [] }
const loginStatuses = []
const authorStatuses = []
function passed(name) { report.checks.push(name); console.log('PASS ' + name) }

async function verifyAuthorWrites(page) {
  const base = '/api/v1/problem-draft-management'
  const imports = `${base}/imports`
  const references = `${base}/reference-requests`
  const referenceWrites = []
  page.on('request', request => {
    const url = new URL(request.url())
    if (url.origin === info.origin && url.pathname.startsWith(references) && request.method() === 'POST') {
      referenceWrites.push(url.pathname)
    }
  })
  const exchange = async (route, method, action, expected = 200) => {
    const [response] = await Promise.all([
      page.waitForResponse(r => r.url() === info.origin + route && r.request().method() === method, { timeout: 8000 }),
      action(),
    ])
    authorStatuses.push(response.status())
    assert.equal(response.status(), expected)
    return response.json()
  }

  phase = 'Firefox draft import'
  await page.goto(info.origin + '/author/drafts')
  await page.getByRole('button', { name: '检查会话并加载来源与记录', exact: true }).click()
  const draft = await exchange(imports, 'POST', () => page.getByRole('button', { name: '导入此修订', exact: true }).click())
  assert.equal(draft.state, 'private-draft')
  assert.equal(draft.validation_state, 'not-run')
  assert.match(draft.draft_id, /^[a-f0-9]{32}$/)
  await page.getByRole('link', { name: '打开草稿预览', exact: true }).click()
  await page.getByRole('button', { name: '检查会话并打开草稿', exact: true }).click()
  await page.getByRole('heading', { name: 'Browser statement', exact: true }).waitFor({ timeout: 8000 })

  phase = 'Firefox reference request'
  const controls = page.getByRole('region', { name: '参考验证申请', exact: true })
  const request = await exchange(references, 'POST', () => controls.getByRole('button', { name: '提交或重试同一申请', exact: true }).click())
  assert.equal(request.draft_id, draft.draft_id)
  assert.equal(request.state, 'queued')
  assert.match(request.request_key, /^[A-Za-z0-9_-]{16,128}$/)
  const original = `${references}/${request.request_key}`
  const read = () => exchange(original, 'GET', () => controls.getByRole('button', { name: '查看申请回执', exact: true }).click())
  assert.deepEqual(await read(), request)

  phase = 'Firefox reference preparation'
  const readPlan = () => exchange(`${original}/plan`, 'GET', () => controls.getByRole('button', { name: '查看材料状态', exact: true }).click())
  const prepare = `${original}/prepare`
  assert.deepEqual(await readPlan(), { ready: false })
  assert.deepEqual(await exchange(prepare, 'POST', () => controls.getByRole('button', { name: '准备参考材料', exact: true }).click(), 202), { ready: false })
  await controls.getByText('已受理准备请求，请稍后查看材料状态。 状态：待完成。', { exact: true }).waitFor({ timeout: 8000 })

  phase = 'Firefox reference plan becomes ready'
  const ready = controls.getByText('材料计划已准备。 状态：待完成。', { exact: true })
  const deadline = performance.now() + 8000
  let plan = await readPlan()
  while (plan?.ready === false && performance.now() < deadline) {
    assert.deepEqual(plan, { ready: false })
    await delay(150)
    plan = await readPlan()
  }
  assert.deepEqual(plan, { ready: true })
  await ready.waitFor({ timeout: 8000 })

  phase = 'Firefox reference plan survives reload'
  await page.reload()
  await page.getByRole('button', { name: '检查会话并打开草稿', exact: true }).click()
  await page.getByRole('heading', { name: 'Browser statement', exact: true }).waitFor({ timeout: 8000 })
  assert.deepEqual(await read(), request)
  assert.deepEqual(await readPlan(), { ready: true })
  await ready.waitFor({ timeout: 8000 })
  assert.deepEqual(referenceWrites, [references, prepare])
  const preparedScreenshot = 'firefox-reference-prepared.png'
  await controls.screenshot({ path: path.join(output, preparedScreenshot) })
  report.screenshots.push(preparedScreenshot)

  phase = 'Firefox reference cancellation'
  const cancel = `${original}/cancel`
  const cancelled = await exchange(cancel, 'POST', () => controls.getByRole('button', { name: '取消本次申请', exact: true }).click())
  assert.equal(cancelled.id, request.id)
  assert.equal(cancelled.draft_id, draft.draft_id)
  assert.equal(cancelled.request_key, request.request_key)
  assert.equal(cancelled.state, 'cancelled')
  assert.deepEqual(await read(), cancelled)
  await controls.getByRole('status').filter({ hasText: '状态：已取消。' }).waitFor({ timeout: 8000 })
  assert.deepEqual(referenceWrites, [references, prepare, cancel])
  const screenshot = 'firefox-reference-cancelled.png'
  await controls.screenshot({ path: path.join(output, screenshot) })
  report.screenshots.push(screenshot)
}

const trust = await firefoxTrust(firefox, output)

let browser
let phase = 'untrusted issuer control'
try {
  browser = await trust.launch(false)
  const control = await browser.newContext({ ignoreHTTPSErrors: false })
  const negative = await control.newPage()
  // Firefox can reject navigation without exposing its internal error page.
  // The fixture uses a CA-signed leaf. Require unknown issuer specifically.
  await assert.rejects(negative.goto(info.origin), { message: /^page\.goto: SEC_ERROR_UNKNOWN_ISSUER(?:\r?\n|$)/ })
  await browser.close()
  browser = undefined
  passed('fresh browser rejects the untrusted issuer')

  phase = 'launch trusted browser'
  browser = await trust.launch(true)
  for (const [name, viewport] of [['desktop', { width: 1280, height: 960 }], ['mobile', { width: 390, height: 844 }]]) {
    phase = `${name} cold load`
    const context = await browser.newContext({ viewport, ignoreHTTPSErrors: false, serviceWorkers: 'block' })
    const foreign = [], errors = [], assets = new Set()
    const page = await context.newPage()
    page.on('request', request => {
      if (new URL(request.url()).origin !== info.origin) foreign.push(request.resourceType())
    })
    page.on('pageerror', error => errors.push(error.name))
    page.on('console', message => { if (message.type() === 'error') errors.push(message.type()) })
    page.on('requestfailed', request => errors.push(request.resourceType()))
    page.on('response', response => {
      const url = new URL(response.url())
      if (url.pathname.startsWith('/assets/') && response.ok()) assets.add(response.request().resourceType())
      if (url.origin === info.origin && ['/api/v1/session/login/begin', '/api/v1/session/login/complete', '/api/v1/session'].includes(url.pathname)) {
        loginStatuses.push(response.status())
      }
    })
    const response = await page.goto(info.origin)
    assert(response?.ok() && await page.evaluate(() => isSecureContext))
    assert.equal(page.url(), info.origin + '/')
    assert((await page.title()).includes('OnlineJudge'))
    await page.getByRole('heading', { name: '题库', exact: true }).waitFor()
    assert((await page.locator('main').innerText()).includes('A+B Problem'))
    assert.equal(await page.locator('vite-error-overlay').count(), 0)
    assert(assets.has('script') && assets.has('stylesheet'), 'Cold page loaded local JS and CSS')
    await page.getByRole('link', { name: '登录', exact: true }).click()
    await page.getByLabel('密码', { exact: true }).waitFor()
    assert.equal(page.url(), info.origin + '/login')
    const screenshot = `${name}-cold-https.png`
    await page.screenshot({ path: path.join(output, screenshot) })
    report.screenshots.push(screenshot)
    if (name === 'desktop') {
      phase = 'strict login'
      await page.getByLabel('邮箱', { exact: true }).fill(info.email)
      await page.getByLabel('密码', { exact: true }).fill('synthetic protocol password')
      await page.getByRole('button', { name: '登录', exact: true }).click()
      await page.getByText('登录成功，当前会话已核验。', { exact: true }).waitFor({ timeout: 8000 })
      phase = 'open account and refresh'
      await page.getByRole('link', { name: '管理个人账号', exact: true }).click()
      await page.waitForURL(info.origin + '/account')
      await page.reload()
      await page.getByText(info.subject_id, { exact: true }).waitFor({ timeout: 8000 })
      assert((await context.cookies(info.origin)).some(cookie => cookie.name === '__Host-oj_session' && cookie.secure && cookie.httpOnly))
      passed('strict HTTPS login survives reload with a real secure session')
      await verifyAuthorWrites(page)
      passed('strict Firefox prepares reference materials and reads readiness after reload without another write')
      passed('strict Firefox imports a private draft and creates reads and cancels its reference request')
    }
    assert.deepEqual(foreign, [])
    assert.deepEqual(errors, [])
    await context.close()
  }
  passed('cold desktop and mobile render local assets with no foreign page-resource requests')
  report.success = true
} catch {
  console.error(`FAIL ${phase}; session HTTP statuses: ${loginStatuses.join(',')}; author HTTP statuses: ${authorStatuses.join(',')}`)
  process.exitCode = 1
} finally {
  if (browser) await browser.close()
  await trust.close()
  await fs.writeFile(path.join(output, 'browser-qa.json'), JSON.stringify(report))
}
