import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import process from 'node:process'
import { createRequire } from 'node:module'

const require = createRequire(import.meta.url)
const { chromium } = require(process.env.OJ_PLAYWRIGHT_MODULE)
const info = JSON.parse(fs.readFileSync(process.env.OJ_BROWSER_QA_INFO, 'utf8'))
const origin = new URL(info.origin)
assert(origin.protocol === 'https:' && origin.hostname === '127.0.0.1' && origin.port && origin.origin === info.origin)
const base = '/api/v1/session'
const report = { checks: [], success: false }
let browser
let phase = 'launch'
const passed = name => report.checks.push(name)

async function login(page, expected) {
  await page.goto(origin.origin + '/login')
  await page.getByLabel('邮箱', { exact: true }).fill(info.email)
  await page.getByLabel('密码', { exact: true }).fill('synthetic protocol password')
  const completed = page.waitForResponse(response => new URL(response.url()).pathname === base + '/login/complete')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  assert.equal((await completed).status(), expected)
  if (expected === 204) await page.getByText('登录成功，当前会话已核验。', { exact: true }).waitFor()
}

async function sessionStatus(context) {
  const response = await context.request.get(origin.origin + base)
  const status = response.status()
  await response.dispose()
  return status
}

try {
  browser = await chromium.launch({
    headless: true, args: ['--disable-background-networking'],
    ...(process.env.OJ_CHROMIUM_EXECUTABLE ? { executablePath: process.env.OJ_CHROMIUM_EXECUTABLE } : {}),
  })
  const contexts = []
  const pages = []
  for (let i = 0; i < 4; i++) {
    const context = await browser.newContext({ ignoreHTTPSErrors: true, viewport: i === 3 ? { width: 390, height: 844 } : { width: 1280, height: 960 } })
    await context.route('**/*', route => new URL(route.request().url()).origin === origin.origin ? route.continue() : route.abort())
    contexts.push(context)
    pages.push(await context.newPage())
  }
  const page = pages[3]
  const fourth = contexts[3]
  const originals = contexts.slice(0, 3)
  phase = 'three slots and pending replacement'
  for (const first of pages.slice(0, 3)) await login(first, 204)
  await login(page, 409)
  assert.deepEqual(await Promise.all(originals.map(sessionStatus)), [200, 200, 200])
  assert.equal(await sessionStatus(fourth), 401)
  await page.getByRole('button', { name: '读取可替换会话', exact: true }).click()
  const choices = page.getByRole('group', { name: '选择要退出的旧会话' }).getByRole('radio')
  await choices.nth(2).waitFor()
  assert.equal(await choices.count(), 3)
  const replace = page.getByRole('button', { name: '退出所选会话并登录', exact: true })
  assert(await replace.isDisabled(), 'No old session can be replaced without a choice')
  passed('fourth browser sees three candidates without replacing any session')

  phase = 'confirm one replacement'
  await choices.first().check()
  const result = page.waitForResponse(response => new URL(response.url()).pathname === base + '/login/replace-device')
  await replace.click()
  assert.equal((await result).status(), 204)
  assert.equal(await sessionStatus(fourth), 200)
  assert.deepEqual((await Promise.all(originals.map(sessionStatus))).sort(), [200, 200, 401])
  assert(!(await fourth.cookies(origin.origin)).some(cookie => cookie.name === '__Host-oj_login'))
  passed('explicit replacement authenticates the fourth browser and revokes exactly one old session')

  phase = 'revoke other sessions'
  await page.goto(origin.origin + '/account')
  await page.getByText(info.subject_id, { exact: true }).waitFor()
  const panel = page.getByRole('region', { name: '已登录的会话' })
  const read = panel.getByRole('button', { name: '读取会话列表', exact: true })
  await read.click()
  await panel.getByRole('listitem').nth(2).waitFor()
  await panel.getByRole('button', { name: '退出其他会话', exact: true }).click()
  await panel.getByRole('button', { name: '确认撤销', exact: true }).click()
  await panel.getByText('已撤销所选会话。请重新读取列表确认当前状态。', { exact: true }).waitFor()
  assert.equal(await sessionStatus(fourth), 200)
  assert.deepEqual(await Promise.all(originals.map(sessionStatus)), [401, 401, 401])
  await read.click()
  await panel.getByText('当前会话', { exact: true }).waitFor()
  assert.equal(await panel.getByRole('listitem').count(), 1)
  assert(await page.evaluate(() => document.cookie === '' && localStorage.length === 0 && sessionStorage.length === 0))
  passed('account UI revokes other sessions and keeps the current one')
  report.success = true
} catch {
  report.failed_phase = phase
  process.exitCode = 1
} finally {
  await browser?.close()
  fs.writeFileSync(path.join(process.env.OJ_BROWSER_QA_OUTPUT, 'browser-qa.json'), JSON.stringify(report, null, 2) + '\n')
}
