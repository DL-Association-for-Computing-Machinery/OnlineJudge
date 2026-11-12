import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AccountError, accountRequest } from './session'
import type { AccountOperation } from './session'

const view = { subject_id: 'a'.repeat(32), created_at: '2026-11-01T00:00:00Z', idle_expires_at: '2026-11-01T02:00:00Z', absolute_deadline: '2026-11-01T12:00:00Z' }
const signal = () => new AbortController().signal
const problem = (status: number, code: string) => Response.json({ type: 'about:blank', status, code, title: 'untrusted response title' }, { status, headers: { 'Content-Type': 'application/problem+json' } })

beforeEach(() => vi.stubGlobal('location', { protocol: 'https:' }))
afterEach(() => vi.unstubAllGlobals())

describe('same-origin account transport', () => {
  it('accepts a fresh session observation without touching or exposing internal versions', async () => {
    const fetch = vi.fn().mockResolvedValue(Response.json(view))
    vi.stubGlobal('fetch', fetch)
    await expect(accountRequest('read', signal())).resolves.toEqual(view)
    expect(fetch).toHaveBeenCalledExactlyOnceWith('/api/v1/session', expect.objectContaining({
      method: 'GET', mode: 'same-origin', credentials: 'same-origin', cache: 'no-store', redirect: 'error', referrerPolicy: 'no-referrer',
    }))
    expect(fetch.mock.calls[0][1]).not.toHaveProperty('body')
  })

  it('sends exact raw password bytes only to the fixed Begin path', async () => {
    const fetch = vi.fn<typeof globalThis.fetch>().mockResolvedValue(new Response(null, { status: 202 }))
    vi.stubGlobal('fetch', fetch)
    const input = { email: 'Name+tag@example.test', password: ' 原 始密碼 e\u0301 ' }
    await accountRequest('begin', signal(), input)
    expect(fetch).toHaveBeenCalledExactlyOnceWith('/api/v1/session/login/begin', expect.objectContaining({
      method: 'POST', body: JSON.stringify(input), mode: 'same-origin', redirect: 'error', referrerPolicy: 'origin',
    }))
    expect(fetch.mock.calls[0][1]?.headers).not.toHaveProperty('Origin')
  })

  it('blocks HTTP before serializing or submitting password input', async () => {
    vi.stubGlobal('location', { protocol: 'http:' })
    const fetch = vi.fn()
    vi.stubGlobal('fetch', fetch)
    const readPassword = vi.fn(() => 'test')
    const input = { email: 'test@example.test', get password() { return readPassword() } }
    await expect(accountRequest('begin', signal(), input)).rejects.toMatchObject({ kind: 'insecure', code: 'HTTPS_REQUIRED' })
    expect(fetch).not.toHaveBeenCalled()
    expect(readPassword).not.toHaveBeenCalled()
  })

  it.each(['logout', 'complete', 'abandon', 'password'] as const)('accepts only 204 for %s', async operation => {
    const fetch = vi.fn().mockResolvedValue(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetch)
    await expect(accountRequest(operation, signal())).resolves.toBeUndefined()
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it.each([
    () => new Response(null, { status: 200 }),
    () => new Response('unexpected', { status: 202 }),
    () => problem(503, 'LOGIN_REJECTED'),
    () => problem(401, 'SESSION_REQUIRED'),
    () => new Response('<html>gateway error</html>', { status: 502 }),
    () => new Response('{', { status: 400, headers: { 'Content-Type': 'application/problem+json' } }),
    () => problem(400, '__proto__'),
  ])('keeps unexpected Begin results uncertain without retries', async response => {
    const fetch = vi.fn().mockResolvedValue(response())
    vi.stubGlobal('fetch', fetch)
    await expect(accountRequest('begin', signal(), { email: 'e@example.test', password: 'test' })).rejects.toMatchObject({ kind: 'unknown' })
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it('uses fixed local messages for recognized operation-specific refusals', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(problem(401, 'PASSWORD_CHANGE_REJECTED')))
    await expect(accountRequest('password', signal())).rejects.toThrow('请核对当前密码')
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(problem(401, 'PASSWORD_CHANGE_REJECTED')))
    await expect(accountRequest('read', signal())).rejects.toMatchObject({ kind: 'unknown' })
  })

  it.each([
    {}, { ...view, auth_generation: 1 }, { ...view, subject_id: 'not-an-id' },
    { ...view, idle_expires_at: view.created_at }, { ...view, absolute_deadline: view.created_at },
    { ...view, created_at: '2026-11-01' }, { ...view, created_at: null },
  ])('rejects malformed successful session data', async data => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json(data)))
    await expect(accountRequest('read', signal())).rejects.toMatchObject({ kind: 'unknown' })
  })

  it('bounds response bodies and never reflects their content', async () => {
    const cancel = vi.fn()
    const body = new ReadableStream<Uint8Array>({
      start(controller) { controller.enqueue(new TextEncoder().encode('sensitive'.repeat(3000))) }, cancel,
    })
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(body, { headers: { 'Content-Type': 'application/json' } })))
    await expect(accountRequest('read', signal())).rejects.toThrow('结果暂时无法确认')
    expect(cancel).toHaveBeenCalledTimes(1)
  })

  it('discards an unusable gateway stream without reading or displaying it', async () => {
    const cancel = vi.fn()
    const stream = new ReadableStream<Uint8Array>({ cancel })
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(stream, { status: 503 })))
    await expect(accountRequest('read', signal())).rejects.toMatchObject({ kind: 'unknown' })
    expect(cancel).toHaveBeenCalledTimes(1)
  })

  it.each(['read', 'begin', 'password'] as AccountOperation[])('sanitizes network and redirect failures for %s', async operation => {
    const fetch = vi.fn().mockRejectedValue(new Error('private redirected URL or body'))
    vi.stubGlobal('fetch', fetch)
    // The overloads deliberately distinguish session observations from writes.
    const request = operation === 'read' ? accountRequest('read', signal()) : accountRequest(operation === 'begin' ? 'begin' : 'password', signal())
    await expect(request).rejects.toEqual(expect.any(AccountError))
    await expect(request).rejects.toThrow('结果暂时无法确认')
    expect(fetch).toHaveBeenCalledTimes(1)
  })
})
