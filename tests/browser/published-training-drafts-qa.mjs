import assert from 'node:assert/strict'
import { readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'

const ready = JSON.parse(await readFile(process.env.OJ_BROWSER_QA_INFO, 'utf8'))
assert.match(ready.url, /^https:\/\/127\.0\.0\.1:\d+$/)
const { chromium } = await import(pathToFileURL(process.env.OJ_PLAYWRIGHT_MODULE))
const browser = await chromium.launch({ executablePath: process.env.OJ_CHROMIUM_EXECUTABLE })
const output = process.env.OJ_BROWSER_QA_OUTPUT
const title = process.env.OJ_BROWSER_TRAINING_TITLE ?? 'A + B'

const editor = page => page.getByLabel('源码', { exact: true })
const saved = page => page.getByText('训练草稿已保存到此浏览器。', { exact: true }).waitFor()
const readyEditor = page => page.waitForFunction(() => {
  const field = document.querySelector('#training-source')
  return field instanceof HTMLTextAreaElement && !field.readOnly
})
async function login(page, email) {
  await page.goto(ready.url + '/login')
  await page.getByLabel('邮箱', { exact: true }).fill(email)
  await page.getByLabel('密码', { exact: true }).fill('synthetic protocol password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await page.getByText('登录成功，当前会话已核验。', { exact: true }).waitFor()
}
async function logout(page) {
  await page.goto(ready.url + '/account')
  await page.getByRole('button', { name: '退出当前会话', exact: true }).click()
  await page.getByText('已退出当前会话。', { exact: true }).waitFor()
}

try {
  const context = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 390, height: 844 } })
  let posts = 0
  const errors = []
  context.on('page', page => page.on('pageerror', error => errors.push(error.name)))
  context.on('request', r => { if (r.method() === 'POST' && r.url().endsWith('/training/submissions')) posts++ })
  const page = await context.newPage()
  await login(page, ready.training_email)
  await page.goto(ready.url + '/problems')
  await page.getByRole('link', { name: title, exact: true }).click()
  await readyEditor(page)
  const problem = page.url()
  await editor(page).fill('// first browser draft')
  await saved(page)
  await page.reload()
  await saved(page)
  assert.equal(await editor(page).inputValue(), '// first browser draft')

  const other = await context.newPage()
  await other.goto(problem)
  await saved(other)
  assert.equal(await editor(other).inputValue(), '// first browser draft')
  await page.bringToFront()
  await readyEditor(page)
  await editor(page).fill('// saved in first tab')
  await saved(page)
  await other.bringToFront()
  await readyEditor(other)
  await editor(other).fill('// different edit from second tab')
  await other.getByText(/另一个标签页已保存不同版本/).waitFor()
  await other.getByRole('button', { name: '保留当前编辑', exact: true }).click()
  await saved(other)
  await other.reload()
  await saved(other)
  assert.equal(await editor(other).inputValue(), '// different edit from second tab')
  assert(await other.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
  await other.screenshot({ path: join(output, 'training-draft-mobile.png'), fullPage: true })

  await logout(other)
  assert.equal(await editor(page).inputValue().catch(() => ''), '')
  await login(other, ready.training_observer_email)
  await other.goto(problem)
  await readyEditor(other)
  assert.equal(await editor(other).inputValue(), '')

  // Fill the existing account ledger through real IndexedDB transactions,
  // keeping the selected problem's key free so the next save needs a new slot.
  await other.evaluate(async owner => {
    const open = indexedDB.open('oj-local-code-drafts', 1)
    const db = await new Promise((resolve, reject) => {
      open.onsuccess = () => resolve(open.result)
      open.onerror = () => reject(open.error)
    })
    try {
      const tx = db.transaction('code', 'readwrite')
      const done = new Promise((resolve, reject) => {
        tx.oncomplete = resolve
        tx.onabort = () => reject(tx.error)
      })
      for (let i = 0; i < 32; i++) {
        tx.objectStore('code').put({ code: '// capacity fixture', revision: 'r' + i },
          [owner, 'capacity-fixture', String(i), 'gnu++17', 'main'])
      }
      await done
    } finally { db.close() }
  }, ready.training_observer_id)
  const unsaved = '// keep this when local storage is full'
  await editor(other).fill(unsaved)
  await other.getByText('本地草稿未确认保存。可下载当前代码后重试。', { exact: true }).waitFor()
  assert.equal(await editor(other).inputValue(), unsaved)
  await other.getByRole('button', { name: '下载当前草稿' }).waitFor()

  const blocked = await browser.newContext({ ignoreHTTPSErrors: true })
  await blocked.addInitScript(() => {
    const open = IDBFactory.prototype.open
    IDBFactory.prototype.open = function (name, version) {
      if (name !== 'oj-local-code-drafts') return open.call(this, name, version)
      // The application's upgrade to v1 is held by this isolated v0 fixture
      // only at the request boundary; no real profile or stored data is touched.
      const request = new EventTarget()
      queueMicrotask(() => request.dispatchEvent(new Event('blocked')))
      return request
    }
  })
  const blockedPage = await blocked.newPage()
  await login(blockedPage, ready.training_email)
  await blockedPage.goto(problem)
  await blockedPage.getByText('本地草稿未确认保存。可下载当前代码后重试。', { exact: true }).waitFor()
  await editor(blockedPage).fill('// retained during blocked upgrade')
  await blockedPage.getByRole('button', { name: '下载当前草稿' }).waitFor()
  assert.equal(await editor(blockedPage).inputValue(), '// retained during blocked upgrade')
  await logout(blockedPage)
  await blocked.close()
  await logout(other)

  const expiring = await browser.newContext({ ignoreHTTPSErrors: true })
  const expiryPage = await expiring.newPage()
  await expiryPage.clock.install()
  await login(expiryPage, ready.training_email)
  await expiryPage.goto(problem)
  await readyEditor(expiryPage)
  const expiryCode = '// restore after the session credential expires'
  await editor(expiryPage).fill(expiryCode)
  await saved(expiryPage)
  await expiryPage.clock.fastForward(3 * 60 * 60 * 1000)
  await expiryPage.waitForFunction(() => !document.querySelector('#training-source') || document.querySelector('#training-source').value === '')
  // Simulate the expired credential being absent on the next navigation.
  // Never call logout here: logout deliberately clears this account's drafts.
  await expiring.clearCookies({ name: '__Host-oj_session' })
  await expiryPage.clock.setSystemTime(new Date())
  await login(expiryPage, ready.training_email)
  await expiryPage.goto(problem)
  await saved(expiryPage)
  assert.equal(await editor(expiryPage).inputValue(), expiryCode)
  await logout(expiryPage)
  await expiring.close()
  assert.equal(posts, 0)
  assert.deepEqual(errors, [])
  await writeFile(join(output, 'training-drafts-qa.json'), JSON.stringify({
    reload: true, competingTabs: true, explicitResolution: true, logout: true, separateAccount: true,
    capacityRetainsSource: true, blockedUpgradeRetainsSource: true, expiryAndSameAccountRestore: true, submissionPosts: posts,
  }) + '\n')
  console.log('PASS published training draft restore, conflict resolution, and account isolation')
} finally {
  await browser.close()
}
