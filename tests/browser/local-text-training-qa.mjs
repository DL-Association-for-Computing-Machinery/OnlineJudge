import assert from 'node:assert/strict'
import { readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'

const ready = JSON.parse(await readFile(process.env.OJ_BROWSER_QA_INFO, 'utf8'))
assert.match(ready.url, /^https:\/\/127\.0\.0\.1:\d+$/)
const { chromium } = await import(pathToFileURL(process.env.OJ_PLAYWRIGHT_MODULE))
const browser = await chromium.launch({ executablePath: process.env.OJ_CHROMIUM_EXECUTABLE })
const output = process.env.OJ_BROWSER_QA_OUTPUT
const endpoint = ready.url + '/api/v1/training/submissions'
const title = process.env.OJ_BROWSER_TRAINING_TITLE ?? '并查集'
const ac = `#include <iostream>
#include <numeric>
#include <vector>
int main() {
  std::ios::sync_with_stdio(false); std::cin.tie(nullptr);
  int n,q; std::cin>>n>>q;
  std::vector<int> p(n),size(n,1); std::iota(p.begin(),p.end(),0);
  auto root=[&](int x) { while(x!=p[x]) { p[x]=p[p[x]]; x=p[x]; } return x; };
  while(q--) {
    int t,u,v; std::cin>>t>>u>>v; u=root(u); v=root(v);
    if(t) std::cout<<(u==v)<<'\\n';
    else if(u!=v) { if(size[u]<size[v]) std::swap(u,v); p[v]=u; size[u]+=size[v]; }
  }
}
`
const wa = 'int main() {}\n'
const ole = "#include <iostream>\nint main(){for(int i=0;i<5000;i++) std::cout << '1';}\n"

async function login(context, email) {
  const page = await context.newPage()
  await page.goto(ready.url + '/login')
  await page.getByLabel('邮箱', { exact: true }).fill(email)
  await page.getByLabel('密码', { exact: true }).fill('synthetic protocol password')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await page.getByText('登录成功，当前会话已核验。', { exact: true }).waitFor()
  return page
}

async function logout(page) {
  await page.goto(ready.url + '/account')
  await page.getByRole('button', { name: '退出当前会话', exact: true }).click()
  await page.getByText('已退出当前会话。', { exact: true }).waitFor()
}

try {
  const owner = await browser.newContext({ ignoreHTTPSErrors: true })
  await owner.addInitScript(() => {
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: async text => { window.__copiedSample = text } },
    })
  })
  const page = await login(owner, ready.training_email)
  const errors = []
  page.on('pageerror', e => errors.push(e.name))
  await page.goto(ready.url + '/problems')
  await page.getByRole('link', { name: title, exact: true }).click()
  const problem = page.url()
  const copy = page.getByRole('button', { name: /^复制样例/ }).first()
  const original = await copy.locator('../..').locator('pre').textContent()
  await copy.press('Enter')
  assert.equal(await page.evaluate(() => window.__copiedSample), original)
  const records = []
  const observer = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 390, height: 844 } })
  const other = await login(observer, ready.training_observer_email)
  await other.goto(ready.url + '/submissions')
  const history = other.getByRole('button', { name: '查看提交历史', exact: true })
  const path = url => url.href.startsWith(endpoint) && new URL(url.href).pathname === new URL(endpoint).pathname
  await other.route(path, route => route.fulfill({ status: 503, body: '' }), { times: 1 })
  await history.focus()
  await other.keyboard.press('Tab')
  await other.keyboard.press('Shift+Tab')
  assert(await history.evaluate(button => button.matches(':focus-visible') && getComputedStyle(button).outlineStyle !== 'none'))
  await other.keyboard.press('Enter')
  await other.getByText('暂时无法读取训练提交，请重试第一页。', { exact: true }).waitFor()
  let unblock
  const hold = new Promise(resolve => { unblock = resolve })
  await other.route(path, async route => { await hold; await route.continue() }, { times: 1 })
  await history.click()
  await other.getByText('正在读取训练提交…', { exact: true }).waitFor()
  unblock()
  await other.getByText('没有可显示的训练提交。', { exact: true }).waitFor()
  const sourceAC = process.env.OJ_BROWSER_TRAINING_AC_FILE ? await readFile(process.env.OJ_BROWSER_TRAINING_AC_FILE, 'utf8') : ac
  const variants = [[sourceAC, 'AC']]
  const spj = process.env.OJ_BROWSER_SPJ_ALTERNATE_FILE
  if (spj) {
    variants.push([await readFile(process.env.OJ_BROWSER_SPJ_ALTERNATE_FILE, 'utf8'), 'AC'])
  }
  variants.push([wa, 'WA'])
  if (!spj) variants.push([ole, 'OLE'])
  for (const [source, verdict] of variants) {
    if (records.length) await page.goto(problem)
    await page.getByRole('heading', { name: title, exact: true }).waitFor()
    await page.getByLabel('源码', { exact: true }).fill(source)
    const [response] = await Promise.all([
      page.waitForResponse(r => r.request().method() === 'POST' && r.url() === endpoint),
      page.getByRole('button', { name: '提交判题', exact: true }).click(),
    ])
    assert.equal(response.status(), 200)
    const receipt = await response.json()
    assert.match(receipt.material_version, /^local-text-[a-f0-9]{64}$/)
    if (!records.length) {
      const body = response.request().postDataJSON()
      const headers = { Origin: ready.url, 'X-OJ-Expected-Subject': ready.training_subject_id,
        'Idempotency-Key': 'local-text-negative-version' }
      const invalid = await owner.request.post(endpoint, { headers, data: { ...body, publication_version: '0'.repeat(64) } })
      assert.equal(invalid.status(), 404)
    }
    await page.getByRole('link', { name: '查看本次提交', exact: true }).click()
    await page.getByRole('button', { name: '读取本次提交', exact: true }).click()
    await page.getByText('已判定 · ' + verdict, { exact: true }).waitFor({ timeout: 70_000 })
    assert.equal(await page.getByLabel('提交时的源码').textContent(), source)
    await page.screenshot({ path: join(output, 'local-text-' + records.length + '-' + verdict.toLowerCase() + '.png'), fullPage: true })
    records.push({ id: receipt.id, version: receipt.material_version, verdict })
  }
  await other.goto(page.url())
  await other.getByRole('button', { name: '读取本次提交', exact: true }).click()
  await other.getByText('该提交详情不可用。', { exact: true }).waitFor()
  assert(await other.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
  assert.deepEqual(errors, [])
  await logout(other)
  await logout(page)
  await writeFile(join(output, 'local-text-qa.json'), JSON.stringify({ records, separateAccount: true, wrongVersionRejected: true,
    keyboardFocus: true, sampleCopy: true, historyFailureLoadingEmpty: true, narrowViewport: true }) + '\n')
  console.log('PASS published ' + (spj ? 'SPJ → actual Worker AC, AC and WA' : 'local text → actual Worker AC, WA and OLE') + ' → owner only')
} finally {
  await browser.close()
}
