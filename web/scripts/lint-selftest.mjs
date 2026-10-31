import assert from 'node:assert/strict'
import { readdir } from 'node:fs/promises'
import { resolve } from 'node:path'
import { test } from 'node:test'
import { ESLint } from 'eslint'

const eslint = new ESLint()
// Use an existing tsconfig member; lintText keeps fixtures off disk and out of builds.
const filePath = 'src/services/health.ts'

// Matching changes must not silently turn a successful lint into a partial scan.
test('glob lint covers TypeScript sources and preserves output ignores', async () => {
  const sources = (await readdir('src', { recursive: true, withFileTypes: true }))
    .filter(entry => entry.isFile() && /\.tsx?$/.test(entry.name))
    .map(entry => resolve(entry.parentPath, entry.name))
  assert.ok(sources.length > 0)
  const results = await eslint.lintFiles('.')
  const scanned = new Set(results.map(result => result.filePath))
  for (const source of [...sources, resolve('vite.config.ts')]) {
    assert.ok(scanned.has(source), `not scanned: ${source}`)
  }
  for (const ignored of ['dist/nested/output.ts', 'coverage/report.ts', 'node_modules/fixture/index.ts']) {
    assert.equal(await eslint.isPathIgnored(ignored), true, `not ignored: ${ignored}`)
  }
})

test('typed lint accepts handled promises', async () => {
  const [result] = await eslint.lintText(`
    export async function check() {
      await Promise.resolve(1)
    }
  `, { filePath })
  assert.equal(result.errorCount, 0, JSON.stringify(result.messages))
  assert.equal(result.warningCount, 0)
})

for (const [rule, source] of [
  ['no-floating-promises', 'export function check() { Promise.resolve(1) }'],
  ['no-misused-promises', 'export function check() { [1].forEach(async () => { await Promise.resolve() }) }'],
  ['no-unsafe-assignment', 'export const data = JSON.parse("{}")'],
]) {
  test(`typed lint rejects ${rule}`, async () => {
    const [result] = await eslint.lintText(source, { filePath })
    assert.equal(result.fatalErrorCount, 0, JSON.stringify(result.messages))
    assert.ok(result.messages.some(message =>
      message.ruleId === `@typescript-eslint/${rule}` && message.severity === 2),
    JSON.stringify(result.messages))
  })
}

test('Vite config also uses type information', async () => {
  const [result] = await eslint.lintText('Promise.resolve(1)', { filePath: 'vite.config.ts' })
  assert.equal(result.fatalErrorCount, 0, JSON.stringify(result.messages))
  assert.ok(result.messages.some(message => message.ruleId === '@typescript-eslint/no-floating-promises'))
})

test('TSX event handlers reject promise-returning callbacks', async () => {
  const [result] = await eslint.lintText(`
    export function Fixture() {
      return <button onClick={async () => { await Promise.resolve() }}>Check</button>
    }
  `, { filePath: 'src/App.tsx' })
  assert.equal(result.fatalErrorCount, 0, JSON.stringify(result.messages))
  assert.ok(result.messages.some(message => message.ruleId === '@typescript-eslint/no-misused-promises'))
})
