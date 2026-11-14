import assert from 'node:assert/strict'
import { readFile, writeFile } from 'node:fs/promises'
import { pathToFileURL } from 'node:url'

const info = JSON.parse(await readFile(process.env.OJ_BROWSER_QA_INFO, 'utf8'))
assert.match(info.url, /^https:\/\/127\.0\.0\.1:\d+$/)
const { chromium } = await import(pathToFileURL(process.env.OJ_PLAYWRIGHT_MODULE))
const browser = await chromium.launch({ executablePath: process.env.OJ_CHROMIUM_EXECUTABLE })

try {
  const context = await browser.newContext({ ignoreHTTPSErrors: true })
  const page = await context.newPage()
  await page.goto(info.url + '/login')
  await page.getByLabel('邮箱', { exact: true }).fill(info.email)
  await page.getByLabel('密码', { exact: true }).fill('synthetic protocol password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await page.getByText('登录成功，当前会话已核验。', { exact: true }).waitFor()
  const headers = { Origin: info.url, 'X-OJ-Expected-Subject': info.subject_id }
  const sources = await context.request.get(info.url + '/api/v1/problem-draft-sources', { headers })
  assert.equal(sources.status(), 200)
  const source = (await sources.json()).sources.find(s => s.source_id === info.source_id)
  assert(source)
  const imported = await context.request.post(info.url + '/api/v1/problem-draft-imports', { headers, data: {
    request_key: 'package-rendering-fixture', source_id: source.source_id, expected_revision: source.import_revision,
  } })
  assert.equal(imported.status(), 200)
  const draft = await imported.json()
  await page.goto(info.url + '/author/drafts/' + draft.draft_id)
  await page.getByRole('button', { name: /检查会话|读取题面/ }).first().click()
  await page.locator('.problem-statement .statement-markdown').waitFor()
  const display = page.locator('.problem-statement')
  const samples = display.getByRole('button', { name: /^复制样例/ })
  const examples = (await samples.count()) / 2
  assert(Number.isInteger(examples) && examples > 0)
  const formulas = await display.locator('.katex').count()
  const formatBlocks = await display.locator('.statement-markdown pre').count()
  for (const width of [1280, 390]) {
    await page.setViewportSize({ width, height: 860 })
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
    assert.equal(await display.locator('.katex-error, .statement-image-placeholder, .statement-markdown details').count(), 0)
    assert.equal(await samples.count(), examples * 2)
  }
  const report = { schema: 'oj.local-text-package-rendering.v1', import_revision: source.import_revision,
    desktop: true, narrow: true, examples, formulas, format_blocks: formatBlocks }
  await writeFile(process.env.OJ_BROWSER_RENDERING_FILE, JSON.stringify(report) + '\n', { mode: 0o600, flag: 'wx' })
  console.log('PASS authorized package preview desktop and narrow')
} finally { await browser.close() }
