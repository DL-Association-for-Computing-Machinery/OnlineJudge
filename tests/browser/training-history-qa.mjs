import assert from 'node:assert/strict'
import fs from 'node:fs/promises'
import path from 'node:path'
import { createRequire } from 'node:module'

const require = createRequire(import.meta.url)
const { chromium } = require(process.env.OJ_PLAYWRIGHT_MODULE)
const output = process.env.OJ_BROWSER_QA_OUTPUT
const info = JSON.parse(await fs.readFile(process.env.OJ_BROWSER_QA_INFO, 'utf8'))
const ready = JSON.parse((await fs.readFile(info.log, 'utf8')).match(/^BROWSER_READY (.+)$/m)?.[1] ?? 'null')
const origin = new URL(info.origin)
assert(origin.protocol === 'https:' && origin.hostname === '127.0.0.1' && origin.port && origin.origin === info.origin)
assert.equal(ready?.url, info.origin)
for (const email of [ready.email, ready.other_email, ready.unverified_email]) assert(/^[a-zA-Z0-9._+-]+@example\.invalid$/.test(email))
for (const id of [ready.subject_id, ready.other_subject_id]) assert(/^[a-f0-9]{32}$/.test(id))
const report = { success: false, checks: [], screenshots: [] }
const endpoint = info.origin + '/api/v1/training/submissions'
const rowID = n => String(n).padStart(32, '0')
let step = 'launch'
function passed(name) { report.checks.push(name); console.log('PASS ' + name) }

async function signIn(page, email) {
  await page.goto(info.origin + '/login')
  await page.getByLabel('邮箱', { exact: true }).fill(email)
  await page.getByLabel('密码', { exact: true }).fill('synthetic protocol password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await page.getByText('登录成功，当前会话已核验。', { exact: true }).waitFor()
  await page.getByRole('link', { name: '前往题库' }).click()
  await page.getByRole('link', { name: '我的提交', exact: true }).click()
  await page.waitForURL(info.origin + '/submissions')
  await page.getByRole('heading', { name: '我的提交', exact: true }).waitFor()
  assert.match(await page.title(), /OnlineJudge/)
}

async function openHistory(page, name, expectedStatus) {
  const [response] = await Promise.all([
    page.waitForResponse(response => response.url().split('?')[0] === endpoint),
    page.getByRole('button', { name, exact: true }).click(),
  ])
  assert.equal(response.status(), expectedStatus)
  return response.url()
}

async function rows(page, expected) {
  const list = page.getByRole('list', { name: '训练提交记录' })
  await list.waitFor()
  assert.deepEqual(await list.locator('h2 code').allTextContents(), expected.map(rowID))
}

async function openDetail(page, id) {
  const row = page.getByRole('heading', { name: '提交 ' + rowID(id), exact: true }).locator('..').locator('..')
  const button = row.getByRole('button', { name: '查看提交详情' })
  await button.focus()
  const [response] = await Promise.all([
    page.waitForResponse(response => response.url() === endpoint + '/' + rowID(id)),
    button.press('Enter'),
  ])
  assert.equal(response.status(), 200, 'detail HTTP status')
  const cache = response.headers()['cache-control']?.split(',').map(value => value.trim())
  assert(cache?.length && cache.every(value => value === 'no-store'), 'detail response must not be cached')
  await row.getByRole('heading', { name: '提交时的源码' }).waitFor()
  assert.equal(await row.locator('pre code').textContent(), 'synthetic-history-private-source', 'visible stored source')
  return row
}

async function screenshot(page, name) {
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
  assert.equal(await page.locator('vite-error-overlay').count(), 0)
  await page.screenshot({ path: path.join(output, name) })
  report.screenshots.push(name)
}

function watch(page) {
  const seen = { history: [], errors: [] }
  page.setDefaultTimeout(8000)
  page.on('pageerror', error => seen.errors.push(error.name))
  page.on('console', message => {
    // 401/403 responses are intentional at the anonymous/eligibility boundary.
    if (['error', 'warning'].includes(message.type()) && !/Failed to load resource.*(401|403)/.test(message.text())) {
      seen.errors.push(message.type())
    }
  })
  page.on('request', request => {
    if (request.url().split('?')[0] === endpoint) {
      seen.history.push({ method: request.method(), subject: request.headers()['x-oj-expected-subject'], url: request.url() })
    }
  })
  return seen
}

let browser
try {
  browser = await chromium.launch({ headless: true, executablePath: process.env.OJ_CHROMIUM_EXECUTABLE })
  const desktop = await browser.newContext({ viewport: { width: 1280, height: 900 }, ignoreHTTPSErrors: true })
  const page = await desktop.newPage()
  const seen = watch(page)
  step = 'signed-out history'
  await page.goto(info.origin + '/submissions')
  await page.getByRole('link', { name: '登录已有账号', exact: true }).waitFor()
  assert.deepEqual(seen.history, [])
  passed('anonymous page offers login without fetching history')

  step = 'owner first and next page'
  await signIn(page, ready.email)
  await openHistory(page, '查看提交历史', 200)
  await page.getByText('第 1 页，共 20 条。', { exact: true }).waitFor()
  await rows(page, Array.from({ length: 20 }, (_, i) => 22 - i))
  assert.equal(await page.getByRole('heading', { name: '提交时的源码' }).count(), 0)
  await screenshot(page, 'training-history-desktop.png')
  await openDetail(page, 22)
  await screenshot(page, 'training-detail-desktop.png')
  const secondURL = await openHistory(page, '查看下一页', 200)
  assert.match(new URL(secondURL).searchParams.get('cursor') ?? '', /^[A-Za-z0-9_-]{80}$/)
  await page.getByText('第 2 页，共 2 条。已到末页。', { exact: true }).waitFor()
  await rows(page, [2, 1])
  assert.equal(await page.getByRole('heading', { name: '提交时的源码' }).count(), 0)
  assert(await page.getByRole('button', { name: '查看下一页' }).isDisabled())
  assert.deepEqual(seen.history, [
    { method: 'GET', subject: ready.subject_id, url: endpoint },
    { method: 'GET', subject: ready.subject_id, url: secondURL },
  ])
  await page.reload()
  await page.getByRole('button', { name: '查看提交历史', exact: true }).waitFor()
  assert.equal(await page.getByRole('list', { name: '训练提交记录' }).count(), 0)
  assert.equal(seen.history.length, 2)
  assert.deepEqual(seen.errors, [])
  passed('desktop owner reads only 22 fixed-context summaries across stable pages')

  step = 'narrow viewport second owner'
  const mobile = await browser.newContext({ viewport: { width: 390, height: 844 }, ignoreHTTPSErrors: true })
  const narrow = await mobile.newPage()
  const narrowSeen = watch(narrow)
  await signIn(narrow, ready.other_email)
  await openHistory(narrow, '查看提交历史', 200)
  await narrow.getByText('第 1 页，共 1 条。已到末页。', { exact: true }).waitFor()
  await rows(narrow, [24])
  const narrowRow = await openDetail(narrow, 24)
  assert(narrowSeen.history.every(request => request.method === 'GET' && request.subject === ready.other_subject_id))
  assert.deepEqual(narrowSeen.errors, [])
  await screenshot(narrow, 'training-history-mobile.png')
  await narrowRow.getByRole('button', { name: '收起提交详情' }).click()
  assert.equal(await narrow.getByRole('heading', { name: '提交时的源码' }).count(), 0)
  passed('390px second account sees only its own summary')

  step = 'unverified personal account'
  const unverified = await browser.newContext({ ignoreHTTPSErrors: true })
  const unverifiedPage = await unverified.newPage()
  const unverifiedSeen = watch(unverifiedPage)
  await signIn(unverifiedPage, ready.unverified_email)
  step = 'unverified history HTTP'
  await openHistory(unverifiedPage, '查看提交历史', 403)
  step = 'unverified eligibility state'
  await unverifiedPage.getByText('只有已验证邮箱的个人账号可以查看训练提交。', { exact: true }).waitFor()
  assert.equal(await unverifiedPage.getByRole('list', { name: '训练提交记录' }).count(), 0)
  assert.equal(unverifiedSeen.history.length, 1)
  assert.deepEqual(unverifiedSeen.errors, [])
  passed('unverified signed-in account sees an eligibility message and no rows')
  report.success = true
} catch (error) {
  const detail = error.message?.split('\n')[0].replaceAll('synthetic protocol password', '[fixture password]')
  console.error('FAIL ' + step + ': ' + error.name + (detail ? ' — ' + detail : ''))
  process.exitCode = 1
} finally {
  await browser?.close()
  await fs.writeFile(path.join(output, 'browser-qa.json'), JSON.stringify(report))
}
