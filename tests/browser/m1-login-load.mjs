import assert from 'node:assert/strict'
import { readFile, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'
import { createRequire } from 'node:module'
import net from 'node:net'

const info = JSON.parse(await readFile(process.env.OJ_BROWSER_QA_INFO, 'utf8'))
const output = process.env.OJ_BROWSER_QA_OUTPUT
const { chromium, firefox } = createRequire(import.meta.url)(process.env.OJ_PLAYWRIGHT_MODULE)
const strict = Boolean(process.env.OJ_FIREFOX_EXECUTABLE)
const accounts = info.login_accounts
const checks = []
const samples = []
const browsers = []
let controlBrowser, deny

function passed(check) { checks.push(check) }

async function login(browser, browserIndex, index, target) {
  let context, started, clicked
  const sample = { index, startLagMs: null, loginMs: null, statuses: [], outcome: 'client_error' }
  if (info.trial_nginx) sample.loginPhases = []
  if (strict) Object.assign(sample, { browserIndex, dispatchLagMs: null, beginRequestLagMs: null })
  try {
    context = await browser.newContext({ ignoreHTTPSErrors: !strict, serviceWorkers: 'block' })
    const page = await context.newPage()
    let foreign = false
    if (strict) {
      page.on('request', request => { if (new URL(request.url()).origin !== info.origin) foreign = true })
    } else {
      await page.route('**/*', route => new URL(route.request().url()).origin === info.origin ? route.continue() : route.abort())
    }
    const observedRequests = new WeakMap()
    if (info.trial_nginx) page.on('request', request => {
      if (started === undefined) return
      const now = performance.now()
      observedRequests.set(request, Math.round(now - started))
      if (strict && sample.beginRequestLagMs === null && request.method() === 'POST' &&
          request.url() === info.origin + '/api/v1/session/login/begin') {
        sample.beginRequestLagMs = Math.round(now - target)
      }
    })
    page.on('response', response => {
      const url = new URL(response.url())
      const request = response.request()
      const method = request.method()
      if (url.origin === info.origin && (url.pathname === '/api/v1/session' || url.pathname.startsWith('/api/v1/session/login/'))) {
        const status = [method, url.pathname, response.status()]
        if (info.trial_nginx) {
          const upstream = response.headers()['x-oj-fixture-upstream-status']
          status.push(/^[1-5]\d\d$/.test(upstream ?? '') ? Number(upstream) : null)
          const time = response.headers()['x-oj-fixture-upstream-time']
          const upstreamMs = /^[0-9]+\.[0-9]{3}$/.test(time ?? '') ? Math.round(Number(time) * 1000) : null
          const requestMs = observedRequests.get(request)
          if (requestMs !== undefined) sample.loginPhases.push({
            method, path: url.pathname, status: response.status(),
            requestMs, responseMs: Math.round(performance.now() - started),
            upstreamMs: status[3] !== null && Number.isSafeInteger(upstreamMs) ? upstreamMs : null,
          })
        }
        sample.statuses.push(status)
      }
    })
    await page.goto(info.origin + '/login', { timeout: 10_000 })
    const [email, subject] = accounts[index]
    await page.getByLabel('邮箱', { exact: true }).fill(email, { timeout: 5_000 })
    await page.getByLabel('密码', { exact: true }).fill('synthetic protocol password', { timeout: 5_000 })
    const submit = page.getByRole('button', { name: '登录', exact: true })
    let point
    if (strict) {
      // Keep locator actionability checks outside the measured interaction.
      await submit.click({ trial: true, timeout: 5_000 })
      const box = await submit.boundingBox({ timeout: 5_000 })
      assert(box)
      point = [box.x + box.width / 2, box.y + box.height / 2]
      page.on('console', message => {
        if (clicked !== undefined || started === undefined || message.type() !== 'debug') return
        const match = /^OJ_FIXTURE_TRUSTED_CLICK ([0-9]+(?:\.[0-9]+)?)$/.exec(message.text())
        if (!match) return
        const time = Number(match[1]) - performance.timeOrigin
        // Firefox may quantize its clock; reject values outside the interaction.
        if (time < started - 2 || time > performance.now() + 2) return
        clicked = Math.max(started, time)
        sample.startLagMs = Math.round(clicked - target)
      })
      await submit.evaluate(button => {
        button.addEventListener('click', event => {
          if (event.isTrusted) console.debug('OJ_FIXTURE_TRUSTED_CLICK', performance.timeOrigin + performance.now())
        }, { capture: true, once: true })
      })
    } else {
      await submit.waitFor({ state: 'visible', timeout: 5_000 })
    }
    await delay(Math.max(0, target - performance.now()))
    started = performance.now()
    if (strict) {
      sample.dispatchLagMs = Math.round(started - target)
      await page.mouse.click(...point)
    } else {
      sample.startLagMs = Math.round(started - target)
      await submit.click({ timeout: 5_000 })
    }
    await page.getByText('登录成功，当前会话已核验。', { exact: true }).waitFor({ timeout: 10_000 })
    const confirmed = performance.now()
    if (strict) assert(clicked !== undefined && clicked <= confirmed)
    sample.loginMs = Math.round(confirmed - (strict ? clicked : started))
    assert(sample.statuses.some(([method, url, status]) => method === 'POST' && url.endsWith('/begin') && status === 202))
    assert(sample.statuses.some(([method, url, status]) => method === 'POST' && url.endsWith('/complete') && status === 204))
    assert(sample.statuses.some(([method, url, status]) => method === 'GET' && url === '/api/v1/session' && status === 200))
    await page.getByRole('link', { name: '管理个人账号', exact: true }).click({ timeout: 5_000 })
    await page.getByText(subject, { exact: true }).waitFor({ timeout: 5_000 })
    assert((await context.cookies(info.origin)).some(cookie => cookie.name === '__Host-oj_session' && cookie.secure && cookie.httpOnly))
    assert(!foreign)
    sample.outcome = 'verified'
  } catch {
    sample.outcome = sample.statuses.some(([, , status]) => status >= 500) ? 'server_error'
      : sample.statuses.some(([, , status]) => status === 429) ? 'limited' : 'unverified'
  } finally {
    await context?.close()
  }
  return sample
}

function percentile(sorted, n) {
  return sorted[Math.ceil(sorted.length * n / 100) - 1] ?? null
}

try {
  assert.match(info.origin, /^https:\/\/127\.0\.0\.1:[1-9]\d{0,4}$/)
  assert.equal(accounts.length, 100)
  for (const [email, subject] of accounts) {
    assert.match(email, /^[a-zA-Z0-9._+-]+@example\.invalid$/)
    assert.match(subject, /^[a-f0-9]{32}$/)
  }
  assert.equal(new Set(accounts.map(([email]) => email.toLowerCase())).size, 100)
  assert.equal(new Set(accounts.map(([, subject]) => subject)).size, 100)
  if (strict) {
    assert(info.trial_nginx && process.env.OJ_BROWSER_CA_FILE)
    deny = net.createServer(socket => socket.destroy())
    await new Promise((resolve, reject) => { deny.once('error', reject); deny.listen(0, '127.0.0.1', resolve) })
    const proxy = { server: `http://127.0.0.1:${deny.address().port}`, bypass: '127.0.0.1' }
    const policy = path.join(output, 'firefox-trial-policy.json')
    await writeFile(policy, JSON.stringify({ policies: { Certificates: {
      ImportEnterpriseRoots: false, Install: [process.env.OJ_BROWSER_CA_FILE],
    } } }), { flag: 'wx' })
    const launch = trusted => {
      const env = { ...process.env }
      delete env.PLAYWRIGHT_FIREFOX_POLICIES_JSON
      if (trusted) env.PLAYWRIGHT_FIREFOX_POLICIES_JSON = policy
      return firefox.launch({ executablePath: process.env.OJ_FIREFOX_EXECUTABLE, headless: true, proxy,
        env, firefoxUserPrefs: { 'security.enterprise_roots.enabled': false } })
    }
    controlBrowser = await launch(false)
    const control = await controlBrowser.newContext({ ignoreHTTPSErrors: false })
    const page = await control.newPage()
    await assert.rejects(page.goto(info.origin), { message: /^page\.goto: SEC_ERROR_UNKNOWN_ISSUER(?:\r?\n|$)/ })
    await controlBrowser.close()
    controlBrowser = undefined
    checks.push('fresh Firefox rejects the untrusted trial issuer')
    // Separate Firefox command queues without changing the aggregate click rate.
    for (let index = 0; index < 4; index++) browsers.push(await launch(true))
  } else {
    browsers.push(await chromium.launch(process.env.OJ_CHROMIUM_EXECUTABLE ? { executablePath: process.env.OJ_CHROMIUM_EXECUTABLE } : {}))
  }
  // Firefox needs more time to prepare isolated pages before the measured clicks.
  const preparationMs = strict ? 4000 : 2000
  const start = performance.now() + preparationMs + 1000
  await Promise.all(accounts.map(async (_, index) => {
    const target = start + index * 600
    await delay(Math.max(0, target - preparationMs - performance.now()))
    const browserIndex = index % browsers.length
    samples[index] = await login(browsers[browserIndex], browserIndex, index, target)
  }))
  assert.equal(samples.filter(s => s.startLagMs !== null && Math.abs(s.startLagMs) <= 500).length, 100)
  passed('100 distinct browser attempts started within 500 ms of the uniform schedule')
  const times = samples.map(s => s.outcome === 'verified' ? s.loginMs : Infinity).sort((a, b) => a - b)
  assert(percentile(times, 95) <= 3000 && times.filter(ms => ms <= 10_000).length >= 99)
  passed('browser login p95 at most 3 seconds and at least 99 verified within 10 seconds')
} finally {
  const closed = await Promise.allSettled([...browsers, ...(controlBrowser ? [controlBrowser] : [])].map(browser => browser.close()))
  if (deny?.listening) await new Promise(resolve => deny.close(resolve))
  const times = samples.map(s => s.outcome === 'verified' ? s.loginMs : Infinity).sort((a, b) => a - b)
  await writeFile(path.join(output, 'browser-qa.json'), JSON.stringify({
    success: checks.length === (strict ? 3 : 2) && closed.every(result => result.status === 'fulfilled'),
    checks, fixture: 'local TLS with disposable CA, PostgreSQL and Redis',
    browserTLS: strict ? 'strict-firefox' : 'loopback-exception',
    clickClock: strict ? 'trusted-page-event' : 'driver-dispatch',
    browserProcesses: browsers.length,
    edge: info.trial_nginx ? 'trial-nginx' : 'go-fixture',
    attempted: samples.length, verified: samples.filter(s => s.outcome === 'verified').length,
    within10s: times.filter(ms => ms <= 10_000).length,
    p50Ms: percentile(times, 50), p95Ms: percentile(times, 95), p99Ms: percentile(times, 99), samples,
  }, null, 2) + '\n')
}
