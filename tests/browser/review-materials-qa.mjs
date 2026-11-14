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
const eligibilityPath = `/api/v1/problem-review/${ready.review_id}/eligibility`
const confirmation = `/api/v1/problem-review/${ready.review_id}/statement-confirmation`
const pagePath = `/review/materials/${ready.review_id}`
const intakePath = '/api/v1/problem-draft-management/review-submissions'
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
      step = 'read current approval blockers'
      const [eligibility] = await Promise.all([
        page.waitForResponse(r => r.url() === info.origin + eligibilityPath),
        page.getByRole('button', { name: '查看当前阻断项' }).click(),
      ])
      assert.equal(eligibility.status(), 200)
      await page.getByText('缺少生产检查器准入证据', { exact: true }).waitFor()
      await page.getByText('缺少目标运行环境验收证据', { exact: true }).waitFor()
      assert.deepEqual(reads.at(-1), ['GET', eligibilityPath, ready.other_subject_id])
      step = 'fit viewport'
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
      const screenshot = `reviewer-${viewport.width}.png`
      await page.screenshot({ path: path.join(output, screenshot), fullPage: true })
      report.screenshots.push(screenshot)
      step = 'refresh review'
      await page.reload()
      await page.getByRole('button', { name: '检查会话并读取材料' }).waitFor()
      assert.equal(await page.getByRole('heading', { name: '冻结标程' }).count(), 0)
      assert.equal(await page.getByText('缺少生产检查器准入证据', { exact: true }).count(), 0)
      assert.equal(reads.length, 2)
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
        assert.deepEqual(reads.slice(2), [
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

async function authorIntake(browser) {
  assert.match(ready.review_draft_id, /^[a-f0-9]{32}$/)
  assert.match(ready.review_reference_key, /^[A-Za-z0-9_-]{16,128}$/)
  const context = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 390, height: 844 } })
  try {
    const page = await context.newPage()
    const calls = []
    page.on('request', request => {
      const url = new URL(request.url())
      if (url.origin === info.origin && url.pathname.startsWith(intakePath)) calls.push([request.method(), url.pathname])
    })
    step = 'author login and restore existing browser reference key'
    await page.goto(info.origin + '/login')
    await page.getByLabel('邮箱', { exact: true }).fill(ready.email)
    await page.getByLabel('密码', { exact: true }).fill('synthetic protocol password')
    await page.getByRole('button', { name: '登录', exact: true }).click()
    await page.getByText('登录成功，当前会话已核验。', { exact: true }).waitFor()
    await page.evaluate(async ({ owner, draft, key }) => {
      const harness = await import('/journal-test/journal-harness.js')
      await harness.seedReportedReferenceKey(owner, draft, key)
    }, { owner: ready.subject_id, draft: ready.review_draft_id, key: ready.review_reference_key })
    step = 'concurrent durable reservations select only one sender'
    const reserved = await page.evaluate(async () => {
      const harness = await import('/journal-test/journal-harness.js')
      const owner = '0'.repeat(32), draft = '1'.repeat(32), report = '2'.repeat(32)
      const entry = { version: '3'.repeat(64), intent: { request_key: 'web-review-v1-' + report,
        draft_id: draft, reference_request_id: report, import_revision: '4'.repeat(64) } }
      const results = await Promise.all([harness.reserveReviewIntake(owner, entry), harness.reserveReviewIntake(owner, entry)])
      const own = await harness.listReviewIntakes(owner, draft, '')
      const other = await harness.listReviewIntakes('5'.repeat(32), draft, '')
      return { senders: results.filter(Boolean).length, own: own.length, other: other.length }
    })
    assert.deepEqual(reserved, { senders: 1, own: 1, other: 0 })
    await page.goto(info.origin + '/author/drafts/' + ready.review_draft_id)
    const openReport = async () => {
      await page.getByRole('button', { name: '检查会话并打开草稿' }).click()
      await page.getByRole('button', { name: '查看申请回执' }).click()
      await page.getByRole('button', { name: '查看验证结果' }).click()
    }
    await openReport()
    step = 'send exact report once and lose the response'
    const submit = page.getByRole('button', { name: '送审此报告' })
    await submit.waitFor()
    assert.deepEqual(calls, [])
    let receipt
    await page.route(info.origin + intakePath, async route => {
      const request = route.request()
      assert.equal(request.method(), 'POST')
      assert.equal(request.headers()['origin'], info.origin)
      assert.equal(request.headers()['x-oj-expected-subject'], ready.subject_id)
      const body = request.postDataJSON()
      assert.equal(body.draft_id, ready.review_draft_id)
      assert.match(body.import_revision, /^[a-f0-9]{64}$/)
      const response = await route.fetch({ maxRedirects: 0, maxRetries: 0 })
      assert.equal(response.status(), 200)
      receipt = await response.json()
      assert.equal(receipt.request_key, body.request_key)
      assert.equal(receipt.reference_request_id, body.reference_request_id)
      assert.equal(receipt.import_revision, body.import_revision)
      await route.abort()
    })
    await submit.click()
    const query = page.getByRole('button', { name: '查询原送审回执' })
    await query.waitFor()
    assert.match(receipt.id, /^[a-f0-9]{32}$/)
    assert.notEqual(receipt.id, ready.review_id)
    step = 'start a new reference request without losing the original intake'
    await page.getByRole('button', { name: '再次申请验证' }).click()
    await page.getByText(/状态：待完成/).waitFor()
    step = 'reload and recover the original intake receipt without the old report'
    await page.reload()
    await page.getByRole('button', { name: '检查会话并打开草稿' }).click()
    await page.getByRole('button', { name: '查看本机送审记录' }).click()
    const historyQuery = page.getByRole('button', { name: '查该报告的送审回执' })
    await historyQuery.waitFor()
    assert.equal(await submit.count(), 0)
    const lookup = intakePath + '/' + receipt.request_key
    await page.route(info.origin + lookup, route => route.fulfill({ status: 404, contentType: 'application/problem+json', body: '{}' }), { times: 1 })
    await historyQuery.click()
    await page.getByText(/暂时查不到并不表示未送审/).waitFor()
    assert.equal(await submit.count(), 0)
    const [response] = await Promise.all([page.waitForResponse(r => r.url() === info.origin + lookup), historyQuery.click()])
    assert.equal(response.status(), 200)
    assert.deepEqual(await response.json(), receipt)
    await page.getByText(`送审 ID：${receipt.id}。此回执仅表示材料已送审。`, { exact: true }).waitFor()
    assert.deepEqual(calls, [['POST', intakePath], ['GET', lookup], ['GET', lookup]])
  } finally { await context.close() }
}

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
  phase = 'author intake'
  await authorIntake(browser)
  passed('author sends once and reload recovers the original review ID after a new reference request')
  report.success = true
} catch {
  console.error(`FAIL ${phase}: ${step}; material HTTP status: ${status ?? 'none'}`)
  process.exitCode = 1
} finally {
  await browser?.close()
  await fs.writeFile(path.join(output, 'browser-qa.json'), JSON.stringify(report))
}
