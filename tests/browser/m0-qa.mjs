import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import process from 'node:process'
import console from 'node:console'
import { createRequire } from 'node:module'

const require = createRequire(import.meta.url)
const { chromium } = require(process.env.OJ_PLAYWRIGHT_MODULE)
const output = process.env.OJ_BROWSER_OUTPUT
const origin = process.env.OJ_BROWSER_ORIGIN
const address = new URL(origin)
assert(address.protocol === 'http:' && address.hostname === '127.0.0.1' && address.port && address.origin === origin)
const operationStorageName = 'oj-m0-fixed-operation-v1'
const result = { success: false, checks: [], operations: [], screenshots: [], viewports: ['1280x960', '390x844'] }
let browser, context, page
let phase = 'launch'
let postCount = 0
const errors = [], credentials = [], destinations = [], routeErrors = [], resourceDiagnostics = []
const passed = name => { result.checks.push(name); console.log('PASS ' + name) }
const status = () => page.getByRole('status')
const text = async value => status().filter({ hasText: value }).waitFor({ timeout: 10000 })
const operation = () => page.evaluate(key => JSON.parse(sessionStorage.getItem(key)), operationStorageName)
const screenshot = async name => {
  const filename = path.join(output, name + '.png')
  await page.screenshot({ path: filename, fullPage: true })
  result.screenshots.push(filename)
}
function safeRoute(fn) {
  return async route => {
    try { await fn(route) }
    catch { routeErrors.push('ROUTE_FAILED'); await route.abort().catch(() => {}) }
  }
}
async function freshPage(viewport = { width: 1280, height: 960 }) {
  const ctx = await browser.newContext({ viewport })
  const p = await ctx.newPage()
  p.setDefaultTimeout(6000)
  p.on('pageerror', error => errors.push(error.name))
  p.on('console', message => {
    if (!['error', 'warning'].includes(message.type())) return
    if (message.text().startsWith('Failed to load resource:')) {
      const url = message.location().url
      resourceDiagnostics.push({ phase, path: url ? new URL(url).pathname : 'unknown' })
    } else errors.push('CONSOLE_' + message.type())
  })
  p.on('request', request => {
    const url = new URL(request.url())
    if (url.origin !== origin || url.search) destinations.push('UNEXPECTED_DESTINATION')
    if (request.headers().authorization || request.headers().cookie) credentials.push('CREDENTIAL_HEADER')
    if (request.method() === 'POST') postCount++
  })
  await ctx.route('**/*', route => new URL(route.request().url()).origin === origin ? route.continue() : route.abort())
  return { ctx, p }
}

process.once('SIGTERM', () => { void browser?.close().finally(() => process.exit(1)) })
try {
  browser = await chromium.launch({ headless: true, ...(process.env.OJ_BROWSER_CHROMIUM ? { executablePath: process.env.OJ_BROWSER_CHROMIUM } : {}) })
  result.browser = browser.version()
  result.node = process.version
  result.playwright = require(path.join(path.dirname(require.resolve(process.env.OJ_PLAYWRIGHT_MODULE)), 'package.json')).version
  ;({ ctx: context, p: page } = await freshPage())
  await page.goto(origin + '/m0', { waitUntil: 'domcontentloaded' })
  await text('尚未提交')
  assert.equal(await page.title(), 'M0 判题实验 · ECUT OJ')
  await page.getByRole('heading', { name: '提交到结果', exact: true }).waitFor()
  assert((await page.locator('#source').innerText()).includes('a + b'))
  assert.equal(await page.locator('[data-vite-dev-id],vite-error-overlay').count(), 0)
  await page.getByRole('button', { name: '提交示例', exact: true }).focus()
  assert.equal(await page.getByRole('button', { name: '提交示例', exact: true }).evaluate(node => getComputedStyle(node).outlineStyle), 'solid')
  passed('page identity, fixed source and keyboard focus')

  phase = 'computed visual styles'
  const styles = await page.evaluate(() => {
    const css = selector => getComputedStyle(document.querySelector(selector))
    return { body: parseFloat(css('body').fontSize), h1: parseFloat(css('h1').fontSize), h2: parseFloat(css('h2').fontSize),
      background: css('html').backgroundColor, foreground: css('body').color, muted: css('.intro').color,
      button: css('#submit').backgroundColor, buttonText: css('#submit').color, buttonHeight: document.querySelector('#submit').getBoundingClientRect().height }
  })
  const luminance = color => {
    const channels = color.match(/\d+(?:\.\d+)?/g).slice(0, 3).map(Number).map(n => n / 255)
    const linear = channels.map(n => n <= 0.04045 ? n / 12.92 : ((n + 0.055) / 1.055) ** 2.4)
    return linear[0] * 0.2126 + linear[1] * 0.7152 + linear[2] * 0.0722
  }
  const contrast = (a, b) => (Math.max(luminance(a), luminance(b)) + 0.05) / (Math.min(luminance(a), luminance(b)) + 0.05)
  assert(styles.h1 / styles.h2 >= 1.25 && styles.h2 / styles.body >= 1.25)
  assert(contrast(styles.foreground, styles.background) >= 4.5 && contrast(styles.muted, styles.background) >= 4.5)
  assert(contrast(styles.buttonText, styles.button) >= 4.5 && styles.buttonHeight >= 44)
  result.styles = styles
  passed('computed typography, text/control contrast and touch target')

  phase = 'server boundaries'
  const bootstrap = await (await context.request.get(origin + '/m0/api/bootstrap')).json()
  const headers = { 'X-OJ-M0-Nonce': bootstrap.nonce, 'Idempotency-Key': 'm0-browser-negative-key', Origin: origin }
  assert.equal((await context.request.get(origin + '/m0/api/receipt', { headers: { ...headers, 'Idempotency-Key': process.env.OJ_BROWSER_OTHER_KEY } })).status(), 404)
  assert.equal((await context.request.get(origin + '/m0/api/receipt')).status(), 403)
  assert.equal((await context.request.post(origin + '/m0/api/submissions', { headers: { ...headers, Origin: 'https://example.invalid' }, data: { preset: 'sum' } })).status(), 403)
  assert.equal((await context.request.post(origin + '/m0/api/submissions', { headers, data: { preset: 'sum', source: 'arbitrary' } })).status(), 400)
  assert.equal((await context.request.post(origin + '/m0/api/submissions', { headers: { ...headers, Host: 'example.invalid' }, data: { preset: 'sum' } })).status(), 403)
  passed('real API rejects forged source, Origin, Host and foreign scope')

  phase = 'actual lost acceptance response'
  let accepted
  const lose = safeRoute(async route => {
    const response = await route.fetch()
    assert.equal(response.status(), 202)
    accepted = await response.json()
    await route.abort('failed') // The actual POST committed; no fabricated success reaches the page.
  })
  await page.route('**/m0/api/submissions', lose)
  await page.getByRole('button', { name: '提交示例', exact: true }).click()
  await text('接收状态未知')
  await page.unroute('**/m0/api/submissions', lose)
  const pending = await operation()
  assert.equal(pending.preset, 'sum')
  assert.equal(pending.binding, 'unverified')
  assert.equal(postCount, 1)
  await screenshot('desktop-unknown')
  await page.reload({ waitUntil: 'domcontentloaded' })
  await text('尚未核验是否对应所选示例')
  assert.equal((await operation()).key, pending.key)
  assert.equal(postCount, 1, 'Refresh must only query, never automatically POST')
  await page.getByRole('button', { name: '停止查询', exact: true }).click()
  await text('自动查询已停止')
  await page.getByRole('button', { name: '原样重试', exact: true }).click()
  await text('已接收，正在排队')
  assert.equal((await operation()).key, pending.key)
  assert.equal((await operation()).binding, 'confirmed')
  assert.equal(await page.locator('#submission-id').innerText(), accepted.id)
  result.operations.push({ id: accepted.id, key: pending.key, preset: 'sum' })
  await screenshot('desktop-queued')
  const marker = path.join(output, 'queued.json')
  fs.writeFileSync(marker + '.tmp', JSON.stringify({ id: accepted.id, key: pending.key }), { flag: 'wx', mode: 0o600 })
  assert(!fs.existsSync(marker), 'Queued marker must publish exactly once')
  fs.renameSync(marker + '.tmp', marker) // Readers cannot observe a partially written marker.
  passed('real lost POST response, refresh lookup and immutable-key retry')

  phase = 'AC terminal'
  await text('判题完成：AC')
  assert.equal(await page.locator('#verdict').innerText(), 'AC')
  assert.equal(await page.getByRole('button', { name: '停止查询', exact: true }).isHidden(), true)
  await screenshot('desktop-AC')
  passed('real Worker AC displayed in browser')

  phase = 'terminal recheck'
  let reads = 0
  const observe = request => { if (new URL(request.url()).pathname === '/m0/api/receipt') reads++ }
  page.on('request', observe)
  await page.getByRole('button', { name: '核验原键回执', exact: true }).click()
  await text('判题完成：AC')
  assert.equal(reads, 1, 'Explicit terminal recheck must send a real GET')
  page.off('request', observe)
  passed('terminal recheck sends one GET and settles')

  phase = 'WA narrow screen'
  await page.getByRole('button', { name: '开始下一次实验', exact: true }).click()
  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByLabel('示例代码', { exact: true }).selectOption('wrong')
  assert((await page.locator('#source').innerText()).includes('wrong'))
  await page.getByRole('button', { name: '提交示例', exact: true }).click()
  await text('判题完成：WA')
  const second = await operation()
  result.operations.push({ id: await page.locator('#submission-id').innerText(), key: second.key, preset: 'wrong' })
  assert.notEqual(second.key, pending.key)
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'No horizontal clipping at 390px')
  await screenshot('mobile-WA')
  passed('real Worker WA and narrow viewport')

  phase = 'conflict survives receipt and refresh'
  await page.evaluate(({ key, value }) => sessionStorage.setItem(key, JSON.stringify(value)), { key: operationStorageName, value: { ...second, preset: 'sum', binding: 'unverified' } })
  await page.reload({ waitUntil: 'domcontentloaded' })
  await text('尚未核验是否对应所选示例')
  await page.getByRole('button', { name: '原样重试', exact: true }).click()
  await text('原幂等键与已有内容冲突')
  assert.equal((await operation()).binding, 'conflict')
  await page.getByRole('button', { name: '核验原键回执', exact: true }).click()
  await text('不能归为所选示例的结果')
  await page.reload({ waitUntil: 'domcontentloaded' })
  await text('不能归为所选示例的结果')
  assert.equal((await operation()).key, second.key)
  await screenshot('mobile-conflict')
  passed('409 remains distinct from the existing key verdict after reload')

  phase = 'history navigation recovery'
  await page.goto(origin + '/m0/assets/page.css', { waitUntil: 'domcontentloaded' })
  await page.goBack({ waitUntil: 'domcontentloaded' })
  await text('不能归为所选示例的结果')
  await page.getByRole('button', { name: '核验原键回执', exact: true }).click()
  await text('不能归为所选示例的结果')
  passed('back navigation reboots nonce and can query the original key')

  phase = 'not found and unavailable'
  const oldContext = context
  ;({ ctx: context, p: page } = await freshPage({ width: 390, height: 844 }))
  await page.goto(origin + '/m0')
  await text('尚未提交')
  const unknown = { key: 'm0-browser-not-found-key', preset: 'sum', version: 'm0-sum-v3', binding: 'unverified' }
  await page.evaluate(({ key, value }) => sessionStorage.setItem(key, JSON.stringify(value)), { key: operationStorageName, value: unknown })
  const postsBefore = postCount
  await page.reload()
  await text('暂未查到原键回执，这不表示提交已回滚')
  const unavailable = safeRoute(route => route.fulfill({ status: 503, contentType: 'application/problem+json', body: '{"status":503,"code":"ACCEPTANCE_UNKNOWN"}' }))
  await page.route('**/m0/api/receipt', unavailable)
  await page.getByRole('button', { name: '核验原键回执', exact: true }).click()
  await text('查询暂时失败，未改变判题结果')
  assert.equal(await page.locator('#verdict').innerText(), '尚无判定')
  assert.equal((await operation()).key, unknown.key)
  assert.equal(postCount, postsBefore)
  await screenshot('mobile-query-unavailable')
  await page.unroute('**/m0/api/receipt', unavailable)
  await page.getByRole('button', { name: '核验原键回执', exact: true }).click()
  await text('暂未查到原键回执，这不表示提交已回滚')
  passed('404 and injected 503 retain original operation without a false verdict')
  await context.close()
  context = oldContext

  phase = 'storage write failure'
  const isolated = await freshPage()
  const isolatedPage = isolated.p
  await isolatedPage.goto(origin + '/m0')
  await isolatedPage.getByRole('status').filter({ hasText: '尚未提交' }).waitFor()
  await isolatedPage.evaluate(() => { Storage.prototype.setItem = () => { throw new Error('synthetic storage failure') } })
  const beforeStorageFailure = postCount
  await isolatedPage.getByRole('button', { name: '提交示例', exact: true }).click()
  await isolatedPage.getByRole('status').filter({ hasText: '原操作尚未发送' }).waitFor()
  assert.equal(postCount, beforeStorageFailure)
  await isolated.ctx.close()
  passed('failed operation persistence prevents POST')

  phase = 'privacy and console health'
  page = context.pages()[0]
  assert.equal(await page.evaluate(() => document.cookie), '')
  const saved = await operation()
  assert.deepEqual(Object.keys(saved).sort(), ['binding', 'key', 'preset', 'version'])
  assert.equal((await page.content()).includes(bootstrap.nonce), false)
  assert.equal((await context.cookies()).length, 0)
  assert.deepEqual(credentials, [])
  assert.deepEqual(destinations, [])
  assert.deepEqual(routeErrors, [])
  assert.deepEqual(errors, [])
  const expectedPhases = ['actual lost acceptance response', 'conflict survives receipt and refresh', 'not found and unavailable', 'history navigation recovery']
  assert(resourceDiagnostics.every(d => ['/m0/api/submissions', '/m0/api/receipt'].includes(d.path) && expectedPhases.includes(d.phase)), 'Every resource error must belong to an explicit failure test')
  result.expected_resource_diagnostics = resourceDiagnostics
  passed('no Bearer, cookies, external destinations, runtime errors or unexpected console errors')
  result.success = true
} catch (error) {
  result.failure = phase
  result.error_kind = error.name
  console.error('FAIL M0_BROWSER ' + phase + ' (' + error.name + ')')
  if (page && !page.isClosed()) {
    await page.screenshot({ path: path.join(output, 'failure.png'), fullPage: true }).catch(() => {})
    fs.writeFileSync(path.join(output, 'failure.txt'), await page.locator('body').innerText().catch(() => 'unavailable'))
  }
  process.exitCode = 1
} finally {
  await browser?.close()
  fs.writeFileSync(path.join(output, 'result.json'), JSON.stringify(result, null, 2), { mode: 0o600 })
}
