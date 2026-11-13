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
assert(ready?.url === info.origin && /^[a-f0-9]{32}$/.test(ready.review_id))
for (const email of [ready.email, ready.other_email]) assert(/^[a-zA-Z0-9._+-]+@example\.invalid$/.test(email))
for (const id of [ready.subject_id, ready.other_subject_id]) assert(/^[a-f0-9]{32}$/.test(id))
const endpoint = `/api/v1/problem-review/${ready.review_id}/materials`
const confirmation = `/api/v1/problem-review/${ready.review_id}/statement-confirmation`
const pagePath = `/review/materials/${ready.review_id}`
const report = { success: false, checks: [], screenshots: [] }
function passed(name) { report.checks.push(name); console.log('PASS ' + name) }
let step = 'setup'
let status

async function exercise(browser, viewport, reviewer, select, confirm = false) {
  status = undefined
  const context = await browser.newContext({ ignoreHTTPSErrors: true, viewport })
  try {
    const page = await context.newPage()
    const reads = [], errors = [], foreign = []
    page.on('pageerror', e => errors.push(e.name))
    page.on('request', r => {
      const url = new URL(r.url())
      if (url.origin !== info.origin) foreign.push(r.resourceType())
      if (url.pathname.startsWith('/api/v1/problem-review/')) reads.push([r.method(), url.pathname, r.headers()['x-oj-expected-subject']])
    })
    step = 'login'
    await page.goto(info.origin + '/login')
    await page.getByLabel('邮箱', { exact: true }).fill(reviewer ? ready.other_email : ready.email)
    await page.getByLabel('密码', { exact: true }).fill('synthetic protocol password')
    await page.getByRole('button', { name: '登录', exact: true }).click()
    await page.getByText('登录成功，当前会话已核验。', { exact: true }).waitFor({ timeout: 8000 })
    step = 'select review'
    await page.goto(info.origin + (select ? '/review/materials' : pagePath))
    await page.getByRole('heading', { name: '审核材料', exact: true }).waitFor()
    if (select) {
      await page.getByLabel('送审 ID', { exact: true }).fill(ready.review_id)
      await page.getByRole('button', { name: '选择送审项' }).click()
    }
    await page.waitForURL(info.origin + pagePath)
    assert.deepEqual(reads, [])
    step = 'read review'
    const [response] = await Promise.all([
      page.waitForResponse(r => r.url() === info.origin + endpoint, { timeout: 8000 }),
      page.getByRole('button', { name: '检查会话并读取材料' }).click(),
    ])
    status = response.status()
    assert.equal(status, reviewer ? 200 : 404)
    assert.deepEqual(reads, [['GET', endpoint, reviewer ? ready.other_subject_id : ready.subject_id]])
    if (reviewer) {
      step = 'render review'
      await page.getByRole('heading', { name: 'Browser statement', exact: true }).waitFor()
      await page.getByRole('heading', { name: '冻结标程', exact: true }).waitFor()
      assert.equal(await page.getByRole('region', { name: '冻结标程' }).locator('pre').first().textContent(), 'synthetic A+B material; not executed by this test\n')
      assert.deepEqual(await page.getByRole('region', { name: '原始样例' }).locator('pre').allTextContents(), ['1 2\n', '3\n'])
      assert(await page.locator('.katex').count() > 0)
      assert.equal(await page.locator('vite-error-overlay').count(), 0)
      step = 'fit viewport'
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
      const screenshot = `reviewer-${viewport.width}.png`
      await page.screenshot({ path: path.join(output, screenshot), fullPage: true })
      report.screenshots.push(screenshot)
      step = 'refresh review'
      await page.reload()
      await page.getByRole('button', { name: '检查会话并读取材料' }).waitFor()
      assert.equal(await page.getByRole('heading', { name: '冻结标程' }).count(), 0)
      assert.equal(reads.length, 1)
      if (confirm) {
        step = 'confirm four explicit checks'
        await page.getByRole('button', { name: '检查会话并读取材料' }).click()
        const submit = page.getByRole('button', { name: '记录题意确认' })
        await submit.waitFor()
        assert(await submit.isDisabled())
        const checks = await page.getByRole('group', { name: '人工核对清单' }).getByRole('checkbox').all()
        assert.equal(checks.length, 4)
        for (const item of checks.slice(0, 3)) await item.check()
        assert(await submit.isDisabled())
        await checks[3].check()
        assert(!await submit.isDisabled())
        assert.equal(reads.filter(([method]) => method === 'POST').length, 0)
        step = 'lose the response after the real API commits'
        let receipt, key
        await page.route(info.origin + confirmation, async route => {
          const request = route.request()
          assert.equal(request.method(), 'POST')
          assert.equal(request.headers()['origin'], info.origin)
          const body = request.postDataJSON()
          assert.equal(body.checklist_version, 'oj.statement-semantics.v1')
          key = body.request_key
          const response = await route.fetch({ maxRedirects: 0, maxRetries: 0 })
          assert.equal(response.status(), 200)
          receipt = await response.json()
          await route.abort()
        })
        await submit.click()
        await page.getByText(/尚未核实本次确认/).waitFor()
        assert.match(key, /^[a-f0-9]{32}$/)
        assert.equal(receipt.request_key, key)
        step = 'refresh and recover by GET without resubmission'
        const before = reads.length
        await page.reload()
        await page.getByRole('button', { name: '检查会话并读取材料' }).waitFor()
        assert.equal(reads.length, before)
        await page.getByRole('button', { name: '检查会话并读取材料' }).click()
        const query = page.getByRole('button', { name: '查询原确认回执' })
        await query.waitFor()
        assert.equal(await submit.count(), 0)
        const receiptPath = `${confirmation}/${key}`
        const [readback] = await Promise.all([
          page.waitForResponse(r => r.url() === info.origin + receiptPath),
          query.click(),
        ])
        assert.equal(readback.status(), 200)
        assert.deepEqual(await readback.json(), receipt)
        await page.getByText('已核实这份冻结题面的确认回执。').waitFor()
        assert.deepEqual(reads.slice(1), [
          ['GET', endpoint, ready.other_subject_id], ['POST', confirmation, ready.other_subject_id],
          ['GET', endpoint, ready.other_subject_id], ['GET', receiptPath, ready.other_subject_id],
        ])
      }
    } else {
      step = 'render denial'
      await page.getByText('无法读取材料。请核对送审 ID、当前分配及服务状态，再手动重试。', { exact: true }).waitFor()
      assert.equal(await page.getByRole('heading', { name: '冻结标程' }).count(), 0)
      assert.equal(await page.getByRole('heading', { name: 'Browser statement' }).count(), 0)
    }
    step = 'check browser events'
    assert.deepEqual(errors, [])
    assert.deepEqual(foreign, [])
  } finally { await context.close() }
}

let browser, phase = 'launch'
try {
  browser = await chromium.launch({ headless: true, executablePath: process.env.OJ_CHROMIUM_EXECUTABLE })
  phase = 'desktop reviewer'
  await exercise(browser, { width: 1280, height: 960 }, true, true)
  passed('assigned reviewer opens frozen statement samples and solution on desktop')
  phase = 'narrow reviewer'
  await exercise(browser, { width: 390, height: 844 }, true, false, true)
  passed('narrow deep link is usable and refresh clears materials without refetching')
  passed('four explicit checks confirm once and reload recovers the lost receipt by original-key GET')
  phase = 'author denied'
  await exercise(browser, { width: 1280, height: 960 }, false, false)
  passed('the author with reviewer role receives no private review materials')
  report.success = true
} catch {
  console.error(`FAIL ${phase}: ${step}; material HTTP status: ${status ?? 'none'}`)
  process.exitCode = 1
} finally {
  await browser?.close()
  await fs.writeFile(path.join(output, 'browser-qa.json'), JSON.stringify(report))
}
