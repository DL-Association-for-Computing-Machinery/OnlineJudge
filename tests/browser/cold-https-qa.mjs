import assert from 'node:assert/strict'
import fs from 'node:fs/promises'
import net from 'node:net'
import path from 'node:path'
import { createRequire } from 'node:module'

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
  const exchange = async (route, method, action) => {
    const [response] = await Promise.all([
      page.waitForResponse(r => r.url() === info.origin + route && r.request().method() === method, { timeout: 8000 }),
      action(),
    ])
    authorStatuses.push(response.status())
    assert.equal(response.status(), 200)
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

  phase = 'Firefox reference cancellation'
  const cancelled = await exchange(`${original}/cancel`, 'POST', () => controls.getByRole('button', { name: '取消本次申请', exact: true }).click())
  assert.equal(cancelled.id, request.id)
  assert.equal(cancelled.draft_id, draft.draft_id)
  assert.equal(cancelled.request_key, request.request_key)
  assert.equal(cancelled.state, 'cancelled')
  assert.deepEqual(await read(), cancelled)
  await controls.getByRole('status').filter({ hasText: '状态：已取消。' }).waitFor({ timeout: 8000 })
  const screenshot = 'firefox-reference-cancelled.png'
  await controls.screenshot({ path: path.join(output, screenshot) })
  report.screenshots.push(screenshot)
}

// The fixture is the sole direct origin. This local proxy never forwards any
// traffic; page events detect foreign resources without intercepting requests
// or changing the browser's native Origin and Fetch Metadata behavior.
const deny = net.createServer(socket => socket.destroy())
await new Promise((resolve, reject) => { deny.once('error', reject); deny.listen(0, '127.0.0.1', resolve) })
const proxy = { server: `http://127.0.0.1:${deny.address().port}`, bypass: '127.0.0.1' }
const policy = path.join(output, 'firefox-policy.json')
await fs.writeFile(policy, JSON.stringify({ policies: { Certificates: {
  ImportEnterpriseRoots: false, Install: [process.env.OJ_BROWSER_CA_FILE],
} } }), { flag: 'wx' })

function launch(trusted) {
  const env = { ...process.env }
  delete env.PLAYWRIGHT_FIREFOX_POLICIES_JSON
  if (trusted) env.PLAYWRIGHT_FIREFOX_POLICIES_JSON = policy
  return firefox.launch({ executablePath: process.env.OJ_FIREFOX_EXECUTABLE, headless: true, proxy,
    env, firefoxUserPrefs: { 'security.enterprise_roots.enabled': false } })
}

let browser
let phase = 'untrusted issuer control'
try {
  browser = await launch(false)
  const control = await browser.newContext({ ignoreHTTPSErrors: false })
  const negative = await control.newPage()
  await assert.rejects(negative.goto(info.origin))
  const code = await negative.locator('#errorCode').textContent({ timeout: 5000 })
  assert.match(code ?? '', /^(SEC_ERROR_UNKNOWN_ISSUER|MOZILLA_PKIX_ERROR_SELF_SIGNED_CERT)$/)
  await browser.close()
  browser = undefined
  passed('fresh browser rejects the untrusted issuer')

  phase = 'launch trusted browser'
  browser = await launch(true)
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
  await new Promise(resolve => deny.close(resolve))
  await fs.writeFile(path.join(output, 'browser-qa.json'), JSON.stringify(report))
}
