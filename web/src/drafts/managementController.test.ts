import { afterEach, expect, it, vi } from 'vitest'
import { AccountController } from '../account/controller'
import { DraftManagementController } from './managementController'
import { ImportJournal, JournalError, type JournalEntry } from './importJournal'
const owner = 'a'.repeat(32)
const source = { source_id: 'library-checker:aplusb', title: 'A+B', version: 'b'.repeat(64), import_revision: 'c'.repeat(64) }
const pending: JournalEntry = { owner, source: source.source_id, key: 'original_request_key', revision: source.import_revision, status: 'pending' }
const receipt = { draft_id: 'd'.repeat(32), request_key: pending.key, source_id: pending.source, version: source.version, import_revision: pending.revision, state: 'private-draft', validation_state: 'not-run', created_at: '2026-11-03T00:00:00Z' }
const session = () => ({ subject_id: owner, created_at: new Date(Date.now() - 1000).toISOString(), idle_expires_at: new Date(Date.now() + 60_000).toISOString(), absolute_deadline: new Date(Date.now() + 120_000).toISOString() })
afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers() })
async function setup() {
  vi.stubGlobal('location', { protocol: 'https:' }); vi.stubGlobal('document', { visibilityState: 'visible' })
  const fetch = vi.fn<typeof globalThis.fetch>().mockImplementation(url => Promise.resolve(Response.json(url === '/api/v1/session' ? session() : { sources: [source] })))
  vi.stubGlobal('fetch', fetch)
  const journal = { connect: vi.fn(), close: vi.fn(), list: vi.fn().mockResolvedValue([]), require: vi.fn().mockResolvedValue(undefined), active: vi.fn().mockResolvedValue(undefined), begin: vi.fn().mockResolvedValue({ entry: pending, created: true }), confirm: vi.fn().mockResolvedValue({ ...pending, status: 'confirmed', draftId: receipt.draft_id }), removeConfirmed: vi.fn() }
  const account = new AccountController(); account.connect(); await vi.waitFor(() => expect(account.getSnapshot().busy).toBe(false))
  const controller = new DraftManagementController(account, journal as unknown as ImportJournal); controller.connect()
  await controller.load()
  return { controller, account, journal, fetch, stop: () => { controller.disconnect(); account.disconnect() } }
}
it('never POSTs on load and waits for durable journal commit before the first POST', async () => {
  const f = await setup(); let release!: (value: unknown) => void
  f.journal.begin.mockImplementationOnce(() => new Promise(resolve => { release = resolve }))
  const action = f.controller.import(source)
  await vi.waitFor(() => expect(f.journal.begin).toHaveBeenCalledOnce())
  expect(f.fetch.mock.calls.some(([, options]) => options?.method === 'POST')).toBe(false)
  f.fetch.mockResolvedValueOnce(Response.json(receipt)); release({ entry: pending, created: true }); await action
  expect(f.fetch.mock.calls.filter(([, options]) => options?.method === 'POST')).toHaveLength(1)
  expect(f.journal.confirm).toHaveBeenCalledWith(pending, receipt); f.stop()
})
it('preserves an interrupted commit without sending a late POST', async () => {
  const f = await setup(); let release!: (value: unknown) => void
  f.journal.begin.mockImplementationOnce(() => new Promise(resolve => { release = resolve }))
  const action = f.controller.import(source); await vi.waitFor(() => expect(f.journal.begin).toHaveBeenCalledOnce())
  f.controller.clear(); release({ entry: pending, created: true }); await action
  expect(f.fetch.mock.calls.some(([, options]) => options?.method === 'POST')).toBe(false)
  expect(f.controller.getSnapshot().owner).toBeNull(); f.stop()
})
it('does not POST on storage failure or another tab winning the insertion', async () => {
  for (const failed of [true, false]) {
    const f = await setup()
    if (failed) f.journal.begin.mockRejectedValueOnce(new JournalError())
    else f.journal.begin.mockResolvedValueOnce({ entry: pending, created: false })
    await f.controller.import(source)
    expect(f.fetch.mock.calls.some(([, options]) => options?.method === 'POST')).toBe(false); f.stop()
  }
})
it('recovers an existing committed request with GET only and never trusts a local confirmed ID', async () => {
  const f = await setup()
  f.fetch.mockResolvedValueOnce(Response.json(session())).mockResolvedValueOnce(Response.json(receipt))
  await f.controller.recover(pending, true)
  expect(f.fetch.mock.calls.some(([, options]) => options?.method === 'POST')).toBe(false)
  expect(f.controller.getSnapshot().receipts[pending.key]).toEqual(receipt)
  f.fetch.mockResolvedValueOnce(Response.json(session())).mockResolvedValueOnce(Response.json(receipt))
  await f.controller.recover({ ...pending, status: 'confirmed', draftId: 'e'.repeat(32) })
  expect(f.controller.getSnapshot().receipts).toEqual({}); f.stop()
})
it('explicit retry GETs first, then POSTs the original tuple only after the revision still matches', async () => {
  for (const revision of [source.import_revision, 'e'.repeat(64)]) {
    const f = await setup(); const start = f.fetch.mock.calls.length
    f.fetch.mockResolvedValueOnce(Response.json(session()))
      .mockResolvedValueOnce(Response.json({ code: 'DRAFT_NOT_FOUND' }, { status: 404, headers: { 'Content-Type': 'application/problem+json' } }))
      .mockResolvedValueOnce(Response.json({ sources: [{ ...source, import_revision: revision }] })).mockResolvedValueOnce(Response.json(receipt))
    await f.controller.recover(pending, true)
    const calls = f.fetch.mock.calls.slice(start)
    expect(calls[1][0]).toContain('/imports/' + pending.key)
    expect(calls.filter(([, options]) => options?.method === 'POST')).toHaveLength(revision === source.import_revision ? 1 : 0)
    if (revision === source.import_revision) expect(calls[3][1]?.body).toBe(JSON.stringify({ source_id: pending.source, request_key: pending.key, expected_revision: pending.revision }))
    f.stop()
  }
})
it('does not continue an old-account intent after a fresh session changes owner', async () => {
  const f = await setup(); f.fetch.mockResolvedValueOnce(Response.json({ ...session(), subject_id: 'f'.repeat(32) }))
  await f.controller.import(source)
  expect(f.journal.begin).not.toHaveBeenCalled()
  expect(f.controller.getSnapshot().owner).toBeNull(); f.stop()
})
it('rejects a stale in-memory pending record after durable storage was cleared', async () => {
  const f = await setup(); f.journal.require.mockRejectedValueOnce(new JournalError())
  await f.controller.recover(pending, true)
  expect(f.fetch.mock.calls.some(([, options]) => options?.method === 'POST')).toBe(false)
  expect(f.controller.getSnapshot().owner).toBeNull(); f.stop()
})
it('drops late catalog and receipt results on invalidation, disposal, and elapsed deadline', async () => {
  for (const operation of ['catalog', 'receipt']) {
    for (const interruption of ['clear', 'disconnect', 'deadline']) {
      const f = await setup(); let release!: (value: Response) => void
      if (interruption === 'deadline') vi.useFakeTimers()
      f.fetch.mockResolvedValueOnce(Response.json(session())).mockImplementationOnce(() => new Promise(resolve => { release = resolve }))
      const action = operation === 'catalog' ? f.controller.load() : f.controller.recover(pending)
      // Flush the session check and journal awaits without advancing expiry.
      for (let i = 0; i < 30 && !release; i++) await Promise.resolve()
      expect(release).toBeTypeOf('function')
      if (interruption === 'clear') f.controller.clear()
      else if (interruption === 'disconnect') f.controller.disconnect()
      else vi.setSystemTime(Date.now() + 60_001)
      release(Response.json(operation === 'catalog' ? { sources: [source] } : receipt)); await action
      expect(f.controller.getSnapshot().sources).toEqual([])
      expect(f.controller.getSnapshot().receipts).toEqual({})
      expect(f.journal.confirm).not.toHaveBeenCalled()
      f.stop(); vi.useRealTimers()
    }
  }
})
it('cannot create a new revision when a locally confirmed receipt is missing, mismatched, or unavailable', async () => {
  for (const result of [Response.json({ code: 'DRAFT_NOT_FOUND' }, { status: 404, headers: { 'Content-Type': 'application/problem+json' } }), Response.json({ ...receipt, draft_id: 'e'.repeat(32) }), new Response('', { status: 503 })]) {
    const f = await setup()
    f.journal.active.mockResolvedValueOnce({ ...pending, status: 'confirmed', draftId: receipt.draft_id })
    f.fetch.mockResolvedValueOnce(Response.json(session())).mockResolvedValueOnce(result)
    await f.controller.import({ ...source })
    expect(f.journal.begin).not.toHaveBeenCalled()
    expect(f.fetch.mock.calls.some(([, options]) => options?.method === 'POST')).toBe(false); f.stop()
  }
})
it('clears records already shown when a late catalog or POST observes expiry before the timer runs', async () => {
  for (const operation of ['catalog', 'import']) {
    const f = await setup(); vi.useFakeTimers(); let release!: (value: Response) => void
    f.journal.list.mockResolvedValue([pending])
    f.fetch.mockResolvedValueOnce(Response.json(session())).mockImplementationOnce(() => new Promise(resolve => { release = resolve }))
    const action = operation === 'catalog' ? f.controller.load() : f.controller.import(source)
    for (let i = 0; i < 40 && !release; i++) await Promise.resolve()
    expect(release).toBeTypeOf('function')
    expect(f.controller.getSnapshot().entries).toEqual([pending])
    vi.setSystemTime(Date.now() + 60_001)
    release(Response.json(operation === 'catalog' ? { sources: [source] } : receipt)); await action
    expect(f.controller.getSnapshot()).toMatchObject({ owner: null, entries: [], sources: [], receipts: {}, busy: false })
    expect(f.journal.confirm).not.toHaveBeenCalled(); f.stop(); vi.useRealTimers()
  }
})
