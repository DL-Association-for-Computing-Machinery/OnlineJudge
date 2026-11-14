import assert from 'node:assert/strict'
import { readFile, writeFile } from 'node:fs/promises'
import { pathToFileURL } from 'node:url'

const info = JSON.parse(await readFile(process.env.OJ_BROWSER_QA_INFO, 'utf8'))
assert.match(info.url, /^https:\/\/127\.0\.0\.1:\d+$/)
const { chromium } = await import(pathToFileURL(process.env.OJ_PLAYWRIGHT_MODULE))
const browser = await chromium.launch({ executablePath: process.env.OJ_CHROMIUM_EXECUTABLE })
const collection = info.url + '/api/v1/training/submissions'
const correct = '#include <stdio.h>\nint main(void){long long a,b;if(scanf("%lld%lld",&a,&b)!=2)return 1;printf("%lld\\n",a+b);}'
// CPU cost per point keeps both leases observable without changing sandbox limits.
const cost = 'for(volatile unsigned i=0;i<9000000u;++i){}'
const slow = correct.replace('printf(', cost + 'printf(')
const recoverySource = correct.replace('printf(', 'for(volatile unsigned i=0;i<120000000u;++i){}printf(')
const sources = {
  overlap: [[slow, 'AC'], [slow, 'AC']],
  limits: [
    ['int main(void){for(;;){}}', 'TLE'],
    ['#include <stdlib.h>\nint main(void){unsigned n=200u*1024u*1024u;volatile unsigned char *p=malloc(n);if(!p)return 1;for(unsigned i=0;i<n;i+=4096)p[i]=1;return 0;}', 'MLE'],
    ['#include <stdio.h>\nint main(void){for(unsigned i=0;i<400000u;++i)putchar(120);return 0;}', 'OLE'],
  ],
  fault: [[recoverySource, 'AC']],
}
const phase = process.env.OJ_C17_PHASE
const output = process.env.OJ_BROWSER_QA_OUTPUT
assert(Object.hasOwn(sources, phase) || phase === 'read')

async function readResult(context, id, expected, code) {
  const page = await context.newPage()
  try {
    await page.goto(info.url + '/submissions?submission=' + id)
    await page.getByRole('button', { name: '读取本次提交', exact: true }).click()
    await page.getByText('已判定 · ' + expected, { exact: true }).waitFor({ timeout: 85_000 })
    await page.getByText('C17', { exact: true }).waitFor()
    assert.equal(await page.getByLabel('提交时的源码').textContent(), code)
  } finally {
    await page.close()
  }
}

try {
  const context = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 390, height: 844 } })
  const page = await context.newPage()
  await page.goto(info.url + '/login')
  await page.getByLabel('邮箱', { exact: true }).fill(info.training_email)
  await page.getByLabel('密码', { exact: true }).fill('synthetic protocol password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await page.getByText('登录成功，当前会话已核验。', { exact: true }).waitFor()
  if (phase === 'read') {
    const rows = JSON.parse(await readFile(output, 'utf8'))
    assert(Array.isArray(rows) && rows.length > 0)
    for (const row of rows) {
      assert.match(row.id, /^[a-f0-9]{32}$/)
      await readResult(context, row.id, row.verdict, row.source)
    }
  } else {
    const rows = []
    for (const [source, verdict] of sources[phase]) {
      await page.goto(info.url + '/problems')
      await page.getByRole('link', { name: 'A + B', exact: true }).click()
      await page.getByLabel('语言', { exact: true }).selectOption('c17')
      await page.locator('#training-source:not([readonly])').waitFor()
      await page.getByLabel('源码', { exact: true }).fill(source)
      const [response] = await Promise.all([page.waitForResponse(r => r.url() === collection),
        page.getByRole('button', { name: '提交判题', exact: true }).click()])
      assert.equal(response.status(), 200)
      const receipt = await response.json()
      assert.equal(receipt.state, 'queued')
      assert.equal(receipt.language, 'c17')
      rows.push({ id: receipt.id, verdict, source })
    }
    await writeFile(output, JSON.stringify(rows) + '\n', { mode: 0o600, flag: 'wx' })
    if (phase !== 'fault') {
      for (const row of rows) await readResult(context, row.id, row.verdict, row.source)
    }
  }
  await page.goto(info.url + '/account')
  await page.getByRole('button', { name: '退出当前会话', exact: true }).click()
  await page.getByText('已退出当前会话。', { exact: true }).waitFor()
  console.log('PASS local C17 browser phase: ' + phase)
} finally {
  await browser.close()
}
