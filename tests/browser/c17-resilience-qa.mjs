import assert from 'node:assert/strict'
import { readFile, writeFile } from 'node:fs/promises'
import { pathToFileURL } from 'node:url'

const info = JSON.parse(await readFile(process.env.OJ_BROWSER_QA_INFO, 'utf8'))
assert.match(info.url, /^https:\/\/127\.0\.0\.1:\d+$/)
const { chromium } = await import(pathToFileURL(process.env.OJ_PLAYWRIGHT_MODULE))
const browser = await chromium.launch({ executablePath: process.env.OJ_CHROMIUM_EXECUTABLE })
const collection = info.url + '/api/v1/training/submissions'
const language = process.env.OJ_BROWSER_TRAINING_LANGUAGE ?? 'c17'
assert(['c17', 'gnu++20', 'java21', 'python3'].includes(language))
const label = { c17: 'C17', 'gnu++20': 'GNU++20', java21: 'Java 21', python3: 'Python 3.14' }[language]
const cpp20Guard = '#if __cplusplus < 202002L\n#error GNU++20 required\n#endif\n'
const correct = '#include <stdio.h>\nint main(void){long long a,b;if(scanf("%lld%lld",&a,&b)!=2)return 1;printf("%lld\\n",a+b);}'
// CPU cost per point keeps both leases observable without changing sandbox limits.
const cost = 'for(volatile unsigned i=0;i<9000000u;++i){}'
const slow = correct.replace('printf(', cost + 'printf(')
const recoverySource = correct.replace('printf(', 'for(volatile unsigned i=0;i<120000000u;++i){}printf(')
const nativeSources = {
  overlap: [[slow, 'AC'], [slow, 'AC']],
  limits: [
    ['int main(void){for(;;){}}', 'TLE'],
    ['#include <stdlib.h>\nint main(void){unsigned n=200u*1024u*1024u;volatile unsigned char *p=malloc(n);if(!p)return 1;for(unsigned i=0;i<n;i+=4096)p[i]=1;return 0;}', 'MLE'],
    ['#include <stdio.h>\nint main(void){for(unsigned i=0;i<400000u;++i)putchar(120);return 0;}', 'OLE'],
  ],
  fault: [[recoverySource, 'AC']],
}
const javaSum = iterations => `import java.util.Scanner;
public class Main {
  public static void main(String[] args) {
    var in = new Scanner(System.in);
    long sum = in.nextLong() + in.nextLong();
    for (int i = 0; i < ${iterations}; i++) sum ^= (i & 1);
    System.out.println(sum);
  }
}`
const javaSources = {
  overlap: [[javaSum(4_000_000), 'AC'], [javaSum(4_000_000), 'AC']],
  fault: [[javaSum(40_000_000), 'AC']],
  limits: [
    // The single array cannot fit within the configured 64 MiB Java heap.
    ['public class Main { public static void main(String[] args) { System.out.println(new byte[80 << 20].length); } }', 'RE'],
    // Retain a bounded number of standard-library zlib buffers to exercise
    // the total-memory cgroup independently of Java heap allocation.
    [`import java.util.zip.Deflater;
import java.util.Random;
public class Main {
  public static void main(String[] args) {
    var input = new byte[32768];
    new Random(123).nextBytes(input);
    var output = new byte[65536];
    var buffers = new Deflater[1100];
    for (int i = 0; i < buffers.length; i++) {
      buffers[i] = new Deflater(1);
      buffers[i].setInput(input);
      buffers[i].finish();
      buffers[i].deflate(output);
    }
    System.out.println(buffers.length);
  }
}`, 'MLE'],
  ],
}
const pythonSum = iterations => `import sys
assert sys.version_info[:2] == (3, 14)
a, b = map(int, sys.stdin.buffer.read().split())
for _ in range(${iterations}):
    pass
print(a + b)
`
const pythonSources = {
  overlap: [[pythonSum(2_000_000), 'AC'], [pythonSum(2_000_000), 'AC']],
  fault: [[pythonSum(12_000_000), 'AC']],
  limits: [
    ['while True:\n    pass\n', 'TLE'],
    ['data = bytearray(144 << 20)\nfor index in range(0, len(data), 4096):\n    data[index] = 1\nprint(len(data))\n', 'MLE'],
    ['import sys\nsys.stdout.buffer.write(b"x" * 400000)\n', 'OLE'],
  ],
}
const sources = language === 'java21' ? javaSources : language === 'python3' ? pythonSources : nativeSources
function sourceForLanguage(source, verdict) {
  if (language !== 'gnu++20') return source
  // C++ requires an explicit conversion from malloc's void pointer.
  const code = verdict === 'MLE' ? source.replace('p=malloc(n)', 'p=static_cast<unsigned char *>(malloc(n))') : source
  return cpp20Guard + code
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
    await page.getByText(label, { exact: true }).waitFor()
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
    for (const [fixture, verdict] of sources[phase]) {
      const source = sourceForLanguage(fixture, verdict)
      await page.goto(info.url + '/problems')
      await page.getByRole('link', { name: 'A + B', exact: true }).click()
      await page.getByLabel('语言', { exact: true }).selectOption(language)
      await page.locator('#training-source:not([readonly])').waitFor()
      await page.getByLabel('源码', { exact: true }).fill(source)
      const [response] = await Promise.all([page.waitForResponse(r => r.url() === collection),
        page.getByRole('button', { name: '提交判题', exact: true }).click()])
      assert.equal(response.status(), 200)
      const receipt = await response.json()
      assert.equal(receipt.state, 'queued')
      assert.equal(receipt.language, language)
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
  console.log('PASS local ' + label + ' browser phase: ' + phase)
} finally {
  await browser.close()
}
