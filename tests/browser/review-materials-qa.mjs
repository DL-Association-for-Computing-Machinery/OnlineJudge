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
const assignments = '/api/v1/problem-review/assignments'
const eligibilityPath = `/api/v1/problem-review/${ready.review_id}/eligibility`
const confirmation = `/api/v1/problem-review/${ready.review_id}/statement-confirmation`
const returnPath = `/api/v1/problem-review/${ready.review_id}/return`
const pagePath = `/review/materials/${ready.review_id}`
const intakePath = '/api/v1/problem-draft-management/review-submissions'
const report = { success: false, checks: [], screenshots: [] }
function passed(name) { report.checks.push(name); console.log('PASS ' + name) }
let step = 'setup'
let status

async function exercise(browser, viewport, reviewer, select, confirm = false, doReturn = false) {
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
    assert.deepEqual(reads, [])
    if (select) {
      const [response] = await Promise.all([
        page.waitForResponse(r => r.url() === info.origin + assignments, { timeout: 8000 }),
        page.getByRole('button', { name: '检查会话并查看分配' }).click(),
      ])
      assert.equal(response.status(), 200)
      assert.deepEqual(reads, [['GET', assignments, reviewer ? ready.other_subject_id : ready.subject_id]])
      if (reviewer) {
        const choice = page.getByRole('link', { name: new RegExp(ready.review_id) })
        await choice.focus()
        assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
        const listScreenshot = `assignments-${viewport.width}.png`
        await page.screenshot({ path: path.join(output, listScreenshot), fullPage: true })
        report.screenshots.push(listScreenshot)
        await choice.press('Enter')
      } else {
        await page.getByText('没有可查看的审核分配。', { exact: true }).waitFor()
        assert.equal(await page.getByRole('link', { name: new RegExp(ready.review_id) }).count(), 0)
        await page.getByLabel('送审 ID', { exact: true }).fill(ready.review_id)
        await page.getByRole('button', { name: '选择送审项' }).click()
      }
      reads.length = 0
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
      if (doReturn) await reviewerReturn(page, reads)
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

async function reviewerReturn(page, reads) {
  step = 'validate reason and lose the real return response after commit'
  const load = page.getByRole('button', { name: '检查会话并读取材料' })
  const query = page.getByRole('button', { name: '查询原退回回执' })
  const submit = page.getByRole('button', { name: '确认退回材料' })
  const reasons = '请更新样例解释。'
  await load.click()
  await page.getByText('填写退回理由', { exact: true }).click()
  const field = page.getByLabel('退回理由', { exact: true })
  assert(await submit.isDisabled())
  await field.fill('题'.repeat(171))
  assert(await submit.isDisabled())
  await field.fill(reasons)
  assert(!await submit.isDisabled())
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
  assert.equal(reads.filter(([method]) => method === 'POST').length, 0)
  let receipt
  await page.route(info.origin + returnPath, async route => {
    const req = route.request()
    assert.equal(req.method(), 'POST')
    assert.equal(req.headers()['origin'], info.origin)
    assert.equal(req.headers()['x-oj-expected-subject'], ready.other_subject_id)
    const body = req.postDataJSON()
    const saved = await page.evaluate(() => Object.entries(sessionStorage).filter(([key]) => key.startsWith('oj.review-return.v1:')))
    assert.equal(saved.length, 1)
    assert(saved[0][0].includes(ready.other_subject_id))
    assert.deepEqual(JSON.parse(saved[0][1]), { key: body.request_key, assignment: body.assignment_revision })
    assert.equal(body.reason, reasons)
    const response = await route.fetch({ maxRedirects: 0, maxRetries: 0 })
    assert.equal(response.status(), 200)
    receipt = await response.json()
    assert.equal(receipt.request_key, body.request_key)
    await route.abort()
  })
  await submit.click()
  await page.getByText('尚未核实本次退回。', { exact: true }).waitFor()
  assert.equal(await submit.count(), 0)
  assert.equal(await page.getByRole('button', { name: '记录题意确认' }).count(), 0)
  const before = reads.length
  step = 'reload, keep missing receipt pending, then recover without another POST'
  await page.reload()
  await load.waitFor()
  assert.equal(reads.length, before)
  await load.click()
  await query.waitFor()
  assert.equal(await submit.count(), 0)
  const receiptPath = `${returnPath}/${receipt.request_key}`
  await page.route(info.origin + receiptPath, route => route.fulfill({ status: 404, contentType: 'application/problem+json', body: '{}' }), { times: 1 })
  await query.click()
  await page.getByText('尚未核实本次退回。', { exact: true }).waitFor()
  assert(await query.isEnabled())
  const [readback] = await Promise.all([
    page.waitForResponse(r => r.url() === info.origin + receiptPath),
    query.click(),
  ])
  assert.equal(readback.status(), 200)
  assert.deepEqual(await readback.json(), receipt)
  await page.getByText('已核实这份材料的退回回执。', { exact: true }).waitFor()
  assert.equal(await query.count(), 0)
  await page.getByRole('button', { name: '查看当前阻断项' }).click()
  await page.getByText('同一修订的材料已被退回', { exact: true }).waitFor()
  assert.deepEqual(reads.slice(2), [
    ['GET', endpoint, ready.other_subject_id], ['POST', returnPath, ready.other_subject_id],
    ['GET', endpoint, ready.other_subject_id], ['GET', receiptPath, ready.other_subject_id],
    ['GET', receiptPath, ready.other_subject_id], ['GET', eligibilityPath, ready.other_subject_id],
  ])
  const screenshot = 'review-return-narrow.png'
  await page.screenshot({ path: path.join(output, screenshot), fullPage: true })
  report.screenshots.push(screenshot)
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
    await authorSolution(page, receipt, calls)
    return { storage: await context.storageState(), receipt }
  } finally { await context.close() }
}

async function authorSolution(page, receipt, calls) {
  const intake = intakePath + '/' + receipt.request_key
  const sourcePath = intake + '/solution'
  const confirmPath = intake + '/solution-confirmation'
  const open = page.getByRole('button', { name: '查看该报告的冻结标程' })
  const submit = page.getByRole('button', { name: '确认这份冻结标程' })
  const query = page.getByRole('button', { name: '查询原标程确认回执' })
  const source = page.getByRole('region', { name: '冻结标程' })
  let from = calls.length
  step = 'reject frozen source for another import revision'
  await page.route(info.origin + sourcePath, async route => {
    const response = await route.fetch({ maxRedirects: 0, maxRetries: 0 })
    assert.equal(response.status(), 200)
    const data = await response.json()
    await route.fulfill({ response, json: { ...data, import_revision: '0'.repeat(64) } })
  }, { times: 1 })
  await open.click()
  await page.getByText('无法核实此报告的冻结标程。', { exact: true }).waitFor()
  assert.equal(await source.count(), 0)
  assert.equal(await submit.count(), 0)
  step = 'view server-bound frozen source without sending a confirmation'
  const [preview] = await Promise.all([page.waitForResponse(r => r.url() === info.origin + sourcePath), open.click()])
  assert.equal(preview.status(), 200)
  const view = await preview.json()
  const binding = { import_revision: receipt.import_revision, formal_plan_sha256: view.formal_plan_sha256 }
  await submit.waitFor()
  assert.equal(await source.locator('pre').first().textContent(), 'synthetic A+B material; not executed by this test\n')
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
  assert.deepEqual(calls.slice(from), [['GET', intake], ['GET', sourcePath], ['GET', intake], ['GET', sourcePath]])
  const screenshot = 'author-solution-narrow.png'
  await source.screenshot({ path: path.join(output, screenshot) })
  report.screenshots.push(screenshot)
  step = 'leave the confirmation unsent when the browser cannot retain its binding'
  await page.evaluate(() => { Storage.prototype.setItem = () => { throw new Error('synthetic storage failure') } })
  await submit.click()
  await page.getByText('本标签页无法保存或读取原确认记录。', { exact: true }).waitFor()
  assert.equal(calls.slice(from).some(([method]) => method === 'POST'), false)
  await page.reload()
  await page.getByRole('button', { name: '检查会话并打开草稿' }).click()
  await page.getByRole('button', { name: '查看本机送审记录' }).click()
  await open.click()
  await submit.waitFor()
  from = calls.length
  step = 'send bound confirmation once and drop the committed response'
  let serverReceipt
  await page.route(info.origin + confirmPath, async route => {
    const request = route.request()
    assert.equal(request.method(), 'POST')
    assert.equal(request.headers()['origin'], info.origin)
    assert.equal(request.headers()['x-oj-expected-subject'], ready.subject_id)
    assert.deepEqual(request.postDataJSON(), binding)
    const saved = await page.evaluate(() => Object.entries(sessionStorage).filter(([key]) => key.startsWith('oj.author-solution-confirmation.v1:')))
    assert.equal(saved.length, 1)
    assert(saved[0][0].includes(receipt.request_key))
    assert.deepEqual(JSON.parse(saved[0][1]), binding)
    const response = await route.fetch({ maxRedirects: 0, maxRetries: 0 })
    assert.equal(response.status(), 200)
    serverReceipt = await response.json()
    await route.abort()
  }, { times: 1 })
  await submit.click()
  await page.getByText(/尚未核实标程确认/).waitFor()
  step = 'refresh and reject a missing receipt before recovering the same binding'
  await page.reload()
  await page.getByRole('button', { name: '检查会话并打开草稿' }).click()
  await page.getByRole('button', { name: '查看本机送审记录' }).click()
  await query.waitFor()
  assert.equal(await source.count(), 0)
  assert.equal(await submit.count(), 0)
  await page.route(info.origin + confirmPath, route => route.fulfill({ status: 404, contentType: 'application/problem+json', body: '{}' }), { times: 1 })
  await query.click()
  await page.getByText(/尚未核实标程确认/).waitFor()
  const [recovered] = await Promise.all([page.waitForResponse(r => r.url() === info.origin + confirmPath), query.click()])
  assert.equal(recovered.status(), 200)
  assert.deepEqual(await recovered.json(), serverReceipt)
  await page.getByText('已核实此报告的作者标程确认回执。', { exact: true }).waitFor()
  assert.equal(await query.count(), 0)
  assert.deepEqual(calls.slice(from), [
    ['POST', confirmPath], ['GET', intake], ['GET', confirmPath], ['GET', intake], ['GET', confirmPath],
  ])
  step = 'observe another account while a frozen source response is pending'
  let started, release, finished
  const responseStarted = new Promise(resolve => { started = resolve })
  const releaseResponse = new Promise(resolve => { release = resolve })
  const responseFinished = new Promise(resolve => { finished = resolve })
  await page.route(info.origin + sourcePath, async route => {
    const response = await route.fetch({ maxRedirects: 0, maxRetries: 0 })
    assert.equal(response.status(), 200)
    started()
    await releaseResponse
    try { await route.fulfill({ response }) } catch { /* Account invalidation can cancel the request. */ }
    finally { finished() }
  }, { times: 1 })
  await open.click()
  await responseStarted
  const sessionPath = info.origin + '/api/v1/session'
  await page.route(sessionPath, async route => {
    const response = await route.fetch({ maxRedirects: 0, maxRetries: 0 })
    assert.equal(response.status(), 200)
    const session = await response.json()
    assert.equal(session.subject_id, ready.subject_id)
    await route.fulfill({ response, json: { ...session, subject_id: ready.other_subject_id } })
  }, { times: 1 })
  await Promise.all([
    page.waitForResponse(r => r.url() === sessionPath && r.status() === 200),
    page.getByRole('button', { name: '检查会话并打开草稿' }).click(),
  ])
  release()
  await responseFinished
  assert.equal(await source.count(), 0)
  assert.equal(await submit.count(), 0)
  assert.equal(await query.count(), 0)
}

async function authorReturn(browser, session) {
  const receipt = session.receipt
  step = 'reopen with the existing author session'
  const context = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 390, height: 844 }, storageState: session.storage })
  try {
    const page = await context.newPage()
    const errors = [], calls = []
    page.on('pageerror', error => errors.push(error.name))
    page.on('request', request => {
      if (request.url().startsWith(info.origin + intakePath)) {
        calls.push([request.method(), new URL(request.url()).pathname, request.headers()['x-oj-expected-subject']])
      }
    })
    await page.goto(info.origin + '/author/drafts/' + ready.review_draft_id)
    step = 'restore the author journal for the same material'
    const original = intakePath + '/' + receipt.request_key
    await page.evaluate(async ({ owner, receipt }) => {
      const { reserveReviewIntake } = await import('/journal-test/journal-harness.js')
      const { request_key, draft_id, reference_request_id, import_revision, content_version } = receipt
      await reserveReviewIntake(owner, { version: content_version, intent: { request_key, draft_id, reference_request_id, import_revision } })
    }, { owner: ready.subject_id, receipt })
    step = 'open the matching author journal entry'
    await page.getByRole('button', { name: '检查会话并打开草稿' }).click()
    await page.getByRole('button', { name: '查看本机送审记录' }).click()
    const find = page.getByRole('button', { name: '查看该报告的退回理由' })
    await find.waitFor()
    assert.deepEqual(calls, [])
    step = 'read the private return status using the original intake key'
    const [readback] = await Promise.all([
      page.waitForResponse(r => r.url() === info.origin + original + '/return', { timeout: 8000 }),
      find.click(),
    ])
    status = readback.status()
    assert.equal(status, 200)
    const returned = await readback.json()
    assert.deepEqual(Object.keys(returned).sort(), ['evidence_sha256', 'import_revision', 'reason', 'returned_at', 'review_id'])
    await page.getByText(`送审 ID：${receipt.id}。已退回。`, { exact: true }).waitFor()
    const reason = page.getByText('退回理由：请更新样例解释。', { exact: true })
    await reason.waitFor()
    assert.deepEqual(calls, [['GET', original, ready.subject_id], ['GET', original + '/return', ready.subject_id]])
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
    const screenshot = 'author-return-narrow.png'
    await page.screenshot({ path: path.join(output, screenshot), fullPage: true })
    report.screenshots.push(screenshot)
    await page.reload()
    await page.getByRole('button', { name: '检查会话并打开草稿' }).waitFor()
    assert.equal(await reason.count(), 0)
    assert.equal(calls.length, 2)
    assert.deepEqual(errors, [])
  } finally { await context.close() }
}

try {
  browser = await chromium.launch({ headless: true, executablePath: process.env.OJ_CHROMIUM_EXECUTABLE })
  phase = 'desktop reviewer'
  await exercise(browser, { width: 1280, height: 960 }, true, true)
  passed('assigned reviewer discovers and opens frozen materials by keyboard on desktop')
  phase = 'narrow reviewer'
  await exercise(browser, { width: 390, height: 844 }, true, true, true)
  passed('narrow assignment list is usable and refreshing the material clears private content')
  passed('four explicit checks confirm once and reload recovers the lost receipt by original-key GET')
  phase = 'author denied'
  await exercise(browser, { width: 1280, height: 960 }, false, true)
  passed('the author with reviewer role gets an empty list and cannot read their materials')
  phase = 'author intake'
  const authorSession = await authorIntake(browser)
  passed('author sends once and reload recovers the original review ID after a new reference request')
  passed('author previews the frozen solution and recovers one explicit confirmation after a missing receipt')
  phase = 'review return'
  await exercise(browser, { width: 390, height: 844 }, true, false, false, true)
  passed('reviewer returns once and recovers a lost decision by the original key after a missing receipt')
  phase = 'author return'
  await authorReturn(browser, authorSession)
  passed('author sees the matching return reason through the original intake and reload clears it')
  report.success = true
} catch {
  console.error(`FAIL ${phase}: ${step}; material HTTP status: ${status ?? 'none'}`)
  process.exitCode = 1
} finally {
  await browser?.close()
  await fs.writeFile(path.join(output, 'browser-qa.json'), JSON.stringify(report))
}
