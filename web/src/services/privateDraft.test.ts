import { afterEach, describe, expect, it, vi } from 'vitest'
import { decodeDraftPreview, fetchDraftPreview } from './privateDraft'

const id = 'a'.repeat(32)
const preview = { draft_id: id, version: 'b'.repeat(64), title: 'Title', language: 'en', markdown: '# Exact\r\n', examples: [{ id: 'case-0001', input: '\ufeff 1\r\n\0', output: '' }] }
const image = { source: 'figures/a.png', media_type: 'image/png', data: 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jL1sAAAAASUVORK5CYII=', width: 1, height: 1 }
afterEach(() => vi.unstubAllGlobals())
describe('private preview response contract', () => {
  it('preserves sample and Markdown strings exactly', () => expect(decodeDraftPreview(preview, id)).toEqual(preview))
  it('accepts only bounded canonical PNG assets with exact paths and header dimensions', () => {
    expect(decodeDraftPreview({ ...preview, images: [image] }, id).images).toEqual([image])
    for (const images of [[], null, [image, image], [{ ...image, source: '../a.png' }], [{ ...image, source: 'https:a.png' }], [{ ...image, source: 'a.png\n' }],
      [{ ...image, data: 'data:image/svg+xml;base64,x' }], [{ ...image, data: image.data + '\n' }], [{ ...image, width: 2 }],
      [{ ...image, media_type: 'image/svg+xml' }], [{ ...image, secret: 'extra' }], [{ ...image, data: 'A'.repeat(349_529) }]]) {
      expect(() => decodeDraftPreview({ ...preview, images }, id)).toThrow()
    }
    expect(() => decodeDraftPreview({ ...preview, markdown: 'x'.repeat(1_048_550), images: [image] }, id)).toThrow()
  })
  it.each([
    { ...preview, version: 1 }, { ...preview, title: '中'.repeat(171) }, { ...preview, markdown: '\ud800' },
    { ...preview, examples: [...preview.examples, ...preview.examples] }, { ...preview, markdown: 'x'.repeat(1_048_577) },
    { ...preview, secret: 'forbidden' }, { ...preview, draft_id: 'c'.repeat(32) },
  ])('rejects malformed, unexpected or excessive fields', value => expect(() => decodeDraftPreview(value, id)).toThrow())
  it('uses one bounded read-only same-origin HTTPS request', async () => {
    vi.stubGlobal('location', { protocol: 'https:' })
    const fetch = vi.fn().mockResolvedValue(Response.json(preview))
    vi.stubGlobal('fetch', fetch)
    expect(await fetchDraftPreview(id, new AbortController().signal)).toEqual(preview)
    expect(fetch).toHaveBeenCalledOnce()
    expect(fetch.mock.calls[0]).toEqual([`/api/v1/problem-drafts/${id}/preview`, expect.objectContaining({ method: 'GET', credentials: 'same-origin', cache: 'no-store', redirect: 'error', referrerPolicy: 'no-referrer' })])
  })
  it('rejects HTTP without a request and never renders server error text', async () => {
    const fetch = vi.fn().mockResolvedValue(new Response('<script>secret</script>', { status: 403 }))
    vi.stubGlobal('fetch', fetch)
    vi.stubGlobal('location', { protocol: 'http:' })
    await expect(fetchDraftPreview(id, new AbortController().signal)).rejects.toThrow('草稿预览暂不可用')
    expect(fetch).not.toHaveBeenCalled()
    vi.stubGlobal('location', { protocol: 'https:' })
    await expect(fetchDraftPreview(id, new AbortController().signal)).rejects.toThrow('草稿预览暂不可用')
    expect(fetch).toHaveBeenCalledOnce()
  })
  it('cancels an excessive response stream and rejects invalid UTF-8', async () => {
    vi.stubGlobal('location', { protocol: 'https:' })
    const cancel = vi.fn()
    const stream = new ReadableStream({ start(c) { c.enqueue(new Uint8Array(7 * 1024 * 1024 + 1)) }, cancel })
    const fetch = vi.fn().mockResolvedValueOnce(new Response(stream, { headers: { 'Content-Type': 'application/json' } })).mockResolvedValueOnce(new Response(new Uint8Array([0xff]), { headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetch)
    await expect(fetchDraftPreview(id, new AbortController().signal)).rejects.toThrow()
    expect(cancel).toHaveBeenCalledOnce()
    await expect(fetchDraftPreview(id, new AbortController().signal)).rejects.toThrow()
  })
})
