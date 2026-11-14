import assert from 'node:assert/strict'
import { readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'

const ready = JSON.parse(await readFile(process.env.OJ_BROWSER_QA_INFO, 'utf8'))
assert.match(ready.url, /^https:\/\/127\.0\.0\.1:\d+$/)
assert.match(ready.api_pid, /^[1-9]\d*$/)
const out = process.env.OJ_BROWSER_QA_OUTPUT
const { chromium } = await import(pathToFileURL(process.env.OJ_PLAYWRIGHT_MODULE))
const browser = await chromium.launch({ executablePath: process.env.OJ_CHROMIUM_EXECUTABLE })
const endpoint = ready.url + '/api/v1/training/submissions'
const aplusb = '#include <iostream>\nint main(){long long a,b;std::cin>>a>>b;std::cout<<a+b<<"\\n";}\n'
const unionfind = await readFile(process.env.OJ_BROWSER_UNION_FIND_SOURCE, 'utf8')

async function login(email) {
  const context = await browser.newContext({ ignoreHTTPSErrors: true })
  const page = await context.newPage()
  await page.goto(ready.url + '/login')
  await page.getByLabel('邮箱', { exact: true }).fill(email)
  await page.getByLabel('密码', { exact: true }).fill('synthetic protocol password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await page.getByText('登录成功，当前会话已核验。', { exact: true }).waitFor()
  return { page, context }
}

try {
  const { page, context } = await login(ready.training_email)
  const records = []
  for (const [title, source, prefix] of [['A + B', aplusb, 'aplusb-'], ['并查集', unionfind, 'local-text-']]) {
    await page.goto(ready.url + '/problems')
    await page.getByRole('link', { name: title, exact: true }).click()
    await page.getByRole('heading', { name: title, exact: true }).waitFor()
    await page.getByLabel('源码', { exact: true }).fill(source)
    const [response] = await Promise.all([
      page.waitForResponse(r => r.url() === endpoint && r.request().method() === 'POST'),
      page.getByRole('button', { name: '提交判题', exact: true }).click(),
    ])
    assert.equal(response.status(), 200)
    const receipt = await response.json()
    assert.equal(receipt.language, 'gnu++17')
    assert(receipt.material_version.startsWith(prefix))
    assert.equal(receipt.material_version.length, prefix.length + 64)
    const body = response.request().postDataJSON()
    const headers = { Origin: ready.url, 'X-OJ-Expected-Subject': ready.training_subject_id,
      'Idempotency-Key': (await response.request().allHeaders())['idempotency-key'] }
    const replay = await context.request.post(endpoint, { headers, data: body })
    assert.equal(replay.status(), 200)
    assert.equal((await replay.json()).id, receipt.id)
    const stale = await context.request.post(endpoint, { headers: { ...headers, 'Idempotency-Key': 'multi-publication-stale-version' },
      data: { ...body, publication_version: '0'.repeat(64) } })
    assert.equal(stale.status(), 404)
    if (records.length) {
      const conflict = await context.request.post(endpoint, { headers: records[0].headers, data: body })
      assert.equal(conflict.status(), 409)
    }
    records.push({ title, source, receipt, headers, detail: '/submissions?submission=' + receipt.id })
  }
  await writeFile(join(out, 'accepted.json'), JSON.stringify({ apiPID: ready.api_pid,
    receipts: records.map(({ title, receipt }) => ({ title, ...receipt })) }) + '\n')
  for (const [i, record] of records.entries()) {
    await page.goto(ready.url + record.detail)
    await page.getByRole('button', { name: '读取本次提交', exact: true }).click()
    await page.getByText('已判定 · AC', { exact: true }).waitFor({ timeout: 420_000 })
    assert.equal(await page.getByLabel('提交时的源码').textContent(), record.source)
    await page.getByText(record.receipt.material_version, { exact: true }).waitFor()
    await page.screenshot({ path: join(out, 'published-training-' + (i + 1) + '.png') })
  }
  const other = await login(ready.training_observer_email)
  for (const record of records) {
    await other.page.goto(ready.url + record.detail)
    await other.page.getByRole('button', { name: '读取本次提交', exact: true }).click()
    await other.page.getByText('该提交详情不可用。', { exact: true }).waitFor()
  }
  console.log('PASS same API/browser account submitted and read two real ACs; stale version, cross-problem replay and foreign reads refused')
} finally {
  await browser.close()
}
