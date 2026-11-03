import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'

export async function runOwnListChecks(browser, info, ready, password, output) {
  const context = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1280, height: 960 } })
  // A fresh profile and denied persistence prove discovery does not rely on the
  // prior browser's import journal. This is a test-only browser substitution.
  await context.addInitScript(() => Object.defineProperty(globalThis, 'indexedDB', { get() { throw new DOMException('fixture', 'SecurityError') } }))
  const page = await context.newPage(); page.setDefaultTimeout(8000)
  const errors = [], diagnostics = [], calls = [], screenshots = []
  page.on('pageerror', error => errors.push(error.name))
  page.on('console', message => { if (['warning', 'error'].includes(message.type())) diagnostics.push(message.type() === 'error' && /Failed to load resource: the server responded with a status of (401|404|503)\b/.test(message.text())) })
  page.on('request', request => { if (new URL(request.url()).pathname.startsWith('/api/')) calls.push({ method: request.method(), path: new URL(request.url()).pathname }) })
  const endpoint = info.origin + '/api/v1/problem-draft-management'
  const headers = { Origin: info.origin, 'X-OJ-Expected-Subject': ready.subject_id }
  let phase = 'login and real owner rows'
  try {
    await page.goto(info.origin + '/login', { waitUntil: 'networkidle' })
    await page.getByLabel('邮箱', { exact: true }).fill(ready.email); await page.getByLabel('密码', { exact: true }).fill(password)
    await page.getByRole('button', { name: '登录', exact: true }).click(); await page.getByText('登录成功，当前会话已核验。', { exact: true }).waitFor()
    const catalog = await context.request.get(endpoint + '/sources', { headers }); assert.equal(catalog.status(), 200)
    const source = (await catalog.json()).sources[0]
    const created = [], requestPrefix = ['browser', 'own', 'list', 'fixture'].join('_')
    for (let n = 0; n < 22; n++) {
      const response = await context.request.post(endpoint + '/imports', { headers, data: { source_id: source.source_id, request_key: requestPrefix + '_' + n, expected_revision: source.import_revision } })
      assert.equal(response.status(), 200); created.push((await response.json()).draft_id)
    }
    const expected = []; let cursor = ''
    do {
      const response = await context.request.get(endpoint + '/drafts' + (cursor ? '?cursor=' + cursor : ''), { headers }); assert.equal(response.status(), 200)
      const data = await response.json(); expected.push(...data.drafts.map(row => row.draft_id)); cursor = data.next_cursor
    } while (cursor)
    assert(created.every(id => expected.includes(id)))
    await page.goto(info.origin + '/author/drafts', { waitUntil: 'networkidle' })
    assert.equal(new URL(page.url()).pathname, '/author/drafts'); assert.match(await page.title(), /ECUT OJ.*OnlineJudge/)
    assert.equal(await page.locator('vite-error-overlay').count(), 0)
    const section = page.getByRole('region', { name: '服务器上的本人草稿', exact: true })
    const first = async () => { await section.getByRole('button', { name: '检查会话并查看第一页', exact: true }).click(); await section.getByText('当前第 1 页，每页最多 20 条。', { exact: true }).waitFor() }
    const ids = () => section.locator('li > div > p > code').allTextContents()
    const before = calls.length
    phase = 'fresh profile with denied IndexedDB finds every server page'
    await first(); assert.equal((await ids()).length, 20)
    const all = await ids(); let number = 1
    while (await section.getByRole('button', { name: '查看下一页', exact: true }).count()) {
      await section.getByRole('button', { name: '查看下一页', exact: true }).click(); number++
      await section.getByText(`当前第 ${number} 页，每页最多 20 条。`, { exact: true }).waitFor(); all.push(...await ids())
    }
    assert.deepEqual(all, expected); assert.equal(new Set(all).size, all.length)
    assert(calls.slice(before).every(c => c.method === 'GET'), 'Discovery makes no POST, including with unavailable IndexedDB')
    const target = await section.getByRole('link', { name: '预览此草稿', exact: true }).first().getAttribute('href')
    assert(target && expected.some(id => target.endsWith(id)))
    await section.getByRole('link', { name: '预览此草稿', exact: true }).first().click()
    await page.getByRole('button', { name: '检查会话并打开草稿', exact: true }).click(); await page.getByRole('heading', { name: 'Browser statement', exact: true }).waitFor()
    phase = 'late next page cannot revive a hidden list'
    await page.goto(info.origin + '/author/drafts', { waitUntil: 'networkidle' }); await first()
    let release, captured, finished
    const gate = new Promise(resolve => { release = resolve }), received = new Promise(resolve => { captured = resolve }), completed = new Promise(resolve => { finished = resolve })
    const pattern = '**/api/v1/problem-draft-management/drafts?cursor=*'
    const hold = async route => { const response = await route.fetch(); captured(); await gate; try { await route.fulfill({ response }) } catch { /* Intentionally aborted by lifecycle cleanup. */ } finally { finished() } }
    await page.route(pattern, hold)
    await section.getByRole('button', { name: '查看下一页', exact: true }).click(); await received
    await page.evaluate(() => window.dispatchEvent(new Event('pagehide'))); release(); await completed
    await section.getByText('列表已清除，请重新检查会话并查看第一页。', { exact: true }).waitFor()
    assert.equal(await section.locator('li').count(), 0)
    await page.unroute(pattern, hold); await first()
    phase = 'desktop and mobile evidence'
    for (const [name, width, height] of [['desktop', 1280, 960], ['mobile', 390, 844]]) {
      await page.setViewportSize({ width, height }); assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
      await page.evaluate(() => window.scrollTo(0, 0))
      const image = path.join(output, 'own-draft-list-' + name + '.png'); await page.screenshot({ path: image, fullPage: true }); screenshots.push(image)
      const viewport = path.join(output, 'own-draft-list-' + name + '-viewport.png'); await page.screenshot({ path: viewport, fullPage: false }); screenshots.push(viewport)
    }
    assert.equal(errors.length, 0); assert(diagnostics.every(Boolean), 'No unexpected console diagnostics')
    fs.writeFileSync(path.join(output, 'own-list-qa.json'), JSON.stringify({ success: true, browserPath: 'Browser plugin not available; existing Playwright', rows: expected.length, pages: number, deniedIndexedDB: true, freshProfile: true, previewedServerDraft: true, latePageDiscarded: true, screenshots }, null, 2) + '\n')
    return screenshots
  } catch (error) {
    await page.screenshot({ path: path.join(output, 'own-list-failure.png'), fullPage: true }).catch(() => {})
    await page.locator('main').innerText().then(text => fs.writeFileSync(path.join(output, 'own-list-failure-dom.txt'), text)).catch(() => {})
    fs.writeFileSync(path.join(output, 'own-list-failure.json'), JSON.stringify({ phase, error: error.name, message: error instanceof assert.AssertionError ? error.message : 'Browser operation failed' }))
    throw error
  } finally {
    await context.request.post(info.origin + '/api/v1/session/logout', { headers: { Origin: info.origin } }).catch(() => {})
    await context.close()
  }
}
