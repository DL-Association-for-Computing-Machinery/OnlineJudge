import { afterEach, expect, it, vi } from 'vitest'
import { decodeReceipt, decodeSources, decodeOwnDraftPage, fetchImportSources, fetchOwnDraftPage, postImport, readAuthorSolution,
  requestAuthorSolutionConfirmation, requestReviewIntake } from './draftManagement'
const tuple = { owner: 'a'.repeat(32), key: 'request_key_1234567', source: 'library-checker:aplusb', revision: 'b'.repeat(64) }
const source = { source_id: tuple.source, title: 'A + B', version: 'c'.repeat(64), import_revision: tuple.revision }
const receipt = { draft_id: 'd'.repeat(32), request_key: tuple.key, source_id: tuple.source, version: source.version, import_revision: tuple.revision, state: 'private-draft', validation_state: 'not-run', created_at: '2026-11-03T00:00:00Z' }
afterEach(() => vi.unstubAllGlobals())
it('validates exact source and receipt shapes and binds the complete tuple', () => {
  expect(decodeSources({ sources: [source] })).toEqual([source])
  expect(decodeSources({ sources: [{ ...source, title: '\ufefftitle', source_id: 'library-checker:a+b' }] })).toHaveLength(1)
  expect(decodeReceipt(receipt, tuple)).toEqual(receipt)
  for (const bad of [{ ...receipt, request_key: tuple.key + 'x' }, { ...receipt, import_revision: 'e'.repeat(64) }, { ...receipt, extra: true }, { ...receipt, state: 'published' }]) expect(() => decodeReceipt(bad, tuple)).toThrow()
  for (const sources of [[source, source], [{ ...source, title: '\ud800' }], [{ ...source, import_revision: 'short' }], []]) expect(() => decodeSources({ sources })).toThrow()
})
it('uses the fail-closed namespace and expected subject with the frozen tuple', async () => {
  vi.stubGlobal('location', { protocol: 'https:' })
  const fetch = vi.fn<typeof globalThis.fetch>().mockResolvedValue(Response.json(receipt)); vi.stubGlobal('fetch', fetch)
  await postImport(tuple, new AbortController().signal)
  expect(fetch).toHaveBeenCalledOnce()
  expect(fetch.mock.calls[0][0]).toBe('/api/v1/problem-draft-management/imports')
  expect(fetch.mock.calls[0][1]).toMatchObject({ method: 'POST', mode: 'same-origin', redirect: 'error', referrerPolicy: 'origin' })
  expect(new Headers(fetch.mock.calls[0][1]?.headers).get('X-OJ-Expected-Subject')).toBe(tuple.owner)
  expect(fetch.mock.calls[0][1]?.body).toBe(JSON.stringify({ source_id: tuple.source, request_key: tuple.key, expected_revision: tuple.revision }))
})
it('does not downgrade, retry, or accept unexpected errors and oversized data', async () => {
  vi.stubGlobal('location', { protocol: 'https:' })
  for (const response of [new Response('not found', { status: 404 }), Response.json({ sources: [source], padding: 'x'.repeat(33_000) })]) {
    const fetch = vi.fn().mockResolvedValue(response); vi.stubGlobal('fetch', fetch)
    await expect(fetchImportSources(tuple.owner, new AbortController().signal)).rejects.toThrow()
    expect(fetch).toHaveBeenCalledOnce()
  }
})
it('accepts bounded owner metadata without rebuilding opaque microsecond cursors', async () => {
  const rows = Array.from({ length: 20 }, (_, n) => ({ ...receipt, draft_id: n.toString(16).padStart(32, '0'), request_key: tuple.key + '_' + n, created_at: '2026-11-03T00:00:00.123456Z' }))
  const page = { drafts: rows, next_cursor: 'Opaque_microseconds_123456' }
  expect(decodeOwnDraftPage(page, tuple.owner)).toEqual(page)
  expect(() => decodeOwnDraftPage({ drafts: [receipt, { ...receipt, draft_id: 'e'.repeat(32) }], next_cursor: '' }, tuple.owner)).toThrow()
  for (const bad of [{ drafts: [receipt, receipt], next_cursor: '' }, { drafts: rows.concat(receipt), next_cursor: '' }, { drafts: [], next_cursor: 'unexpected' }, { ...page, next_cursor: 'encoded%2Falias' }, { ...page, extra: true }]) expect(() => decodeOwnDraftPage(bad, tuple.owner)).toThrow()
  vi.stubGlobal('location', { protocol: 'https:' })
  const fetch = vi.fn<typeof globalThis.fetch>().mockResolvedValue(Response.json({ drafts: [receipt], next_cursor: '' })); vi.stubGlobal('fetch', fetch)
  await fetchOwnDraftPage(tuple.owner, page.next_cursor, new AbortController().signal)
  expect(fetch.mock.calls[0][0]).toBe('/api/v1/problem-draft-management/drafts?cursor=' + page.next_cursor)
  expect(fetch.mock.calls[0][1]).toMatchObject({ method: 'GET', referrerPolicy: 'no-referrer' })
  expect(fetch).toHaveBeenCalledOnce()
})

it('binds a readback to the original report and full frozen revision', async () => {
  const intent = { request_key: tuple.key, draft_id: receipt.draft_id, reference_request_id: 'e'.repeat(32), import_revision: tuple.revision }
  const accepted = { ...intent, id: 'f'.repeat(32), content_version: receipt.version, created_at: receipt.created_at }
  vi.stubGlobal('location', { protocol: 'https:' })
  const fetch = vi.fn<typeof globalThis.fetch>().mockResolvedValueOnce(Response.json({ ...accepted, import_revision: 'e'.repeat(64) }))
    .mockResolvedValueOnce(Response.json(accepted))
  vi.stubGlobal('fetch', fetch)
  const signal = new AbortController().signal
  await expect(requestReviewIntake('read', tuple.owner, intent, receipt.version, signal)).rejects.toThrow()
  await expect(requestReviewIntake('read', tuple.owner, intent, receipt.version, signal)).resolves.toBe(accepted.id)
  for (const [url, init] of fetch.mock.calls) {
    expect(url).toBe('/api/v1/problem-draft-management/review-submissions/' + tuple.key)
    expect(init).toMatchObject({ method: 'GET', referrerPolicy: 'no-referrer' })
    expect(init?.body).toBeUndefined()
    expect(new Headers(init?.headers).get('X-OJ-Expected-Subject')).toBe(tuple.owner)
  }
})

const intent = { request_key: tuple.key, draft_id: receipt.draft_id, reference_request_id: 'e'.repeat(32), import_revision: tuple.revision }
const intake = { ...intent, id: 'f'.repeat(32), content_version: receipt.version, created_at: receipt.created_at }
const binding = { import_revision: intent.import_revision, formal_plan_sha256: 'd'.repeat(64) }
const solution = { ...binding, standard_solution: { source: 'int main() {}\n' } }
const confirmation = { ...binding, request_key: intent.request_key, confirmed_at: receipt.created_at }
const intakePath = '/api/v1/problem-draft-management/review-submissions/' + tuple.key

it('does not request source after an unmatched intake and rejects a preview outside the wire limit', async () => {
  vi.stubGlobal('location', { protocol: 'https:' })
  const fetch = vi.fn<typeof globalThis.fetch>().mockResolvedValueOnce(Response.json({ ...intake, draft_id: intake.id }))
  vi.stubGlobal('fetch', fetch)
  const read = () => readAuthorSolution(tuple.owner, intent, receipt.version, new AbortController().signal)
  await expect(read()).rejects.toThrow()
  expect(fetch.mock.calls.map(([url]) => url)).toEqual([intakePath])
  fetch.mockResolvedValueOnce(Response.json(intake)).mockResolvedValueOnce(Response.json({ ...solution, padding: 'x'.repeat(512 * 1024) }))
  await expect(read()).rejects.toThrow()
  fetch.mockResolvedValueOnce(Response.json(intake)).mockResolvedValueOnce(Response.json(solution))
  await expect(read()).resolves.toEqual({ id: intake.id, binding, solution: solution.standard_solution })
})

it('reads the original confirmation with the same account and rejects a receipt for another plan', async () => {
  vi.stubGlobal('location', { protocol: 'https:' })
  const fetch = vi.fn<typeof globalThis.fetch>().mockResolvedValueOnce(Response.json(intake))
    .mockResolvedValueOnce(Response.json({ ...confirmation, formal_plan_sha256: '0'.repeat(64) }))
    .mockResolvedValueOnce(Response.json(intake)).mockResolvedValueOnce(Response.json(confirmation))
  vi.stubGlobal('fetch', fetch)
  const read = () => requestAuthorSolutionConfirmation('read', tuple.owner, intent, receipt.version, binding, new AbortController().signal)
  await expect(read()).rejects.toThrow()
  await expect(read()).resolves.toBeUndefined()
  expect(fetch.mock.calls.map(([url]) => url)).toEqual([intakePath, intakePath + '/solution-confirmation', intakePath, intakePath + '/solution-confirmation'])
  for (const [, init] of fetch.mock.calls) {
    expect(init).toMatchObject({ method: 'GET', cache: 'no-store', redirect: 'error' })
    expect(new Headers(init?.headers).get('X-OJ-Expected-Subject')).toBe(tuple.owner)
    expect(init?.body).toBeUndefined()
  }
})
