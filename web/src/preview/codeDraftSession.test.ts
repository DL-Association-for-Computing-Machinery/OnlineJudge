import { afterEach, describe, expect, it, vi } from 'vitest'
import { CodeDraftSession } from './codeDraftSession'
import { loadCodeDraft, writeCodeDraft, type DraftWrite } from './codeDraftStore'

vi.mock('./codeDraftStore', () => ({ loadCodeDraft: vi.fn(), writeCodeDraft: vi.fn() }))
const load = vi.mocked(loadCodeDraft)
const write = vi.mocked(writeCodeDraft)
afterEach(() => { vi.useRealTimers(); vi.resetAllMocks() })

async function start(allowed = () => true) {
  vi.useFakeTimers()
  load.mockResolvedValue({ epoch: 'initial', conflicts: [] })
  const session = new CodeDraftSession(['account', 'practice', 'P1001', 'gnu++17'], 'starter', allowed)
  session.connect()
  await vi.advanceTimersByTimeAsync(0)
  return session
}

describe('local editor save scheduling', () => {
  it('retries a private initial load paused by a fresh session check', async () => {
    vi.useFakeTimers()
    let allowed = true
    let finish!: () => void
    load.mockReturnValueOnce(new Promise(resolve => {
      finish = () => resolve({ epoch: 'initial', current: { code: 'first read', revision: 'r1' }, conflicts: [] })
    }))
    const session = new CodeDraftSession(['account', 'practice', 'P1001', 'gnu++17'], 'starter', () => allowed)
    session.connect()
    allowed = false
    finish()
    await vi.advanceTimersByTimeAsync(0)
    expect(session.getSnapshot().code).toBe('starter')
    allowed = true
    load.mockResolvedValueOnce({ epoch: 'initial', current: { code: 'fresh read', revision: 'r2' }, conflicts: [] })
    session.resume()
    await vi.advanceTimersByTimeAsync(0)
    expect(session.getSnapshot()).toMatchObject({ phase: 'saved', code: 'fresh read' })
    session.leave(false)
  })

  it('finishes an in-flight write and its latest queued edit across route navigation', async () => {
    const session = await start()
    let finish!: (reply: DraftWrite) => void
    write.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    session.edit('first')
    await vi.advanceTimersByTimeAsync(1_000)
    session.edit('leave with this')
    session.leave(true)
    expect(write.mock.calls[0][1]()).toBe(true)
    write.mockResolvedValueOnce({ status: 'saved', current: { revision: 'second', code: 'leave with this' } })
    finish({ status: 'saved', current: { revision: 'first', code: 'first' } })
    await vi.advanceTimersByTimeAsync(0)
    expect(write.mock.calls[1][0]).toMatchObject({ expected: 'first', code: 'leave with this' })
  })

  it('waits for the current revision receipt and never marks newer code saved on an old receipt', async () => {
    const session = await start()
    let finish!: (reply: DraftWrite) => void
    write.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    session.edit('first')
    await vi.advanceTimersByTimeAsync(999)
    expect(write).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)
    expect(session.getSnapshot().phase).toBe('saving')
    session.edit('newer')
    finish({ status: 'saved', current: { revision: 'first-version', code: 'first' } })
    await vi.advanceTimersByTimeAsync(0)
    expect(session.getSnapshot()).toMatchObject({ phase: 'dirty', code: 'newer' })
    write.mockResolvedValueOnce({ status: 'saved', current: { revision: 'second-version', code: 'newer' } })
    await vi.advanceTimersByTimeAsync(1_000)
    expect(write.mock.calls[1][0]).toMatchObject({ expected: 'first-version', code: 'newer' })
    expect(session.getSnapshot()).toMatchObject({ phase: 'saved', code: 'newer' })
    session.leave(false)
  })

  it('writes within five seconds of continual editing and makes a refusal visible', async () => {
    const session = await start()
    write.mockRejectedValueOnce(new Error('quota'))
    for (let index = 0; index < 6; index++) {
      session.edit(String(index))
      await vi.advanceTimersByTimeAsync(800)
    }
    expect(write).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(200)
    expect(write).toHaveBeenCalledTimes(1)
    expect(session.getSnapshot()).toMatchObject({ phase: 'failed', code: '5' })
    session.leave(false)
  })

  it('invalidates both a pending acknowledgement and its permission to write', async () => {
    let permitted = true
    const session = await start(() => permitted)
    let finish!: (reply: DraftWrite) => void
    write.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    session.edit('private')
    await vi.advanceTimersByTimeAsync(1_000)
    permitted = false
    session.hide()
    expect(write.mock.calls[0][1]()).toBe(false)
    finish({ status: 'saved', current: { revision: 'old', code: 'private' } })
    await vi.advanceTimersByTimeAsync(0)
    expect(session.getSnapshot()).toMatchObject({ phase: 'cleared', code: 'starter' })
    session.leave(false)
  })
})
