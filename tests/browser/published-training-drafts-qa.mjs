import assert from 'node:assert/strict'
import { readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'

const ready = JSON.parse(await readFile(process.env.OJ_BROWSER_QA_INFO, 'utf8'))
assert.match(ready.url, /^https:\/\/127\.0\.0\.1:\d+$/)
const { chromium } = await import(pathToFileURL(process.env.OJ_PLAYWRIGHT_MODULE))
const browser = await chromium.launch({ executablePath: process.env.OJ_CHROMIUM_EXECUTABLE })
const output = process.env.OJ_BROWSER_QA_OUTPUT

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

try {
  const context = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 390, height: 844 } })
  let posts = 0
  const errors = []
  context.on('page', page => page.on('pageerror', error => errors.push(error.name)))
  context.on('request', r => { if (r.method() === 'POST' && r.url().endsWith('/training/submissions')) posts++ })
  const page = await context.newPage()
  await login(page, ready.training_email)
  await page.goto(ready.url + '/problems')
  await page.getByRole('link', { name: 'A + B', exact: true }).click()
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

  await other.goto(ready.url + '/account')
  await other.getByRole('button', { name: '退出当前会话', exact: true }).click()
  await other.getByText('已退出当前会话。', { exact: true }).waitFor()
  assert.equal(await editor(page).inputValue().catch(() => ''), '')
  await login(other, ready.training_observer_email)
  await other.goto(problem)
  await readyEditor(other)
  assert.equal(await editor(other).inputValue(), '')
  assert.equal(posts, 0)
  assert.deepEqual(errors, [])
  await writeFile(join(output, 'training-drafts-qa.json'), JSON.stringify({
    reload: true, competingTabs: true, explicitResolution: true, logout: true, separateAccount: true, submissionPosts: posts,
  }) + '\n')
  console.log('PASS published training draft restore, conflict resolution, and account isolation')
} finally {
  await browser.close()
}
