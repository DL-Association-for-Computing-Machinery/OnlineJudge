import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import fs from 'node:fs'
import { createRequire } from 'node:module'
import path from 'node:path'

const require = createRequire(import.meta.url)
const { chromium } = require(process.env.OJ_PLAYWRIGHT_MODULE)
const output = process.env.OJ_BROWSER_QA_OUTPUT
const info = JSON.parse(fs.readFileSync(process.env.OJ_BROWSER_QA_INFO, 'utf8'))
assert.match(info.origin, /^https:\/\/127\.0\.0\.1:\d+$/)
const ready = JSON.parse(fs.readFileSync(info.log, 'utf8').match(/^BROWSER_READY (.+)$/m)?.[1] ?? 'null')
assert.equal(ready?.url, info.origin)
assert.equal(ready.subject_id, info.subject_id)
for (const email of [ready.email, ready.other_email]) assert.match(email, /^[a-zA-Z0-9._+-]+@example\.invalid$/)
assert.notEqual(ready.email, ready.other_email)
assert.match(ready.other_subject_id, /^[a-f0-9]{32}$/)
assert.notEqual(ready.subject_id, ready.other_subject_id)
const expected = new Map(info.authorized_drafts.map(draft => [draft.source_id, draft]))
assert.equal(expected.size, 10)
const hash = text => createHash('sha256').update(text, 'utf8').digest('hex')
const management = info.origin + '/api/v1/problem-draft-management'
const previewPath = id => '/api/v1/problem-drafts/' + id + '/preview'
const report = { driver: 'existing Playwright', checks: [], problems: [], screenshots: [], viewports: ['1280x960', '390x844'],
  expectedAnonymousSessionDiagnostics: 0 }
const errors = [], offOrigin = [], diagnostics = []
let browser, page, phase = 'launch'

function passed(name) { report.checks.push(name); console.log('PASS ' + name) }

async function author(email) {
  const context = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1280, height: 960 },
    permissions: ['clipboard-read', 'clipboard-write'] })
  await context.route('**/*', route => {
    if (new URL(route.request().url()).origin === info.origin) return route.continue()
    offOrigin.push(true)
    return route.abort()
  })
  const tab = await context.newPage()
  let authenticated = false
  tab.setDefaultTimeout(10000)
  tab.on('pageerror', error => errors.push(error.name))
  tab.on('console', message => {
    if (!['warning', 'error'].includes(message.type())) return
    const status = /^Failed to load resource: the server responded with a status of (\d{3})\b/.exec(message.text())?.[1]
    const sessionResource = message.location().url === info.origin + '/api/v1/session'
    if (!authenticated && message.type() === 'error' && sessionResource && status === '401') {
      report.expectedAnonymousSessionDiagnostics++
      return
    }
    diagnostics.push({ kind: message.type(), status, sessionResource })
  })
  await tab.goto(info.origin + '/login')
  await tab.getByLabel('邮箱', { exact: true }).fill(email)
  await tab.getByLabel('密码', { exact: true }).fill('synthetic protocol password')
  await tab.getByRole('button', { name: '登录', exact: true }).click()
  await tab.getByText('登录成功，当前会话已核验。', { exact: true }).waitFor()
  authenticated = true
  return { context, tab }
}

function privateResponse(response, status) {
  assert.equal(response.status(), status)
  const cache = response.headers()['cache-control']
  assert(cache && cache.split(/[,\n]/).every(directive => directive.trim() === 'no-store'))
  assert.equal(response.headers()['set-cookie'], undefined)
}

async function main() {
  browser = await chromium.launch({ headless: true, args: ['--disable-background-networking'],
    ...(process.env.OJ_CHROMIUM_EXECUTABLE ? { executablePath: process.env.OJ_CHROMIUM_EXECUTABLE } : {}) })
  report.browser = browser.version()
  phase = 'owner login and pinned imports'
  const owner = await author(ready.email)
  page = owner.tab
  const headers = { Origin: info.origin, 'X-OJ-Expected-Subject': ready.subject_id }
  const catalog = await owner.context.request.get(management + '/sources', { headers })
  privateResponse(catalog, 200)
  const { sources } = await catalog.json()
  assert.deepEqual(sources.map(item => item.source_id).sort(), [...expected.keys()].sort())
  const imported = []
  for (const [i, source] of sources.entries()) {
    phase = 'import ' + source.source_id
    const draft = expected.get(source.source_id)
    assert.equal(source.version, draft.version)
    const response = await owner.context.request.post(management + '/imports', { headers, timeout: 65000,
      data: { source_id: source.source_id, request_key: 'authorized-browser-import-' + i, expected_revision: source.import_revision } })
    privateResponse(response, 200)
    const row = await response.json()
    assert.match(row.draft_id, /^[a-f0-9]{32}$/)
    assert.equal(row.version, draft.version)
    assert.equal(row.import_revision, source.import_revision)
    assert.equal(row.state, 'private-draft')
    assert.equal(row.validation_state, 'not-run')
    imported.push({ id: row.draft_id, draft })
  }
  assert.equal(new Set(imported.map(row => row.id)).size, 10)
  await page.goto(info.origin + '/author/drafts')
  const list = page.getByRole('region', { name: '服务器上的本人草稿', exact: true })
  await list.getByRole('button', { name: '检查会话并查看第一页', exact: true }).click()
  await list.getByText('当前第 1 页，每页最多 20 条。', { exact: true }).waitFor()
  const links = list.getByRole('link', { name: '预览此草稿', exact: true })
  assert.deepEqual((await links.evaluateAll(nodes => nodes.map(node => node.getAttribute('href')))).sort(),
    imported.map(row => '/author/drafts/' + row.id).sort())
  await links.and(page.locator(`a[href="/author/drafts/${imported[0].id}"]`)).click()
  passed('ten pinned drafts imported and discovered through the real owner list')

  for (const [i, { id, draft }] of imported.entries()) {
    phase = 'preview ' + draft.source_id
    await page.setViewportSize({ width: 1280, height: 960 })
    if (i !== 0) await page.goto(info.origin + '/author/drafts/' + id)
    assert.equal(new URL(page.url()).pathname, '/author/drafts/' + id)
    assert.match(await page.title(), /OnlineJudge/)
    const article = page.getByRole('article', { name: draft.title, exact: true })
    assert.equal(await article.count(), 0)
    const waiting = page.waitForResponse(response => new URL(response.url()).pathname === previewPath(id) && response.request().method() === 'GET')
    await page.getByRole('button', { name: '检查会话并打开草稿', exact: true }).click()
    const response = await waiting
    privateResponse(response, 200)
    const body = await response.json()
    assert.deepEqual(Object.keys(body).sort(), ['draft_id', 'examples', 'language', 'markdown', 'title', 'version'])
    assert.equal(body.draft_id, id)
    assert.equal(body.title, draft.title)
    assert.equal(body.version, draft.version)
    assert.equal(body.language, 'en')
    assert.equal(hash(body.markdown), draft.markdown_sha256)
    assert.deepEqual(body.examples.map(example => ({ id: example.id, input_sha256: hash(example.input), output_sha256: hash(example.output) })), draft.examples)
    assert(body.examples.every(example => Object.keys(example).sort().join(',') === 'id,input,output'))
    await article.getByText('版本 ' + draft.version + ' · 英文题面 · 私有草稿', { exact: true }).waitFor()
    const statement = article.locator('.statement-markdown')
    assert.deepEqual(await statement.locator('h2').allTextContents(), draft.headings)
    assert.equal(await statement.locator('.katex-error, .math-budget-error, .statement-image-placeholder, img, script').count(), 0)
    const displayedMath = await statement.locator('.katex-display annotation[encoding="application/x-tex"]').allTextContents()
    const formatMath = displayedMath.map(text => text.trim()).filter(text => text.startsWith('\\begin{gathered}')).map(hash)
    assert.deepEqual(formatMath, draft.format_math)
    const formulas = await statement.locator('.katex').count()
    assert(formulas > 0)
    assert.equal(await page.locator('vite-error-overlay').count(), 0)
    const examples = article.getByRole('region', { name: '原始样例', exact: true })
    assert.deepEqual(await examples.locator(':scope > div > h3').allTextContents(), draft.examples.map(example => example.id))
    const copy = async (name, key) => {
      const buttons = examples.getByRole('button', { name, exact: true })
      assert.equal(await buttons.count(), draft.examples.length)
      for (const [n, expectedSample] of draft.examples.entries()) {
        const button = buttons.nth(n)
        await button.click()
        await button.filter({ hasText: '已复制' }).waitFor()
        assert.equal(hash(await page.evaluate(() => navigator.clipboard.readText())), expectedSample[key])
      }
    }
    await copy('复制样例输入', 'input_sha256')
    await copy('复制样例输出', 'output_sha256')
    await page.evaluate(() => document.fonts.ready)
    assert(await page.evaluate(() => document.fonts.check('16px KaTeX_Main')))
    for (const [name, width, height] of [['desktop', 1280, 960], ['mobile', 390, 844]]) {
      await page.setViewportSize({ width, height })
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'No page-level horizontal overflow')
      if (draft.source_id === 'library-checker:convolution_mod') {
        await page.evaluate(() => window.scrollTo(0, 0))
        const screenshot = path.join(output, 'authorized-convolution-' + name + '.png')
        await page.screenshot({ path: screenshot })
        report.screenshots.push(screenshot)
        if (name === 'mobile') {
          await statement.evaluate(element => window.scrollBy(0, element.getBoundingClientRect().top - 135))
          const detail = path.join(output, 'authorized-convolution-mobile-statement.png')
          await page.screenshot({ path: detail })
          report.screenshots.push(detail)
        }
      }
    }
    report.problems.push({ source_id: draft.source_id, version: draft.version, examples: draft.examples.length, formulas,
      format_blocks: formatMath.length })
  }
  assert.equal(report.problems.reduce((count, draft) => count + draft.examples, 0), 18)
  passed('ten real preview responses and eighteen copied sample pairs match the verified bundle')
  phase = 'rendering and resource checks'
  assert.deepEqual(errors, [])
  assert.deepEqual(diagnostics, [])
  assert.deepEqual(offOrigin, [])
  passed('ten statements render formulas at both widths without console errors or external requests')

  phase = 'anonymous and other author'
  const other = await author(ready.other_email)
  const anonymous = await browser.newContext({ ignoreHTTPSErrors: true })
  const unknown = await other.context.request.get(info.origin + previewPath('f'.repeat(32)))
  privateResponse(unknown, 404)
  for (const { id } of imported) {
    const refused = await other.context.request.get(info.origin + previewPath(id))
    privateResponse(refused, 404)
    assert.deepEqual(await refused.json(), await unknown.json())
    privateResponse(await anonymous.request.get(info.origin + previewPath(id)), 401)
  }
  assert.deepEqual(errors, [])
  assert.deepEqual(diagnostics, [])
  assert.deepEqual(offOrigin, [])
  passed('all ten previews reject anonymous and other-author requests without revealing existence')
}

try {
  await main()
  report.success = true
} catch (error) {
  report.success = false
  report.failure = { phase, type: error.name, line: error.stack?.match(/authorized-draft-qa\.mjs:(\d+):/)?.[1], diagnostics }
  // Do not preserve a raw assertion message: it may quote private content.
  if (page) await page.screenshot({ path: path.join(output, 'authorized-draft-failure.png') }).catch(() => {})
  console.error('FAIL ' + phase + ' (' + error.name + ')')
  process.exitCode = 1
} finally {
  if (browser) await browser.close()
  fs.writeFileSync(path.join(output, 'browser-qa.json'), JSON.stringify(report, null, 2) + '\n')
}
