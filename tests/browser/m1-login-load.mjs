import assert from 'node:assert/strict'
import { readFile, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'
import { createRequire } from 'node:module'

const info = JSON.parse(await readFile(process.env.OJ_BROWSER_QA_INFO, 'utf8'))
const output = process.env.OJ_BROWSER_QA_OUTPUT
const { chromium } = createRequire(import.meta.url)(process.env.OJ_PLAYWRIGHT_MODULE)
const accounts = info.login_accounts
const checks = []
const samples = []
let browser

function passed(check) { checks.push(check) }

async function login(index, target) {
  let context, started
  const sample = { index, startLagMs: null, loginMs: null, statuses: [], outcome: 'client_error' }
  try {
    context = await browser.newContext({ ignoreHTTPSErrors: true })
    const page = await context.newPage()
    await page.route('**/*', route => new URL(route.request().url()).origin === info.origin ? route.continue() : route.abort())
    page.on('response', response => {
      const url = new URL(response.url())
      const method = response.request().method()
      if (url.origin === info.origin && (url.pathname === '/api/v1/session' || url.pathname.startsWith('/api/v1/session/login/'))) {
        sample.statuses.push([method, url.pathname, response.status()])
      }
    })
    await page.goto(info.origin + '/login', { timeout: 10_000 })
    const [email, subject] = accounts[index]
    await page.getByLabel('邮箱', { exact: true }).fill(email, { timeout: 5_000 })
    await page.getByLabel('密码', { exact: true }).fill('synthetic protocol password', { timeout: 5_000 })
    const submit = page.getByRole('button', { name: '登录', exact: true })
    // Wait for startup session discovery before measuring the click.
    await submit.waitFor({ state: 'visible', timeout: 5_000 })
    await delay(Math.max(0, target - performance.now()))
    started = performance.now()
    sample.startLagMs = Math.round(started - target)
    await submit.click({ timeout: 5_000 })
    await page.getByText('登录成功，当前会话已核验。', { exact: true }).waitFor({ timeout: 10_000 })
    sample.loginMs = Math.round(performance.now() - started)
    assert(sample.statuses.some(([method, url, status]) => method === 'POST' && url.endsWith('/begin') && status === 202))
    assert(sample.statuses.some(([method, url, status]) => method === 'POST' && url.endsWith('/complete') && status === 204))
    assert(sample.statuses.some(([method, url, status]) => method === 'GET' && url === '/api/v1/session' && status === 200))
    await page.getByRole('link', { name: '管理个人账号', exact: true }).click({ timeout: 5_000 })
    await page.getByText(subject, { exact: true }).waitFor({ timeout: 5_000 })
    assert((await context.cookies(info.origin)).some(cookie => cookie.name === '__Host-oj_session' && cookie.secure && cookie.httpOnly))
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
  browser = await chromium.launch(process.env.OJ_CHROMIUM_EXECUTABLE ? { executablePath: process.env.OJ_CHROMIUM_EXECUTABLE } : {})
  const start = performance.now() + 3000
  await Promise.all(accounts.map(async (_, index) => {
    const target = start + index * 600
    await delay(Math.max(0, target - 2000 - performance.now()))
    samples[index] = await login(index, target)
  }))
  assert.equal(samples.filter(s => s.startLagMs !== null && Math.abs(s.startLagMs) <= 500).length, 100)
  passed('100 distinct browser attempts started within 500 ms of the uniform schedule')
  const times = samples.map(s => s.outcome === 'verified' ? s.loginMs : Infinity).sort((a, b) => a - b)
  assert(percentile(times, 95) <= 3000 && times.filter(ms => ms <= 10_000).length >= 99)
  passed('browser login p95 at most 3 seconds and at least 99 verified within 10 seconds')
} finally {
  await browser?.close()
  const times = samples.map(s => s.outcome === 'verified' ? s.loginMs : Infinity).sort((a, b) => a - b)
  await writeFile(path.join(output, 'browser-qa.json'), JSON.stringify({
    success: checks.length === 2, checks, fixture: 'local TLS with disposable CA, PostgreSQL and Redis',
    attempted: samples.length, verified: samples.filter(s => s.outcome === 'verified').length,
    within10s: times.filter(ms => ms <= 10_000).length,
    p50Ms: percentile(times, 50), p95Ms: percentile(times, 95), p99Ms: percentile(times, 99), samples,
  }, null, 2) + '\n')
}
