import assert from 'node:assert/strict'
import { readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'

const ready = JSON.parse(await readFile(process.env.OJ_BROWSER_QA_INFO, 'utf8'))
assert.match(ready.url, /^https:\/\/127\.0\.0\.1:\d+$/)
const { chromium } = await import(pathToFileURL(process.env.OJ_PLAYWRIGHT_MODULE))
const browser = await chromium.launch({ executablePath: process.env.OJ_CHROMIUM_EXECUTABLE })
const collection = ready.url + '/api/v1/training/submissions'
const recovery = ready.url + '/api/v1/training/receipt'
const source = "#include <iostream>\nint main(){long long a,b;std::cin>>a>>b;std::cout<<a+b<<'\\n';}\n"
const output = process.env.OJ_BROWSER_QA_OUTPUT

async function signIn(context, email) {
  const page = await context.newPage()
  await page.goto(ready.url + '/login')
  await page.getByLabel('邮箱', { exact: true }).fill(email)
  await page.getByLabel('密码', { exact: true }).fill('synthetic protocol password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await page.getByText('登录成功，当前会话已核验。', { exact: true }).waitFor()
  return page
}

try {
  const owner = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1280, height: 900 } })
  const other = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 390, height: 844 } })
  const page = await signIn(owner, ready.training_email)
  const errors = []
  page.on('pageerror', error => errors.push(error.message))
  await page.goto(ready.url + '/problems')
  await page.getByRole('link', { name: 'A + B', exact: true }).click()
  await page.getByRole('heading', { name: 'A + B', exact: true }).waitFor()
  await page.getByLabel('源码', { exact: true }).fill(source)
  let accepted, key, intent
  await page.route(collection, async route => {
    assert.equal(route.request().method(), 'POST')
    key = route.request().headers()['idempotency-key']
    intent = route.request().postDataJSON()
    assert.equal(intent.source, source)
    const response = await route.fetch({ maxRetries: 0 })
    assert.equal(response.status(), 200)
    accepted = await response.json()
    assert.equal(accepted.state, 'queued')
    await route.abort()
  }, { times: 1 })
  await page.getByRole('button', { name: '提交判题', exact: true }).click()
  await page.getByText(/尚未确认是否收到提交/).waitFor()
  const [readback] = await Promise.all([
    page.waitForResponse(r => r.url() === recovery),
    page.getByRole('button', { name: '查询原回执', exact: true }).click(),
  ])
  assert.equal(readback.status(), 200)
  assert.equal(readback.request().headers()['idempotency-key'], key)
  assert.equal((await readback.json()).id, accepted.id)
  const headers = { Origin: ready.url, 'X-OJ-Expected-Subject': ready.training_subject_id, 'Idempotency-Key': key }
  const replay = await owner.request.post(collection, { headers, data: intent })
  assert.equal(replay.status(), 200)
  assert.equal((await replay.json()).id, accepted.id)
  assert.equal((await owner.request.post(collection, { headers, data: { ...intent, source: source + '\n' } })).status(), 409)
  assert.equal((await owner.request.post(collection, { headers, data: { ...intent, publication_version: '0'.repeat(64) } })).status(), 404)
  await page.getByRole('link', { name: '查看本次提交', exact: true }).click()
  await page.getByRole('button', { name: '读取本次提交', exact: true }).click()
  await page.getByText('已判定 · AC', { exact: true }).waitFor({ timeout: 60_000 })
  assert.equal(await page.getByLabel('提交时的源码').textContent(), source)
  await page.screenshot({ path: join(output, 'owner-ac.png'), fullPage: true })
  assert.deepEqual(errors, [])

  const observer = await signIn(other, ready.training_observer_email)
  await observer.goto(page.url())
  await observer.getByRole('button', { name: '读取本次提交', exact: true }).click()
  await observer.getByText('该提交详情不可用。', { exact: true }).waitFor()
  const denied = await other.request.get(recovery, { headers: {
    'Idempotency-Key': key, 'X-OJ-Expected-Subject': ready.training_observer_id,
  } })
  assert.equal(denied.status(), 404)
  assert(await observer.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
  await observer.screenshot({ path: join(output, 'other-account.png'), fullPage: true })
  await writeFile(join(output, 'training-qa.json'), JSON.stringify({
    verdict: 'AC', id: accepted.id, publicationVersion: intent.publication_version, materialVersion: accepted.material_version,
    receiptRecovered: true, replaySame: true, sourceConflict: true, wrongVersionRejected: true, ownerOnly: true,
  }) + '\n')
  console.log('PASS published A+B browser → actual Worker AC → owner result; original receipt and account isolation')
} finally {
  await browser.close()
}
