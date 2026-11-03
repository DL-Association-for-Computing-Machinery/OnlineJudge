import { afterEach, describe, expect, it, vi } from 'vitest'
import { AccountController } from '../account/controller'
import { DraftPreviewController } from './controller'

const id = 'a'.repeat(32)
const view = { subject_id: 'b'.repeat(32), created_at: new Date(Date.now() - 1000).toISOString(), idle_expires_at: new Date(Date.now() + 60_000).toISOString(), absolute_deadline: new Date(Date.now() + 120_000).toISOString() }
const preview = { draft_id: id, version: 'c'.repeat(64), title: 'private', language: 'en', markdown: 'private text', examples: [] }
afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers() })
async function setup() {
  vi.stubGlobal('location', { protocol: 'https:' })
  const fetch = vi.fn<typeof globalThis.fetch>().mockImplementation(() => Promise.resolve(Response.json(view)))
  vi.stubGlobal('fetch', fetch)
  const account = new AccountController()
  account.connect()
  await vi.waitFor(() => expect(account.getSnapshot().busy).toBe(false))
  const controller = new DraftPreviewController(account, id)
  controller.connect()
  return { account, controller, fetch }
}
describe('private display lifetime', () => {
  it('reads the session before the owner endpoint, and drops data on any account operation', async () => {
    const { controller, account, fetch } = await setup()
    fetch.mockResolvedValueOnce(Response.json(view)).mockResolvedValueOnce(Response.json(preview))
    await controller.load()
    expect(controller.getSnapshot().data).toEqual(preview)
    expect(fetch.mock.calls.slice(-2).map(call => call[0])).toEqual(['/api/v1/session', `/api/v1/problem-drafts/${id}/preview`])
    const pending = account.run('logout')
    expect(controller.getSnapshot().data).toBeNull()
    await pending
    controller.disconnect(); account.disconnect()
  })
  it('discards late success after invalidation, route disposal, and session checking', async () => {
    const { controller, account, fetch } = await setup()
    let release!: (value: Response) => void
    fetch.mockResolvedValueOnce(Response.json(view)).mockImplementationOnce(() => new Promise(resolve => { release = resolve }))
    const pending = controller.load()
    await vi.waitFor(() => expect(controller.getSnapshot().busy).toBe(true))
    controller.clear()
    release(Response.json(preview)); await pending
    expect(controller.getSnapshot().data).toBeNull()
    controller.disconnect(); account.disconnect()
  })
  it('does not revive a load invalidated while its session read is pending', async () => {
    const { controller, account, fetch } = await setup()
    let release!: (value: Response) => void
    fetch.mockImplementationOnce(() => new Promise(resolve => { release = resolve }))
    const pending = controller.load()
    controller.clear()
    release(Response.json(view)); await pending
    expect(fetch.mock.calls.some(call => typeof call[0] === 'string' && call[0].includes('problem-drafts'))).toBe(false)
    controller.disconnect(); account.disconnect()
  })
  it('clears at the observed session deadline without renewal', async () => {
    const { controller, account, fetch } = await setup()
    vi.useFakeTimers()
    fetch.mockResolvedValueOnce(Response.json(view)).mockResolvedValueOnce(Response.json(preview))
    await controller.load()
    expect(controller.getSnapshot().data).not.toBeNull()
    await vi.advanceTimersByTimeAsync(60_001)
    expect(controller.getSnapshot().data).toBeNull()
    expect(fetch.mock.calls).toHaveLength(3)
    controller.disconnect(); account.disconnect()
  })
  it('rejects a response past expiry even before the delayed expiry timer runs', async () => {
    const { controller, account, fetch } = await setup()
    vi.useFakeTimers()
    let release!: (value: Response) => void
    fetch.mockResolvedValueOnce(Response.json(view)).mockImplementationOnce(() => new Promise(resolve => { release = resolve }))
    const pending = controller.load()
    await vi.advanceTimersByTimeAsync(1)
    expect(controller.getSnapshot().busy).toBe(true)
    vi.setSystemTime(Date.parse(view.idle_expires_at) + 1)
    release(Response.json(preview)); await pending
    expect(controller.getSnapshot().data).toBeNull()
    controller.disconnect(); account.disconnect()
  })
})
