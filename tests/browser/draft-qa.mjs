import assert from 'node:assert/strict'
import path from 'node:path'
import fs from 'node:fs'

export async function runDraftChecks(browser, info, email, password, output) {
  const context = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1280, height: 960 }, permissions: ['clipboard-read', 'clipboard-write'] })
  const page = await context.newPage()
  page.setDefaultTimeout(8000)
  const requests = [], errors = [], diagnostics = [], screenshots = []
  let phase = 'login and real import'
  page.on('request', request => requests.push(request.url()))
  page.on('pageerror', error => errors.push(error.name))
  page.on('console', message => {
    if (['warning', 'error'].includes(message.type())) diagnostics.push({ expected: message.type() === 'error' && /Failed to load resource: the server responded with a status of 401\b/.test(message.text()) })
  })
  await context.route('**/*', route => new URL(route.request().url()).origin === info.origin ? route.continue() : route.abort())
  const open = () => page.getByRole('button', { name: '检查会话并打开草稿', exact: true }).click()
  const displayed = () => page.getByRole('heading', { name: 'Browser statement', exact: true }).waitFor()
  try {
    await page.goto(info.origin + '/login', { waitUntil: 'networkidle' })
    assert(!requests.some(url => /katex|StatementMarkdown/.test(url)), 'Login does not download the lazy renderer')
    await page.getByLabel('邮箱', { exact: true }).fill(email)
    await page.getByLabel('密码', { exact: true }).fill(password)
    await page.getByRole('button', { name: '登录', exact: true }).click()
    await page.getByText('登录成功，当前会话已核验。', { exact: true }).waitFor()
    // This renderer regression retains legacy API coverage. The separate
    // management flow exercises the new subject-pinned import UI.
    const imported = await context.request.post(info.origin + '/api/v1/problem-drafts', { headers: { Origin: info.origin }, data: { source_id: 'library-checker:aplusb', request_key: 'browser-render-draft-1' } })
    assert.equal(imported.status(), 200)
    const draft = await imported.json()
    const url = info.origin + '/author/drafts/' + draft.id
    phase = 'real owner rendering and copy'
    await page.goto(url, { waitUntil: 'networkidle' })
    await open(); await displayed()
    assert.equal(await page.locator('.statement-markdown .katex').count(), 4)
    assert.equal(await page.locator('.statement-markdown table').count(), 1)
    assert.equal(await page.locator('.statement-markdown .katex-error').count(), 1)
    assert.equal(await page.locator('.statement-markdown script, .statement-markdown iframe').count(), 0)
    const authorizedImage = page.getByRole('img', { name: '授权的本题示意图', exact: true })
    await authorizedImage.waitFor()
    assert(await authorizedImage.evaluate(async image => { await image.decode(); return image.naturalWidth === 640 && image.naturalHeight === 160 && image.src.startsWith('data:image/png;base64,') }))
    assert.equal(await page.locator('.statement-markdown img').count(), 1)
    assert.equal(await page.evaluate(() => window.__unsafeStatement), undefined)
    assert(!requests.some(url => !url.startsWith(info.origin)), 'Statement creates zero external requests')
    assert.equal(await page.locator('.statement-markdown a').getAttribute('referrerpolicy'), 'no-referrer')
    await page.evaluate(() => document.fonts.ready)
    assert(await page.evaluate(() => document.fonts.check('16px KaTeX_Main')), 'Bundled math fonts load locally')
    await page.getByRole('button', { name: '复制样例输入', exact: true }).click()
    assert.equal(await page.evaluate(() => navigator.clipboard.readText()), '1 2\n')
    assert.equal(await page.locator('article .sample pre').first().textContent(), '1 2\n')
    for (const [name, width, height] of [['desktop', 1280, 960], ['mobile', 390, 844]]) {
      await page.setViewportSize({ width, height })
      assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'No page horizontal overflow')
      const image = path.join(output, 'private-draft-' + name + '.png')
      await page.screenshot({ path: image, fullPage: true }); screenshots.push(image)
    }
    const endpoint = '**/api/v1/problem-drafts/' + draft.id + '/preview'
    // The real image path above is complete; this substitution separately
    // proves browser decode failures preserve a readable placeholder.
    phase = 'broken image presentation (HTTP substitution)'
    const validPreview = await (await context.request.get(info.origin + '/api/v1/problem-drafts/' + draft.id + '/preview')).json()
    const brokenImage = { ...validPreview.images[0], data: Buffer.from(validPreview.images[0].data, 'base64').subarray(0, 24).toString('base64') }
    await page.route(endpoint, route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ...validPreview, images: [brokenImage] }) }))
    await open()
    await page.getByText('图片无法显示：授权的本题示意图', { exact: true }).waitFor()
    assert.equal(await page.locator('.statement-markdown img').count(), 0)
    await page.unroute(endpoint)
    await open(); await displayed()
    // Late HTTP success after hiding must not revive private content.
    phase = 'late response after pagehide'
    let release, captured, finished
    const gate = new Promise(resolve => { release = resolve })
    const capturedResponse = new Promise(resolve => { captured = resolve })
    const finishedResponse = new Promise(resolve => { finished = resolve })
    await page.route(endpoint, async route => {
      const response = await route.fetch()
      captured()
      await gate
      // The browser may discard the intercepted request after the application's
      // AbortController runs. This is the intentional late-response case.
      try { await route.fulfill({ response }) } catch { /* Already aborted. */ }
      finished()
    })
    await open()
    await page.getByText('正在读取本人草稿…', { exact: true }).waitFor()
    await capturedResponse
    await page.evaluate(() => window.dispatchEvent(new Event('pagehide')))
    release()
    await finishedResponse
    await page.getByText('预览已清除，请重新检查会话并打开草稿。', { exact: true }).waitFor()
    await page.unroute(endpoint)
    assert.equal(await page.locator('#draft-title').count(), 0)
    assert.equal(await page.locator('.statement-markdown img').count(), 0, 'Page invalidation clears private images')
    await open(); await displayed()
    const other = await context.newPage()
    phase = 'cross-tab invalidation without focus'
    await other.goto(info.origin + '/account', { waitUntil: 'networkidle' })
    await page.bringToFront()
    await open(); await displayed()
    const beforeInvalidation = requests.filter(url => url.includes('/api/')).length
    // evaluate(click) deliberately avoids focusing the other tab. Content is
    // confirmed present immediately before logout, so focus cleanup cannot
    // make a broken BroadcastChannel test pass.
    await other.getByRole('button', { name: '退出当前会话', exact: true }).evaluate(button => button.click())
    await other.getByText('已退出当前会话。', { exact: true }).waitFor()
    await page.getByText('预览已清除，请重新检查会话并打开草稿。', { exact: true }).waitFor()
    assert.equal(await page.locator('#draft-title').count(), 0)
    assert.equal(requests.filter(url => url.includes('/api/')).length, beforeInvalidation, 'Invalidation does not authorize an automatic read/reload')
    await other.close()
    await open()
    await page.getByRole('link', { name: '前往登录', exact: true }).waitFor()
    assert.equal(await page.locator('#draft-title').count(), 0)
    // Re-login only for the separate, explicitly mocked rendering corpus. The
    // real owner/API assertions above are complete before these substitutions.
    await page.goto(info.origin + '/login')
    await page.getByLabel('邮箱', { exact: true }).fill(email)
    await page.getByLabel('密码', { exact: true }).fill(password)
    await page.getByRole('button', { name: '登录', exact: true }).click()
    await page.getByText('登录成功，当前会话已核验。', { exact: true }).waitFor()
    await page.goto(url)
    const corpus = [...(info.statements || []),
      { name: 'depth-budget', markdown: '> '.repeat(70) + 'deep' },
      { name: 'fenced-math-budget', markdown: '```math\n' + 'x'.repeat(4097) + '\n```' },
      { name: 'formula-count-budget', markdown: Array.from({ length: 129 }, () => '$x$').join(' ') },
      { name: 'macro-expansion-budget', markdown: '$\\def\\a{\\a}\\a$ then $x+1$' },
      { name: 'output-budget', markdown: Array.from({ length: 10 }, () => '$' + 'x'.repeat(4000) + '$').join(' ') },
      { name: 'input-budget', markdown: 'x'.repeat(131073) },
    ]
    const measurements = []
    for (const item of corpus) {
      phase = 'renderer corpus: ' + item.name
      await page.evaluate(() => {
        window.__renderLongTasks = []
        window.__renderObserver?.disconnect()
        if (PerformanceObserver.supportedEntryTypes.includes('longtask')) {
          window.__renderObserver = new PerformanceObserver(list => window.__renderLongTasks.push(...list.getEntries().map(entry => entry.duration)))
          window.__renderObserver.observe({ type: 'longtask' })
        }
      })
      await page.route(endpoint, route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ draft_id: draft.id, version: draft.version, title: item.name, language: 'en', markdown: item.markdown, examples: [] }) }))
      const started = performance.now()
      await open()
      await page.getByText('当前版本预览。此草稿尚未发布。', { exact: true }).waitFor()
      const metrics = await page.locator('.statement-markdown').evaluate(element => ({ nodes: element.querySelectorAll('*').length, text: element.textContent.length, mathErrors: element.querySelectorAll('.katex-error').length }))
      const longTasks = await page.evaluate(async () => {
        await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))
        return window.__renderLongTasks
      })
      measurements.push({ name: item.name, openMilliseconds: Math.round(performance.now() - started), maxLongTaskMilliseconds: longTasks.length ? Math.round(Math.max(...longTasks)) : 0, ...metrics })
      assert(metrics.nodes < 20000, 'Rendered DOM stays within finite output budget')
      if (item.name === 'macro-expansion-budget') assert.equal(metrics.mathErrors, 1)
      if (item.name.includes('budget') && item.name !== 'macro-expansion-budget') assert(await page.locator('.statement-markdown').innerText().then(text => text.includes('预算')))
      await page.unroute(endpoint)
    }
    fs.writeFileSync(path.join(output, 'statement-render-measurements.json'), JSON.stringify({ source: 'renderer-only HTTP substitution; real owner/API covered separately', measurements }, null, 2) + '\n')
    phase = 'missing local fonts fallback'
    const fontPage = await context.newPage()
    const fontErrors = []
    fontPage.on('pageerror', error => fontErrors.push(error.name))
    await fontPage.route('**/assets/KaTeX*', route => route.abort())
    await fontPage.goto(url)
    await fontPage.getByRole('button', { name: '检查会话并打开草稿', exact: true }).click()
    await fontPage.getByText('数学字体未能加载，可查看原文。', { exact: true }).waitFor()
    await fontPage.getByText('查看原文', { exact: true }).click()
    assert((await fontPage.locator('details pre').textContent()).includes('$x^2$'))
    assert.deepEqual(fontErrors, [], 'Missing fonts do not crash the application')
    const fontShot = path.join(output, 'private-draft-font-fallback.png')
    await fontPage.screenshot({ path: fontShot, fullPage: true }); screenshots.push(fontShot)
    await fontPage.close()
    assert.equal(errors.length, 0, 'No uncaught application errors')
    assert(diagnostics.every(item => item.expected), 'No unexpected console warning/error')
    assert(await page.evaluate(() => localStorage.length === 0 && sessionStorage.length === 0), 'No private content persisted')
    return screenshots
  } catch (error) {
    fs.writeFileSync(path.join(output, 'draft-failure.json'), JSON.stringify({ phase, error: error.name }) + '\n')
    await page.screenshot({ path: path.join(output, 'draft-failure.png'), fullPage: true }).catch(() => {})
    await page.locator('main').innerText().then(text => fs.writeFileSync(path.join(output, 'draft-failure-dom.txt'), text)).catch(() => {})
    throw error
  } finally { await context.close() }
}
