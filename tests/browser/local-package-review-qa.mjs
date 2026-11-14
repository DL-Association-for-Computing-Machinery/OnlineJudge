import assert from 'node:assert/strict'
import { readFile, writeFile } from 'node:fs/promises'
import { pathToFileURL } from 'node:url'

const info = JSON.parse(await readFile(process.env.OJ_BROWSER_QA_INFO, 'utf8'))
assert.match(info.url, /^https:\/\/127\.0\.0\.1:\d+$/)
const { chromium } = await import(pathToFileURL(process.env.OJ_PLAYWRIGHT_MODULE))
const browser = await chromium.launch({ executablePath: process.env.OJ_CHROMIUM_EXECUTABLE })
const draftAPI = '/api/v1/problem-draft-management'
const reference = draftAPI + '/reference-requests'
const submission = draftAPI + '/review-submissions'
const requestKey = 'package-qa-reference'
const reviewKey = 'package-qa-review'

async function login(email, subject) {
  const context = await browser.newContext({ ignoreHTTPSErrors: true })
  const page = await context.newPage()
  await page.goto(info.url + '/login')
  await page.getByLabel('邮箱', { exact: true }).fill(email)
  await page.getByLabel('密码', { exact: true }).fill('synthetic protocol password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await page.getByText('登录成功，当前会话已核验。', { exact: true }).waitFor()
  const request = (path, method, data) => context.request.fetch(info.url + path, { method, data, headers: {
    Origin: info.url, 'X-OJ-Expected-Subject': subject,
  } })
  return { page, request }
}
async function checked(response, status = 200) {
  assert.equal(response.status(), status, await response.text())
  return response.json()
}
async function save(path, value) {
  await writeFile(process.env[path], JSON.stringify(value) + '\n', { mode: 0o600, flag: 'wx' })
}
async function logout(page) {
  await page.goto(info.url + '/account')
  await page.getByRole('button', { name: '退出当前会话', exact: true }).click()
  await page.getByText('已退出当前会话。', { exact: true }).waitFor()
}

try {
  switch (process.env.OJ_BROWSER_PACKAGE_STEP) {
  case 'prepare': {
    const author = await login(info.email, info.subject_id)
    const { sources } = await checked(await author.request('/api/v1/problem-draft-sources', 'GET'))
    const source = sources.find(s => s.source_id === info.source_id)
    assert(source)
    const draft = await checked(await author.request('/api/v1/problem-draft-imports', 'POST', {
      request_key: 'package-qaimport', source_id: source.source_id, expected_revision: source.import_revision,
    }))
    const ref = await checked(await author.request(reference, 'POST', { request_key: requestKey, draft_id: draft.draft_id }))
    assert.equal((await author.request(reference + '/' + requestKey + '/prepare', 'POST')).status(), 202)
    await save('OJ_BROWSER_PACKAGE_STATE', { source, draft, ref })
    await logout(author.page)
    break
  }
  case 'intake': {
    const state = JSON.parse(await readFile(process.env.OJ_BROWSER_PACKAGE_STATE, 'utf8'))
    const author = await login(info.email, info.subject_id)
    const report = await checked(await author.request(reference + '/' + requestKey + '/report', 'GET'))
    assert.equal(report.formal.verdict, 'AC')
    assert.equal(report.formal.passed, report.formal.total)
    const r = await checked(await author.request(submission, 'POST', {
      request_key: reviewKey, draft_id: state.draft.draft_id, import_revision: state.source.import_revision, reference_request_id: state.ref.id,
    }))
    const solution = await checked(await author.request(submission + '/' + reviewKey + '/solution', 'GET'))
    await checked(await author.request(submission + '/' + reviewKey + '/solution-confirmation', 'POST', {
      import_revision: solution.import_revision, formal_plan_sha256: solution.formal_plan_sha256,
    }))
    const self = await author.request('/api/v1/problem-review/' + r.id + '/eligibility', 'GET')
    assert([403, 404].includes(self.status()))
    await save('OJ_BROWSER_PACKAGE_REVIEW', { id: r.id, formal: report.formal })
    await logout(author.page)
    break
  }
  case 'approve': {
    const { id } = JSON.parse(await readFile(process.env.OJ_BROWSER_PACKAGE_REVIEW, 'utf8'))
    const reviewer = await login(info.other_email, info.other_subject_id)
    const path = '/api/v1/problem-review/' + id + '/eligibility'
    const before = await checked(await reviewer.request(path, 'GET'))
    assert.equal(before.eligible, false)
    assert.deepEqual(before.blockers, ['statement_quality_missing'])
    const page = reviewer.page
    await page.goto(info.url + '/review/materials/' + id)
    await page.getByRole('button', { name: '检查会话并读取材料' }).click()
    const list = page.getByRole('group', { name: '人工核对清单' }).getByRole('checkbox')
    for (let i = 0; i < 4; i++) await list.nth(i).check()
    const [confirmation] = await Promise.all([page.waitForResponse(r => r.url().endsWith('/statement-confirmation')),
      page.getByRole('button', { name: '记录题意确认' }).click()])
    assert.equal(confirmation.status(), 200)
    const after = await checked(await reviewer.request(path, 'GET'))
    assert.equal(after.eligible, true)
    assert.deepEqual(after.blockers, [])
    assert(after.input_validator_evidence.inputs > 0 && after.input_validator_evidence.controls === 4)
    assert(after.independent_correctness_evidence.cases >= 10 && after.test_coverage_evidence.non_sample_criteria > 0)
    assert(after.test_coverage_evidence.wrong_solutions.every(solution => solution.rejected_points > 0))
    await page.getByRole('button', { name: '查看当前阻断项' }).click()
    await page.getByLabel('批准理由').fill('本机题包质量和独立审核已核验')
    const [approval] = await Promise.all([page.waitForResponse(r => r.url().endsWith('/approve')),
      page.getByRole('button', { name: '确认批准', exact: true }).click()])
    assert.equal(approval.status(), 200)
    await save('OJ_BROWSER_PACKAGE_APPROVED', { reviewer: true, statementGate: true, quality: true })
    await logout(page)
    break
  }
  default:
    throw new Error('Unknown package review QA step')
  }
  console.log('PASS package ' + process.env.OJ_BROWSER_PACKAGE_STEP)
} finally { await browser.close() }
