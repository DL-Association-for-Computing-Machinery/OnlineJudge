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
  const journal = new Map<string, string>()
  vi.stubGlobal('sessionStorage', {
    getItem: (slot: string) => journal.get(slot) ?? null,
    setItem: (slot: string, key: string) => { journal.set(slot, key) },
  })
  const fetch = vi.fn<typeof globalThis.fetch>().mockImplementation(url => Promise.resolve(Response.json(url === '/api/v1/session' ? session() : material())))
  vi.stubGlobal('fetch', fetch)
  const account = new AccountController(); account.connect(); await vi.waitFor(() => expect(account.getSnapshot().busy).toBe(false))
  const controller = new ReviewMaterialsController(account, review); controller.connect()
  return { fetch, account, controller, journal, stop: () => { controller.disconnect(); account.disconnect() } }
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

const eligibility = () => ({ evidence: material().evidence, policy_version: 'oj.review-approval.v14', eligible: false,
  blockers: ['statement_quality_missing', 'target_runtime_missing'] })

it('reads current blockers only on request and discards a response for another assignment revision', async () => {
  const f = await setup()
  try {
    await f.controller.load()
    expect(f.controller.getSnapshot().eligibility).toBeNull()
    f.fetch.mockResolvedValueOnce(Response.json(eligibility()))
    await f.controller.readEligibility()
    const [url, options] = f.fetch.mock.calls.at(-1) ?? []
    expect(url).toBe(`/api/v1/problem-review/${review}/eligibility`)
    expect(new Headers(options?.headers).get('X-OJ-Expected-Subject')).toBe(owner)
    expect(f.controller.getSnapshot().eligibility?.blockers).toEqual(eligibility().blockers)
    f.fetch.mockResolvedValueOnce(Response.json({ ...eligibility(), evidence: { ...material().evidence, assignment_revision: 2 } }))
    await f.controller.readEligibility()
    expect(f.controller.getSnapshot()).toMatchObject({ data: null, eligibility: null, busy: false })
  } finally { f.stop() }
})

it.each([
  { formal_plan_sha256: 'f'.repeat(64) }, { profile_sha256: 'f'.repeat(64) },
  { source_lock_sha256: 'f'.repeat(64) }, { formal_verdict: 'WA' },
  { comparison_plan_sha256: 'f'.repeat(64), comparison_outcome: 'passed' },
])('clears materials when a blocker response changes frozen evidence: %o', async changed => {
  const f = await setup()
  try {
    await f.controller.load()
    f.fetch.mockResolvedValueOnce(Response.json({ ...eligibility(), evidence: { ...material().evidence, ...changed } }))
    await f.controller.readEligibility()
    expect(f.controller.getSnapshot()).toMatchObject({ data: null, eligibility: null, busy: false })
  } finally { f.stop() }
})

it('does not revive eligibility after account invalidation', async () => {
  const f = await setup()
  try {
    await f.controller.load()
    let release!: (response: Response) => void
    f.fetch.mockImplementationOnce(() => new Promise(resolve => { release = resolve }))
    const pending = f.controller.readEligibility()
    f.fetch.mockResolvedValueOnce(Response.json({ ...session(), subject_id: draft }))
    await f.account.run('check')
    release(Response.json(eligibility()))
    await pending
    expect(f.controller.getSnapshot()).toMatchObject({ data: null, eligibility: null })
  } finally { f.stop() }
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

const confirmationPath = `/api/v1/problem-review/${review}/statement-confirmation`
const confirmationReceipt = (intent: object) => ({ ...intent, review_id: review, confirmed_at: new Date().toISOString() })

it('does not resend a confirmation after a lost response, absent receipt, or mismatched receipt', async () => {
  const f = await setup()
  await f.controller.load()
  await f.controller.confirm('confirm', false)
  expect(f.journal.size).toBe(0)
  expect(f.fetch.mock.calls.some(([url]) => url === confirmationPath)).toBe(false)
  f.fetch.mockRejectedValueOnce(new TypeError('response lost'))
  await f.controller.confirm('confirm', true)
  const [url, write] = f.fetch.mock.calls.at(-1) ?? []
  expect(url).toBe(confirmationPath)
  expect(write).toMatchObject({ method: 'POST', referrerPolicy: 'origin', mode: 'same-origin', redirect: 'error' })
  expect(new Headers(write?.headers).get('X-OJ-Expected-Subject')).toBe(owner)
  if (typeof write?.body !== 'string') throw new Error('Expected a JSON request body')
  const intent = JSON.parse(write.body) as { request_key: string }
  expect([...f.journal.values()]).toEqual([intent.request_key])
  expect(intent).toMatchObject({ assignment_revision: 1, import_revision: hash, evidence_sha256: hash, checklist_version: 'oj.statement-semantics.v1' })
  const receipt = confirmationReceipt(intent)
  f.fetch.mockResolvedValueOnce(new Response(null, { status: 404 }))
    .mockResolvedValueOnce(Response.json({ ...receipt, assignment_revision: 2 }))
    .mockResolvedValueOnce(Response.json(receipt))
  const start = f.fetch.mock.calls.length
  await f.controller.confirm('read')
  expect(f.controller.getSnapshot().confirmation).toBe('pending')
  await f.controller.confirm('confirm', true)
  await f.controller.confirm('read')
  expect(f.controller.getSnapshot().confirmation).toBe('pending')
  await f.controller.confirm('read')
  expect(f.controller.getSnapshot().confirmation).toBe('recorded')
  expect(f.fetch.mock.calls.slice(start).map(([target, options]) => [target, options?.method]))
    .toEqual(Array.from({ length: 3 }, () => [`${confirmationPath}/${intent.request_key}`, 'GET']))
  f.stop()
})

it('fails closed when the browser cannot save the original confirmation key', async () => {
  const f = await setup()
  await f.controller.load()
  vi.stubGlobal('sessionStorage', { getItem: () => null, setItem: () => { throw new Error('storage blocked') } })
  await f.controller.confirm('confirm', true)
  expect(f.controller.getSnapshot().confirmation).toBe('unavailable')
  expect(f.fetch.mock.calls.some(([url]) => url === confirmationPath)).toBe(false)
  f.stop()
})

it('drops a confirmation receipt that arrives after private invalidation', async () => {
  const f = await setup()
  await f.controller.load()
  let release!: () => void
  f.fetch.mockImplementationOnce((_, options) => new Promise(resolve => {
    if (typeof options?.body !== 'string') throw new Error('Expected a JSON request body')
    const intent = JSON.parse(options.body) as object
    release = () => resolve(Response.json(confirmationReceipt(intent)))
  }))
  const pending = f.controller.confirm('confirm', true)
  expect(f.controller.getSnapshot().busy).toBe(true)
  f.controller.clear()
  release()
  await pending
  expect(f.controller.getSnapshot()).toMatchObject({ data: null, busy: false, confirmation: 'unavailable' })
  expect(f.journal.size).toBe(1)
  f.stop()
})
