import { afterEach, expect, it, vi } from 'vitest'
import { sendReferencePreparation, sendReferenceRequest } from './referenceRequests'

const owner = 'a'.repeat(32)
const draft = 'b'.repeat(32)
const key = 'request_key_1234567'
const receipt = { id: 'c'.repeat(32), request_key: key, draft_id: draft, state: 'queued', created_at: '2026-11-12T00:00:00Z', cancelled_at: null }

afterEach(() => vi.unstubAllGlobals())

it('keeps reads private and preserves the browser Origin for bodyless writes', async () => {
  vi.stubGlobal('location', { protocol: 'https:' })
  const fetch = vi.fn<typeof globalThis.fetch>()
    .mockResolvedValueOnce(Response.json(receipt))
    .mockResolvedValueOnce(Response.json({ ready: true }))
    .mockResolvedValueOnce(Response.json({ ...receipt, state: 'cancelled', cancelled_at: receipt.created_at }))
  vi.stubGlobal('fetch', fetch)
  const signal = new AbortController().signal
  await expect(sendReferenceRequest('read', owner, draft, key, signal)).resolves.toEqual(receipt)
  await expect(sendReferencePreparation('prepare', owner, key, signal)).resolves.toBe(true)
  await expect(sendReferenceRequest('cancel', owner, draft, key, signal)).resolves.toMatchObject({ state: 'cancelled' })
  const root = `/api/v1/problem-draft-management/reference-requests/${key}`
  expect(fetch.mock.calls.map(([url, options]) => [url, options?.method, options?.referrerPolicy])).toEqual([
    [root, 'GET', 'no-referrer'], [`${root}/prepare`, 'POST', 'origin'], [`${root}/cancel`, 'POST', 'origin'],
  ])
  for (const [, options] of fetch.mock.calls) {
    expect(options).toMatchObject({ mode: 'same-origin', credentials: 'same-origin', redirect: 'error' })
    expect(options).not.toHaveProperty('body')
    const headers = new Headers(options?.headers)
    expect(headers.get('X-OJ-Expected-Subject')).toBe(owner)
    expect(headers.has('Origin')).toBe(false)
  }
})
