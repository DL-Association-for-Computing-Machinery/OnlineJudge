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
const language = process.env.OJ_BROWSER_TRAINING_LANGUAGE ?? 'gnu++17'
assert(['c17', 'gnu++17', 'gnu++20', 'java21', 'python3'].includes(language))
const c17 = language === 'c17'
const gnu20 = language === 'gnu++20'
const java = language === 'java21'
const python = language === 'python3'
const nonDefault = language !== 'gnu++17'
const extension = python ? 'py' : java ? 'java' : c17 ? 'c' : 'cpp'
const label = { 'gnu++17': 'GNU++17', c17: 'C17', 'gnu++20': 'GNU++20', java21: 'Java 21', python3: 'Python 3.14' }[language]
const cSource = "#include <stdio.h>\nint main(void){long long a,b;return scanf(\"%lld%lld\",&a,&b)==2 ? (printf(\"%lld\\n\",a+b),0):1;}\n"
const cppSource = "#include <iostream>\nint main(){long long a,b;std::cin>>a>>b;std::cout<<a+b<<'\\n';}\n"
const cpp20Source = "#include <concepts>\n#include <iostream>\ntemplate<std::integral T> T sum(T a, T b) { return a+b; }\nint main(){long long a,b;std::cin>>a>>b;std::cout<<sum(a,b)<<'\\n';}\n"
const javaSource = 'import java.io.*;\nimport java.util.*;\npublic class Main {\n record Pair(long a, long b) { long sum() { return a+b; } }\n public static void main(String[] args) throws IOException {\n  var in = new BufferedReader(new InputStreamReader(System.in));\n  var t = new StringTokenizer(in.readLine());\n  System.out.println(new Pair(Long.parseLong(t.nextToken()), Long.parseLong(t.nextToken())).sum());\n }\n}\n'
const pythonSource = 'import sys\nassert sys.version_info[:3] == (3, 14, 10)\na, b = map(int, sys.stdin.buffer.read().split())\nprint(a+b)\n'
const source = python ? pythonSource : java ? javaSource : c17 ? cSource : gnu20 ? cpp20Source : cppSource
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
  if (nonDefault) await page.getByLabel('语言', { exact: true }).selectOption(language)
  const editor = page.getByLabel('源码', { exact: true })
  await page.locator('#training-source:not([readonly])').waitFor()
  const picker = page.getByLabel('选择源码文件', { exact: true })
  await picker.setInputFiles({ name: 'invalid.zip', mimeType: 'application/zip', buffer: Buffer.from('invalid') })
  const wrongType = { py: '请选择 .py 文件。', java: '请选择 .java 文件。', c: '请选择 .c 文件。', cpp: '请选择 .cpp、.cc 或 .cxx 文件。' }
  await page.getByText(wrongType[extension], { exact: true }).waitFor()
  await picker.setInputFiles({ name: 'large.' + extension, mimeType: 'text/plain', buffer: Buffer.alloc(65_537, 'x') })
  await page.getByText('源文件不能超过 64 KiB。', { exact: true }).waitFor()
  await editor.fill('// keep until explicitly replaced')
  await picker.setInputFiles({ name: 'answer.' + extension, mimeType: 'text/plain', buffer: Buffer.from(source) })
  assert.equal(await editor.inputValue(), '// keep until explicitly replaced')
  await page.getByRole('button', { name: '保留当前源码', exact: true }).click()
  assert.equal(await editor.inputValue(), '// keep until explicitly replaced')
  await picker.setInputFiles({ name: 'answer.' + extension, mimeType: 'text/plain', buffer: Buffer.from(source) })
  await page.getByRole('button', { name: '替换编辑区内容', exact: true }).click()
  await page.getByText('已载入源码文件，确认后可提交。', { exact: true }).waitFor()
  assert.equal(await editor.inputValue(), source)
  await page.getByText('训练草稿已保存到此浏览器。', { exact: true }).waitFor()
  // The request never reaches the API. After reload a 404 must not trigger
  // a new POST, and the user can explicitly discard the uncertain key.
  await page.route(collection, route => route.abort(), { times: 1 })
  await page.getByRole('button', { name: '提交判题', exact: true }).click()
  await page.getByText(/尚未确认是否收到提交/).waitFor()
  await page.reload()
  await page.getByRole('button', { name: '查询原回执', exact: true }).click()
  await page.getByText(/原请求可能仍在处理/).waitFor()
  await page.getByRole('button', { name: '清除本地回执', exact: true }).click()
  await page.getByRole('button', { name: '确认清除回执', exact: true }).click()
  await page.getByText(/已清除此标签页的原回执/).waitFor()
  if (nonDefault) {
    assert.equal(await editor.inputValue(), '')
    await page.getByLabel('语言', { exact: true }).selectOption(language)
    await page.locator('#training-source:not([readonly])').waitFor()
  }
  await page.getByText('训练草稿已保存到此浏览器。', { exact: true }).waitFor()
  await picker.setInputFiles({ name: 'answer.' + extension, mimeType: 'text/plain', buffer: Buffer.from(source) })
  await page.getByRole('button', { name: '替换编辑区内容', exact: true }).click()
  await page.getByText('已载入源码文件，确认后可提交。', { exact: true }).waitFor()
  await page.getByText('训练草稿已保存到此浏览器。', { exact: true }).waitFor()
  let accepted, key, intent
  await page.route(collection, async route => {
    assert.equal(route.request().method(), 'POST')
    key = route.request().headers()['idempotency-key']
    intent = route.request().postDataJSON()
    assert.equal(intent.source, source)
    assert.equal(intent.language, language)
    const response = await route.fetch({ maxRetries: 0 })
    assert.equal(response.status(), 200)
    accepted = await response.json()
    assert.equal(accepted.state, 'queued')
    assert.equal(accepted.language, language)
    await route.abort()
  }, { times: 1 })
  await page.getByRole('button', { name: '提交判题', exact: true }).click()
  await page.getByText(/尚未确认是否收到提交/).waitFor()
  const slots = await page.evaluate(() => Object.entries(sessionStorage).filter(([name]) => name.startsWith('oj.training-receipt.v1:')))
  assert.equal(slots.length, 1)
  assert.equal(slots[0][1], key)
  let additionalPosts = 0
  page.on('request', r => { if (r.method() === 'POST' && r.url() === collection) additionalPosts++ })
  await page.reload()
  await page.getByText('有一笔提交尚未确认。请查询原回执。', { exact: true }).waitFor()
  if (!nonDefault) {
    await page.getByText('训练草稿已保存到此浏览器。', { exact: true }).waitFor()
    assert.equal(await editor.inputValue(), source)
  }
  assert.equal(await page.getByRole('button', { name: '重试原提交', exact: true }).count(), 0)
  const [readback] = await Promise.all([
    page.waitForResponse(r => r.url() === recovery),
    page.getByRole('button', { name: '查询原回执', exact: true }).click(),
  ])
  assert.equal(readback.status(), 200)
  assert.equal(readback.request().headers()['idempotency-key'], key)
  assert.equal((await readback.json()).id, accepted.id)
  assert.equal(additionalPosts, 0)
  const headers = { Origin: ready.url, 'X-OJ-Expected-Subject': ready.training_subject_id, 'Idempotency-Key': key }
  const replay = await owner.request.post(collection, { headers, data: intent })
  assert.equal(replay.status(), 200)
  assert.equal((await replay.json()).id, accepted.id)
  if (!nonDefault) {
    const legacy = await owner.request.post(collection, { headers, data: {
      problem_id: intent.problem_id, publication_version: intent.publication_version, source: intent.source,
    } })
    assert.equal(legacy.status(), 200)
    assert.equal((await legacy.json()).id, accepted.id)
  }
  assert.equal((await owner.request.post(collection, { headers, data: { ...intent, source: source + '\n' } })).status(), 409)
  assert.equal((await owner.request.post(collection, { headers, data: { ...intent, language: c17 ? 'gnu++17' : 'c17' } })).status(), 409)
  for (const invalid of ['c++20', '', null])
    assert.equal((await owner.request.post(collection, { headers, data: { ...intent, language: invalid } })).status(), 400)
  assert.equal((await owner.request.post(collection, { headers, data: { ...intent, publication_version: '0'.repeat(64) } })).status(), 404)
  await page.getByRole('link', { name: '查看本次提交', exact: true }).click()
  await page.getByRole('button', { name: '读取本次提交', exact: true }).click()
  await page.getByText('已判定 · AC', { exact: true }).waitFor({ timeout: 60_000 })
  await page.getByText(label, { exact: true }).first().waitFor()
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
  const verdicts = [{ id: accepted.id, verdict: 'AC' }]
  if (nonDefault) {
    const detailURL = page.url()
    const cases = [['WA', source.replace('a+b', 'a+b+1')],
      ['CE', python ? 'if:\n    pass\n' : java ? 'public class Main { INVALID }' : 'int main(void) { return ; }', language]]
    if (python) cases.push(['RE', 'raise RuntimeError("synthetic runtime failure")\n'])
    if (gnu20) cases.push(['CE', source, 'gnu++17'])
    for (const [verdict, code, compiler = language] of cases) {
      await page.goto(ready.url + '/problems')
      await page.getByRole('link', { name: 'A + B', exact: true }).click()
      await page.getByLabel('语言', { exact: true }).selectOption(compiler)
      await page.locator('#training-source:not([readonly])').waitFor()
      await editor.fill(code)
      const [post] = await Promise.all([page.waitForResponse(r => r.url() === collection),
        page.getByRole('button', { name: '提交判题', exact: true }).click()])
      assert.equal(post.status(), 200)
      const receipt = await post.json()
      assert.equal(receipt.language, compiler)
      await page.getByRole('link', { name: '查看本次提交', exact: true }).click()
      await page.getByRole('button', { name: '读取本次提交', exact: true }).click()
      const row = page.getByRole('listitem').filter({ hasText: receipt.id })
      await row.getByText('已判定 · ' + verdict, { exact: true }).waitFor({ timeout: 60_000 })
      assert.equal(await page.getByLabel('提交时的源码').textContent(), code)
      verdicts.push({ id: receipt.id, verdict, language: compiler })
    }
    assert.notEqual(page.url(), detailURL)
  }
  await writeFile(join(output, 'training-qa.json'), JSON.stringify({
    verdict: 'AC', id: accepted.id, language, verdicts, publicationVersion: intent.publication_version, materialVersion: accepted.material_version,
    receiptRecoveredAfterReload: true, abandonedUnsentKey: true, uploadValidation: true, replaySame: true, sourceConflict: true,
    wrongVersionRejected: true, ownerOnly: true,
  }) + '\n')
  for (const active of [observer, page]) {
    await active.goto(ready.url + '/account')
    await active.getByRole('button', { name: '退出当前会话', exact: true }).click()
    await active.getByText('已退出当前会话。', { exact: true }).waitFor()
  }
  console.log('PASS published A+B ' + label + ' browser → actual Worker → owner result; original receipt and account isolation')
} finally {
  await browser.close()
}
