import { afterEach, expect, it, vi } from 'vitest'
import { AccountController } from '../account/controller'
import { decodeReviewMaterials, fetchReviewMaterials } from '../services/reviewMaterials'
import { ReviewMaterialsController } from './materialsController'

const review = 'a'.repeat(32), owner = 'b'.repeat(32), draft = 'c'.repeat(32), hash = 'd'.repeat(64)
const path = `/api/v1/problem-review/${review}/materials`
const session = () => ({ subject_id: owner, created_at: new Date(Date.now() - 1000).toISOString(), idle_expires_at: new Date(Date.now() + 60_000).toISOString(), absolute_deadline: new Date(Date.now() + 120_000).toISOString() })
const material = () => ({ evidence: { review_id: review, draft_id: draft, reference_request_id: 'e'.repeat(32),
  assignment_revision: 1, source_id: 'library-checker:aplusb', content_version: hash, import_revision: hash,
  source_lock_sha256: hash, formal_plan_sha256: hash, profile_sha256: hash, evidence_sha256: hash, formal_verdict: 'AC' },
  statement: { draft_id: draft, version: hash, title: 'Frozen title', language: 'en', markdown: '# Fixed text', examples: [] },
  standard_solution: { source: '<script>not executable</script>', dependencies: [{ name: 'lib/prime.hpp', source: 'fixed header' }] } })

afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers() })

async function setup() {
  vi.stubGlobal('location', { protocol: 'https:' }); vi.stubGlobal('document', { visibilityState: 'visible' })
  const fetch = vi.fn<typeof globalThis.fetch>().mockImplementation(url => Promise.resolve(Response.json(url === '/api/v1/session' ? session() : material())))
  vi.stubGlobal('fetch', fetch)
  const account = new AccountController(); account.connect(); await vi.waitFor(() => expect(account.getSnapshot().busy).toBe(false))
  const controller = new ReviewMaterialsController(account, review); controller.connect()
  return { fetch, account, controller, stop: () => { controller.disconnect(); account.disconnect() } }
}

it('reads only after explicit session verification and binds the same-origin GET to that reviewer', async () => {
  const f = await setup()
  expect(f.fetch.mock.calls.map(([url]) => url)).toEqual(['/api/v1/session'])
  await f.controller.load()
  expect(f.controller.getSnapshot().data).toEqual(material())
  const [url, options] = f.fetch.mock.calls.at(-1) ?? []
  expect(url).toBe(path)
  expect(options).toMatchObject({ method: 'GET', mode: 'same-origin', credentials: 'same-origin', cache: 'no-store', redirect: 'error', referrerPolicy: 'no-referrer' })
  expect(new Headers(options?.headers).get('X-OJ-Expected-Subject')).toBe(owner)
  f.stop()
})

it('does not display partial material for a mismatched review, draft, version, or unknown dependency', () => {
  const m = material()
  const cases = [
    { ...m, evidence: { ...m.evidence, review_id: draft } },
    { ...m, statement: { ...m.statement, draft_id: review } },
    { ...m, statement: { ...m.statement, version: 'f'.repeat(64) } },
    { ...m, standard_solution: { ...m.standard_solution, dependencies: [{ name: '../file', source: 'x' }] } },
    { ...m, standard_solution: { ...m.standard_solution, dependencies: [...m.standard_solution.dependencies, ...m.standard_solution.dependencies] } },
    { ...m, standard_solution: { ...m.standard_solution, source: 'x'.repeat(65_536) } },
  ]
  for (const value of cases) expect(() => decodeReviewMaterials(value, review)).toThrow()
})

it('accepts exactly 64 KiB of combined UTF-8 source and dependencies', () => {
  const m = material()
  const source = '字'.repeat(21_845) // 65,535 UTF-8 bytes.
  const exact = { ...m, standard_solution: { source, dependencies: [{ name: 'lib/prime.hpp', source: 'x' }] } }
  expect(decodeReviewMaterials(exact, review).standard_solution).toEqual(exact.standard_solution)
  expect(() => decodeReviewMaterials({ ...exact, standard_solution: { ...exact.standard_solution, source: source + 'x' } }, review)).toThrow()
})

it('rejects mismatched content type, malformed UTF-8, and oversized streams without partial results', async () => {
  vi.stubGlobal('location', { protocol: 'https:' })
  const fetch = vi.fn<typeof globalThis.fetch>(); vi.stubGlobal('fetch', fetch)
  const signal = new AbortController().signal
  const headers = { 'Content-Type': 'application/json' }
  const cancel = vi.fn()
  const oversized = new ReadableStream<Uint8Array>({
    start(controller) { controller.enqueue(new Uint8Array(7 * 1024 * 1024 + 1)) }, cancel,
  })
  fetch.mockResolvedValueOnce(new Response(JSON.stringify(material()), { headers: { 'Content-Type': 'text/plain' } }))
    .mockResolvedValueOnce(new Response(new Uint8Array([255]), { headers }))
    .mockResolvedValueOnce(new Response(oversized, { headers }))
  for (let i = 0; i < 3; i++) await expect(fetchReviewMaterials(review, owner, signal)).rejects.toThrow()
  expect(cancel).toHaveBeenCalledOnce()
})

it('rejects non-HTTPS and invalid identifiers before I/O', async () => {
  vi.stubGlobal('location', { protocol: 'http:' }); const fetch = vi.fn(); vi.stubGlobal('fetch', fetch)
  const signal = new AbortController().signal
  await expect(fetchReviewMaterials(review, owner, signal)).rejects.toThrow()
  vi.stubGlobal('location', { protocol: 'https:' })
  await expect(fetchReviewMaterials('../review', owner, signal)).rejects.toThrow()
  await expect(fetchReviewMaterials(review, 'invalid', signal)).rejects.toThrow()
  expect(fetch).not.toHaveBeenCalled()
})

it('drops already displayed material when a later access check returns a generic 404', async () => {
  const f = await setup()
  await f.controller.load()
  expect(f.controller.getSnapshot().data).not.toBeNull()
  f.fetch.mockResolvedValueOnce(Response.json(session())).mockResolvedValueOnce(new Response(null, { status: 404 }))
  await f.controller.load()
  expect(f.controller.getSnapshot()).toMatchObject({ data: null, busy: false })
  f.stop()
})

it.each(['invalidation', 'account change', 'route disposal', 'deadline'])('ignores a late authorized response after %s', async event => {
  const f = await setup()
  let release!: (response: Response) => void
  f.fetch.mockResolvedValueOnce(Response.json(session())).mockImplementationOnce(() => new Promise(resolve => { release = resolve }))
  const pending = f.controller.load()
  await vi.waitFor(() => expect(f.controller.getSnapshot().busy).toBe(true))
  if (event === 'invalidation') f.controller.clear()
  if (event === 'account change') {
    f.fetch.mockResolvedValueOnce(Response.json({ ...session(), subject_id: draft }))
    await f.account.run('check')
  }
  if (event === 'route disposal') f.controller.disconnect()
  if (event === 'deadline') { vi.useFakeTimers(); vi.setSystemTime(Date.now() + 61_000) }
  release(Response.json(material())); await pending
  expect(f.controller.getSnapshot().data).toBeNull()
  f.stop()
})
