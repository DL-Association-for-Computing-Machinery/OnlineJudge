import { afterEach, expect, it, vi } from 'vitest'
import { AccountController } from '../account/controller'
import { HistoryController } from './historyController'

const owner = 'a'.repeat(32)
const other = 'b'.repeat(32)
const cursor = 'A'.repeat(80)
const session = (subject: string) => ({
  subject_id: subject, created_at: new Date(Date.now() - 1000).toISOString(),
  idle_expires_at: new Date(Date.now() + 60_000).toISOString(),
  absolute_deadline: new Date(Date.now() + 120_000).toISOString(),
})
const row = (number: number) => ({
  id: number.toString(16).padStart(32, '0'), material_version: 'synthetic-history-material', language: 'gnu++17',
  state: 'finished', verdict: 'AC', created_at: '2026-10-25T12:34:56.123456Z',
})
const first = () => Response.json({ submissions: Array.from({ length: 20 }, (_, n) => row(22 - n)), next_cursor: cursor })
const last = () => Response.json({ submissions: [row(2), row(1)], next_cursor: '' })

afterEach(() => vi.unstubAllGlobals())

async function setup() {
  vi.stubGlobal('location', { protocol: 'https:' })
  vi.stubGlobal('document', { visibilityState: 'visible' })
  const fetch = vi.fn<typeof globalThis.fetch>().mockResolvedValueOnce(Response.json(session(owner)))
  vi.stubGlobal('fetch', fetch)
  const account = new AccountController()
  const history = new HistoryController(account)
  history.connect()
  account.connect()
  await vi.waitFor(() => expect(account.getSnapshot().busy).toBe(false))
  return { fetch, account, history, stop: () => { history.disconnect(); account.disconnect() } }
}

it('drops a late page when the observed account changes and never carries over its cursor', async () => {
  const f = await setup()
  try {
    f.fetch.mockResolvedValueOnce(first())
    await f.history.first()
    expect(f.history.getSnapshot().page?.submissions).toHaveLength(20)
    let release!: (response: Response) => void
    f.fetch.mockImplementationOnce(() => new Promise(resolve => { release = resolve }))
    const pending = f.history.next()
    expect(f.fetch.mock.calls.at(-1)?.[0]).toBe('/api/v1/training/submissions?cursor=' + cursor)
    expect(f.history.getSnapshot().page).toBeNull()
    f.fetch.mockResolvedValueOnce(Response.json(session(other)))
    await f.account.run('check')
    release(last())
    await pending
    expect(f.history.getSnapshot().page).toBeNull()
    const calls = f.fetch.mock.calls.length
    await f.history.next()
    expect(f.fetch.mock.calls).toHaveLength(calls)
    f.fetch.mockResolvedValueOnce(Response.json({ submissions: [row(24)], next_cursor: '' }))
    await f.history.first()
    const [url, request] = f.fetch.mock.calls.at(-1) ?? []
    expect(url).toBe('/api/v1/training/submissions')
    expect(new Headers(request?.headers).get('X-OJ-Expected-Subject')).toBe(other)
    expect(f.history.getSnapshot().page?.submissions).toEqual([row(24)])
  } finally { f.stop() }
})

it('clears the page on a mismatched subject and checks the cookie without retrying the old cursor', async () => {
  const f = await setup()
  try {
    f.fetch.mockResolvedValueOnce(first())
    await f.history.first()
    f.fetch.mockResolvedValueOnce(new Response(null, { status: 409 }))
      .mockResolvedValueOnce(Response.json(session(other)))
    await f.history.next()
    expect(f.history.getSnapshot().page).toBeNull()
    expect(f.account.getSnapshot().session?.subject_id).toBe(other)
    expect(f.fetch.mock.calls.slice(2).map(([url]) => url)).toEqual([
      '/api/v1/training/submissions?cursor=' + cursor, '/api/v1/session',
    ])
  } finally { f.stop() }
})

it('discards a late source when another detail is opened or the account changes', async () => {
  const f = await setup()
  try {
    f.fetch.mockResolvedValueOnce(last())
    await f.history.first()
    let release!: (response: Response) => void
    f.fetch.mockImplementationOnce(() => new Promise(resolve => { release = resolve }))
    const old = f.history.openDetail(row(1).id)
    const source = '\x01'.repeat(65_536)
    // Go escapes every control byte in this payload as six JSON bytes.
    const encoded = JSON.stringify({ ...row(2), source })
    f.fetch.mockResolvedValueOnce(new Response(encoded, { headers: { 'Content-Type': 'application/json' } }))
    await f.history.openDetail(row(2).id)
    expect(f.history.getSnapshot().detail?.value).toEqual({ ...row(2), source })
    release(Response.json({ ...row(1), source: 'older source' }))
    await old
    expect(f.history.getSnapshot().detail?.value?.id).toBe(row(2).id)

    f.fetch.mockImplementationOnce(() => new Promise(resolve => { release = resolve }))
    const pending = f.history.openDetail(row(2).id)
    f.fetch.mockResolvedValueOnce(Response.json(session(other)))
    await f.account.run('check')
    release(Response.json({ ...row(2), source }))
    await pending
    expect(f.history.getSnapshot().detail).toBeNull()
  } finally { f.stop() }
})

it('rejects a detail response for a different submission', async () => {
  const f = await setup()
  try {
    f.fetch.mockResolvedValueOnce(last())
    await f.history.first()
    f.fetch.mockResolvedValueOnce(Response.json({ ...row(1), source: 'unexpected source' }))
    await f.history.openDetail(row(2).id)
    expect(f.history.getSnapshot().detail).toMatchObject({ id: row(2).id, value: null, busy: false })
  } finally { f.stop() }
})
