import { afterEach, expect, it, vi } from 'vitest'
import { AccountController } from '../account/controller'
import { decodeReferenceReceipt, type ReferenceReceipt, type sendReferenceRequest } from '../services/referenceRequests'
import { ReferenceController } from './referenceController'
import type { ReferenceJournal } from './referenceJournal'

const owner = 'a'.repeat(32)
const draft = 'b'.repeat(32)
const key = 'original-reference-request'
const another = 'new-reference-request'
const session = () => ({ subject_id: owner, created_at: new Date(Date.now() - 1000).toISOString(),
  idle_expires_at: new Date(Date.now() + 60_000).toISOString(), absolute_deadline: new Date(Date.now() + 120_000).toISOString() })
const receipt = (state: ReferenceReceipt['state'] = 'queued'): ReferenceReceipt => ({
  id: 'd'.repeat(32), draft_id: draft, request_key: key, state, created_at: '2026-11-11T00:00:00Z',
  cancelled_at: state === 'cancelled' ? '2026-11-11T01:00:00Z' : null,
})
afterEach(() => vi.unstubAllGlobals())

async function setup() {
  vi.stubGlobal('location', { protocol: 'https:' })
  vi.stubGlobal('document', { visibilityState: 'visible' })
  vi.stubGlobal('fetch', vi.fn<typeof globalThis.fetch>().mockImplementation(() => Promise.resolve(Response.json(session()))))
  const account = new AccountController()
  account.connect()
  await vi.waitFor(() => expect(account.getSnapshot().status).toBe('authenticated'))
  const journal = { read: vi.fn<ReferenceJournal['read']>().mockResolvedValue(key), select: vi.fn<ReferenceJournal['select']>().mockResolvedValue(key) }
  const send = vi.fn<typeof sendReferenceRequest>().mockResolvedValue(receipt())
  const controller = new ReferenceController(account, draft, journal, send)
  controller.connect()
  return { account, journal, send, controller, stop: () => { controller.disconnect(); account.disconnect() } }
}

it('recovers an unknown write by the same key and only on explicit actions', async () => {
  const f = await setup()
  expect(f.send).not.toHaveBeenCalled()
  f.send.mockRejectedValueOnce(new Error('dropped response'))
  await f.controller.run('create')
  expect(f.controller.getSnapshot().receipt).toBeNull()
  expect(f.journal.select).toHaveBeenCalledWith(owner, draft, undefined)
  f.controller.disconnect(); f.controller.connect()
  expect(f.send).toHaveBeenCalledTimes(1)
  await f.controller.run('read')
  expect(f.send.mock.calls.map(args => args.slice(0, 4))).toEqual([['create', owner, draft, key], ['read', owner, draft, key]])
  expect(f.controller.getSnapshot().receipt?.state).toBe('queued')
  f.stop()
})

it('never cancels a different local request and requires a fresh receipt after an account change', async () => {
  const f = await setup()
  await f.controller.run('read')
  f.journal.read.mockResolvedValueOnce(another)
  await f.controller.run('cancel')
  expect(f.send).toHaveBeenCalledTimes(1)
  expect(f.controller.getSnapshot().receipt).toBeNull()
  await f.controller.run('read')
  await f.account.run('check')
  await f.controller.run('cancel')
  expect(f.send).toHaveBeenCalledTimes(2)
  f.stop()
})

it('does not send until the journal resolves, then discards an invalidated action or response', async () => {
  const f = await setup()
  let resolveKey!: (key: string) => void
  f.journal.select.mockImplementationOnce(() => new Promise(resolve => { resolveKey = resolve }))
  const before = f.controller.run('create')
  expect(f.send).not.toHaveBeenCalled()
  f.controller.clear()
  resolveKey(key)
  await before
  expect(f.send).not.toHaveBeenCalled()
  let resolveReceipt!: (receipt: ReferenceReceipt) => void
  f.send.mockImplementationOnce(() => new Promise(resolve => { resolveReceipt = resolve }))
  const after = f.controller.run('create')
  await vi.waitFor(() => expect(f.send).toHaveBeenCalledTimes(1))
  await f.account.run('check')
  resolveReceipt(receipt())
  await after
  expect(f.controller.getSnapshot().receipt).toBeNull()
  f.stop()
})

it('permits a fresh key only after a terminal receipt and rejects an overbroad receipt', async () => {
  const f = await setup()
  f.send.mockResolvedValueOnce(receipt('reported'))
  await f.controller.run('read')
  await f.controller.run('create')
  expect(f.journal.select).toHaveBeenCalledWith(owner, draft, key)
  expect(() => decodeReferenceReceipt({ ...receipt(), report: 'private' }, draft, key)).toThrow()
  expect(() => decodeReferenceReceipt(receipt(), draft, another)).toThrow()
  expect(decodeReferenceReceipt(receipt('cancelled'), draft, key).state).toBe('cancelled')
  f.stop()
})
