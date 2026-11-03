import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { runJournalChecks } from './journal-qa.mjs'

export async function runManagementChecks(browser, info, ready, password, output) {
  const context = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1280, height: 960 } })
  const page = await context.newPage(); page.setDefaultTimeout(8000)
  const calls = [], errors = [], diagnostics = [], screenshots = [], routeErrors = []
  const base = '/api/v1/problem-draft-management'
  let phase = 'real IndexedDB transactions'
  page.on('request', request => { if (new URL(request.url()).pathname.startsWith(base)) calls.push({ method: request.method(), path: new URL(request.url()).pathname, body: request.postDataJSON() }) })
  page.on('pageerror', error => errors.push(error.name))
  page.on('console', message => { if (['warning', 'error'].includes(message.type())) diagnostics.push(message.type() === 'error' && /Failed to load resource: the server responded with a status of (401|404|409|503)\b/.test(message.text())) })
  const load = async () => { await page.getByRole('button', { name: '检查会话并加载来源与记录', exact: true }).click(); await page.getByRole('button', { name: '导入此修订', exact: true }).waitFor() }
  const posts = () => calls.filter(c => c.method === 'POST')
  const login = async () => {
    await page.goto(info.origin + '/login', { waitUntil: 'networkidle' })
    await page.getByLabel('邮箱', { exact: true }).fill(ready.email); await page.getByLabel('密码', { exact: true }).fill(password)
    await page.getByRole('button', { name: '登录', exact: true }).click(); await page.getByText('登录成功，当前会话已核验。', { exact: true }).waitFor()
  }
  const problem = code => ({ status: 503, contentType: 'application/problem+json', body: JSON.stringify({ code }) })
  const readRecords = () => page.evaluate(async () => {
    const db = await new Promise((resolve, reject) => { const r = indexedDB.open('oj-draft-import-recovery', 1); r.onsuccess = () => resolve(r.result); r.onerror = () => reject(r.error) })
    return new Promise((resolve, reject) => { const tx = db.transaction('records'); const r = tx.objectStore('records').getAll(); tx.oncomplete = () => { db.close(); resolve(r.result) }; tx.onerror = () => { db.close(); reject(tx.error) } })
  })
  const clearRecords = () => page.evaluate(async () => {
    const db = await new Promise(resolve => { const r = indexedDB.open('oj-draft-import-recovery', 1); r.onsuccess = () => resolve(r.result) })
    await new Promise(resolve => { const tx = db.transaction(['records', 'slots'], 'readwrite'); tx.objectStore('records').clear(); tx.objectStore('slots').clear(); tx.oncomplete = resolve }); db.close()
  })
  try {
    const journal = await runJournalChecks(page, info.origin)
    phase = 'real owner import with lost committed response'
    await login(); await page.goto(info.origin + '/author/drafts', { waitUntil: 'networkidle' }); await load()
    assert.equal(posts().length, 0)
    phase = 'inherited Object key is not receipt evidence'
    await page.evaluate(async owner => {
      const db = await new Promise(resolve => { const r = indexedDB.open('oj-draft-import-recovery', 1); r.onsuccess = () => resolve(r.result) })
      const entry = { owner, source: 'fixture:inherited-key', key: 'propertyIsEnumerable', revision: '0'.repeat(64), status: 'pending' }
      await new Promise(resolve => { const tx = db.transaction(['records', 'slots'], 'readwrite'); tx.objectStore('records').put(entry, [owner, entry.key]); tx.objectStore('slots').put({ owner, source: entry.source, key: entry.key }, [owner, entry.source]); tx.oncomplete = resolve }); db.close()
    }, ready.subject_id)
    await load()
    assert.equal(await page.getByText('结果待确认', { exact: true }).count(), 1)
    assert.equal(await page.getByText('服务端回执已核验', { exact: true }).count(), 0)
    assert.equal(await page.getByRole('link', { name: '打开草稿预览', exact: true }).count(), 0)
    await page.getByRole('button', { name: '查询原回执', exact: true }).click()
    await page.getByText('此刻尚未查到回执，不代表原请求已取消。可手动重试同一请求。', { exact: true }).waitFor()
    assert.equal(await page.getByRole('link', { name: '打开草稿预览', exact: true }).count(), 0)
    assert.equal(posts().length, 0); assert.equal((await readRecords())[0].key, 'propertyIsEnumerable')
    await clearRecords(); await load()
    phase = 'real owner import with lost committed response'
    let committed
    const lose = async route => {
      try { const response = await route.fetch(); assert.equal(response.status(), 200); committed = await response.json(); await route.fulfill(problem('DRAFT_UNAVAILABLE')) }
      catch { routeErrors.push('lost-response'); await route.abort().catch(() => {}) }
    }
    await page.route('**' + base + '/imports', lose)
    await page.getByRole('button', { name: '导入此修订', exact: true }).click()
    await page.getByText('结果尚未确认。请重新加载本地记录，再手动查询原回执；不要创建替代请求。', { exact: true }).waitFor()
    assert(committed && posts().length === 1)
    const tuple = posts()[0].body
    let records = await readRecords(); assert.equal(records.length, 1); assert.equal(records[0].status, 'pending'); assert.equal(records[0].key, tuple.request_key)
    await page.unroute('**' + base + '/imports', lose)
    await page.reload({ waitUntil: 'networkidle' }); assert.equal(posts().length, 1)
    await load(); assert.equal(await page.getByRole('link', { name: '打开草稿预览', exact: true }).count(), 0)
    const beforeServerList = await readRecords()
    await page.getByRole('button', { name: '检查会话并查看第一页', exact: true }).click()
    await page.getByRole('region', { name: '服务器上的本人草稿', exact: true }).getByRole('link', { name: '预览此草稿', exact: true }).first().waitFor()
    assert.deepEqual(await readRecords(), beforeServerList, 'Server discovery never reconciles or clears a pending import')
    assert.equal(posts().length, 1)
    await load()
    await page.getByRole('button', { name: '查询原回执', exact: true }).click(); await page.getByRole('link', { name: '打开草稿预览', exact: true }).waitFor()
    assert.equal(posts().length, 1, 'Refresh and recovery use GET only')
    records = await readRecords(); assert.equal(records[0].status, 'confirmed'); assert.equal(records[0].draftId, committed.draft_id)
    await load(); await page.getByRole('button', { name: '导入此修订', exact: true }).click()
    await page.getByText('此修订已导入，回执已核验。', { exact: true }).waitFor()
    assert.equal(posts().length, 1, 'Confirmed same revision is rechecked, not reimported')
    for (const [name, width, height] of [['desktop', 1280, 960], ['mobile', 390, 844]]) {
      await page.setViewportSize({ width, height }); assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
      const image = path.join(output, 'draft-management-' + name + '.png'); await page.screenshot({ path: image, fullPage: true }); screenshots.push(image)
    }
    await page.getByRole('link', { name: '打开草稿预览', exact: true }).click(); await page.getByRole('button', { name: '检查会话并打开草稿', exact: true }).click()
    await page.getByRole('heading', { name: 'Browser statement', exact: true }).waitFor()
    phase = 'pending storage loss prevents memory retry'
    await page.goto(info.origin + '/author/drafts', { waitUntil: 'networkidle' }); await load()
    await page.getByRole('button', { name: '移除本机已确认记录', exact: true }).click(); await page.getByText('已移除此条本机记录；服务端草稿仍保留。', { exact: true }).waitFor()
    await load()
    const beforeQuota = posts().length
    await page.evaluate(() => { window.__originalAdd = IDBObjectStore.prototype.add; IDBObjectStore.prototype.add = function (...args) { if (this.name === 'records') throw new DOMException('fixture', 'QuotaExceededError'); return window.__originalAdd.apply(this, args) } })
    await page.getByRole('button', { name: '导入此修订', exact: true }).click()
    await page.getByText('本地恢复记录不可用、已满或已变化。未发起新的导入；请保留浏览器数据并重新检查。', { exact: true }).waitFor()
    assert.equal(posts().length, beforeQuota, 'Quota failure sends zero POST')
    await page.evaluate(() => { IDBObjectStore.prototype.add = window.__originalAdd; delete window.__originalAdd })
    await load()
    const unavailable = route => route.fulfill(problem('DRAFT_UNAVAILABLE'))
    await page.route('**' + base + '/imports', unavailable)
    await page.getByRole('button', { name: '导入此修订', exact: true }).click(); await page.getByText('结果尚未确认。请重新加载本地记录，再手动查询原回执；不要创建替代请求。', { exact: true }).waitFor()
    await page.unroute('**' + base + '/imports', unavailable)
    await load(); const beforeClear = posts().length; await clearRecords()
    await page.getByRole('button', { name: '重试同一请求', exact: true }).click()
    await page.getByText('本地恢复记录不可用、已满或已变化。未发起新的导入；请保留浏览器数据并重新检查。', { exact: true }).waitFor()
    assert.equal(posts().length, beforeClear)
    phase = 'cookie switch after stale A Read is refused by real B session'
    await load()
    let release, captured, completed
    const gate = new Promise(resolve => { release = resolve }), readCaptured = new Promise(resolve => { captured = resolve }), readCompleted = new Promise(resolve => { completed = resolve })
    const hold = async route => {
      try { const response = await route.fetch(); assert.equal((await response.json()).subject_id, ready.subject_id); captured(); await gate; await route.fulfill({ response }) }
      catch { routeErrors.push('held-read'); await route.abort().catch(() => {}) } finally { completed() }
    }
    await page.route('**/api/v1/session', hold)
    await page.getByRole('button', { name: '导入此修订', exact: true }).click(); await readCaptured
    assert.equal((await context.request.post(info.origin + '/api/v1/session/logout', { headers: { Origin: info.origin } })).status(), 204)
    assert.equal((await context.request.post(info.origin + '/api/v1/session/login/begin', { headers: { Origin: info.origin }, data: { email: ready.other_email, password: ['synthetic', 'protocol', 'password'].join(' ') } })).status(), 202)
    assert.equal((await context.request.post(info.origin + '/api/v1/session/login/complete', { headers: { Origin: info.origin } })).status(), 204)
    release(); await readCompleted
    await page.getByText('浏览器账号已变化。已清除显示，请重新检查当前账号。', { exact: true }).waitFor()
    await page.unroute('**/api/v1/session', hold)
    const raced = posts().at(-1).body
    for (const [method, suffix, data] of [['GET', '/sources'], ['GET', '/drafts'], ['GET', '/imports/' + raced.request_key], ['POST', '/imports', raced]]) {
      const response = await context.request.fetch(info.origin + base + suffix, { method, headers: { Origin: info.origin, 'X-OJ-Expected-Subject': ready.subject_id }, ...(data ? { data } : {}) })
      assert.equal(response.status(), 409); assert.equal((await response.json()).code, 'DRAFT_SUBJECT_CHANGED')
    }
    const bHeaders = { 'X-OJ-Expected-Subject': ready.other_subject_id }
    assert.equal((await context.request.get(info.origin + base + '/sources', { headers: bHeaders })).status(), 200, 'B is an authorized author')
    assert.equal((await context.request.get(info.origin + base + '/imports/' + raced.request_key, { headers: bHeaders })).status(), 404, 'Stale A intent did not create a B draft')
    phase = 'new owner scope remount requires a fresh explicit load'
    const sourcesBeforeB = calls.filter(c => c.path === base + '/sources').length
    await page.getByRole('button', { name: '检查会话并加载来源与记录', exact: true }).click()
    await page.getByText('检查会话后可选择来源，或恢复本机保留的导入。', { exact: true }).waitFor()
    assert.equal(calls.filter(c => c.path === base + '/sources').length, sourcesBeforeB, 'Identity scope remount does not auto load')
    await load(); assert.equal(await page.getByText('此账号在本机没有保留记录。', { exact: true }).count(), 1)
    assert.equal((await readRecords()).length, 1, 'A pending journal is retained but hidden from B')
    assert.equal(errors.length, 0); assert.equal(routeErrors.length, 0); assert(diagnostics.every(Boolean), 'No unexpected console warnings/errors')
    fs.writeFileSync(path.join(output, 'management-qa.json'), JSON.stringify({ success: true, journal, actualCommittedImport: true, lostResponseRefreshGetOnly: true, staleCookieRoutesRefused: 4, pendingUnchangedByServerList: true, storageClearZeroRetryPost: true, screenshots }, null, 2) + '\n')
    return screenshots
  } catch (error) {
    await page.screenshot({ path: path.join(output, 'management-failure.png'), fullPage: true }).catch(() => {})
    await page.locator('main').innerText().then(text => fs.writeFileSync(path.join(output, 'management-failure-dom.txt'), text)).catch(() => {})
    fs.writeFileSync(path.join(output, 'management-failure.json'), JSON.stringify({ phase, error: error.name, message: error instanceof assert.AssertionError ? error.message : 'Browser operation failed', routeErrors: routeErrors.length }, null, 2))
    throw error
  } finally { await context.close() }
}
