import { afterEach, expect, it, vi } from 'vitest'
import { AccountController } from '../account/controller'
import { OwnDraftListController } from './listController'

const owner = 'a'.repeat(32)
const session = () => ({ subject_id: owner, created_at: new Date(Date.now() - 1000).toISOString(), idle_expires_at: new Date(Date.now() + 60_000).toISOString(), absolute_deadline: new Date(Date.now() + 120_000).toISOString() })
const receipt = (id = 'b'.repeat(32)) => ({ draft_id: id, request_key: 'propertyIsEnumerable', source_id: 'library-checker:aplusb', version: 'c'.repeat(64), import_revision: 'd'.repeat(64), state: 'private-draft', validation_state: 'not-run', created_at: '2026-11-03T00:00:00.123456Z' })
const first = () => ({ drafts: Array.from({ length: 20 }, (_, n) => ({ ...receipt(n.toString(16).padStart(32, '0')), request_key: 'list-fixture-key-' + n })), next_cursor: 'opaque_cursor_with_microseconds' })
afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers() })
async function setup() {
  vi.stubGlobal('location', { protocol: 'https:' }); vi.stubGlobal('document', { visibilityState: 'visible' })
  const fetch = vi.fn<typeof globalThis.fetch>().mockImplementation(url => Promise.resolve(Response.json(url === '/api/v1/session' ? session() : first())))
  vi.stubGlobal('fetch', fetch)
  // Any accidental dependency on browser persistence would fail this suite.
  vi.stubGlobal('indexedDB', { open: () => { throw new Error('unavailable') } })
  const account = new AccountController(); account.connect(); await vi.waitFor(() => expect(account.getSnapshot().busy).toBe(false))
  const controller = new OwnDraftListController(account); controller.connect()
  return { fetch, account, controller, stop: () => { controller.disconnect(); account.disconnect() } }
}
it('does nothing on connect, then reads owner pages without persistence or writes', async () => {
  const f = await setup(); expect(f.fetch.mock.calls).toHaveLength(1)
  await f.controller.first()
  expect(f.controller.getSnapshot().page?.drafts).toHaveLength(20)
  f.fetch.mockResolvedValueOnce(Response.json(session())).mockResolvedValueOnce(Response.json({ drafts: [receipt()], next_cursor: '' }))
  await f.controller.next()
  expect(f.fetch.mock.calls.at(-1)?.[0]).toBe('/api/v1/problem-draft-management/drafts?cursor=opaque_cursor_with_microseconds')
  expect(new Headers(f.fetch.mock.calls.at(-1)?.[1]?.headers).get('X-OJ-Expected-Subject')).toBe(owner)
  expect(f.controller.getSnapshot().page?.drafts).toHaveLength(1)
  expect(f.controller.getSnapshot().pageNumber).toBe(2)
  expect(f.fetch.mock.calls.some(([, options]) => options?.method === 'POST')).toBe(false); f.stop()
})
it('discards late first/next pages on clear, account publication, disposal and expiry before a timer', async () => {
  for (const operation of ['first', 'next']) for (const interruption of ['clear', 'account', 'disconnect', 'deadline']) {
    const f = await setup(); await f.controller.first()
    let release!: (value: Response) => void
    if (interruption === 'deadline') vi.useFakeTimers()
    f.fetch.mockResolvedValueOnce(Response.json(session())).mockImplementationOnce(() => new Promise(resolve => { release = resolve }))
    const action = operation === 'first' ? f.controller.first() : f.controller.next()
    for (let i = 0; i < 30 && !release; i++) await Promise.resolve()
    expect(release).toBeTypeOf('function')
    expect(f.controller.getSnapshot().page).toBeNull()
    if (interruption === 'clear') f.controller.clear()
    else if (interruption === 'account') await f.account.run('check')
    else if (interruption === 'disconnect') f.controller.disconnect()
    else vi.setSystemTime(Date.now() + 60_001)
    release(Response.json(first())); await action
    expect(f.controller.getSnapshot().page).toBeNull(); expect(f.controller.getSnapshot().owner).toBeNull()
    f.stop(); vi.useRealTimers()
  }
})
it('does not carry an old cursor into a newly observed owner', async () => {
  const f = await setup(); await f.controller.first(); const before = f.fetch.mock.calls.length
  f.fetch.mockResolvedValueOnce(Response.json({ ...session(), subject_id: 'e'.repeat(32) }))
  await f.controller.next()
  expect(f.fetch.mock.calls).toHaveLength(before + 1)
  expect(f.controller.getSnapshot().page).toBeNull(); f.stop()
})
it('clears stale data on a failed page and requires an explicit first-page retry', async () => {
  const f = await setup(); await f.controller.first()
  f.fetch.mockResolvedValueOnce(Response.json(session())).mockResolvedValueOnce(new Response('', { status: 503 }))
  await f.controller.next(); expect(f.controller.getSnapshot().page).toBeNull()
  const count = f.fetch.mock.calls.length; await f.controller.next(); expect(f.fetch.mock.calls).toHaveLength(count)
  await f.controller.first(); expect(f.controller.getSnapshot().pageNumber).toBe(1); f.stop()
})
