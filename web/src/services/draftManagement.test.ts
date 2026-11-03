import { afterEach, expect, it, vi } from 'vitest'
import { decodeReceipt, decodeSources, fetchImportSources, postImport } from './draftManagement'
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
