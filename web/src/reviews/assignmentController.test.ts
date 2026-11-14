import { afterEach, expect, it, vi } from 'vitest'
import { AccountController } from '../account/controller'
import { ReviewAssignmentController } from './assignmentController'

const owner = 'a'.repeat(32)
const other = 'b'.repeat(32)
const path = '/api/v1/problem-review/assignments'
const cursor = 'A'.repeat(91)
const item = { review_id: '1'.repeat(32), source_id: 'library-checker:aplusb', import_revision: 'e'.repeat(64), assignment_revision: 2 }

async function fixture() {
  vi.stubGlobal('location', { protocol: 'https:' })
  vi.stubGlobal('document', { visibilityState: 'visible' })
  let subject = owner
  const fetch = vi.fn<typeof globalThis.fetch>().mockImplementation(url => Promise.resolve(Response.json(url === '/api/v1/session'
    ? { subject_id: subject, created_at: new Date(Date.now() - 1000).toISOString(), idle_expires_at: new Date(Date.now() + 60_000).toISOString(), absolute_deadline: new Date(Date.now() + 120_000).toISOString() }
    : { assignments: url === path ? [] : [item], next_cursor: url === path ? cursor : '' })))
  vi.stubGlobal('fetch', fetch)
  const account = new AccountController()
  account.connect()
  await vi.waitFor(() => expect(account.getSnapshot().busy).toBe(false))
  const controller = new ReviewAssignmentController(account)
  controller.connect()
  return { controller, account, fetch, changeSubject: () => { subject = other }, stop: () => {
    controller.disconnect()
    account.disconnect()
  } }
}

afterEach(() => { vi.unstubAllGlobals() })

it('checks the current subject before each explicit request and can advance from an empty page', async () => {
  const f = await fixture()
  try {
    expect(f.fetch.mock.calls.map(([url]) => url)).toEqual(['/api/v1/session'])
    await f.controller.load()
    expect(f.controller.getSnapshot().page).toEqual({ assignments: [], next_cursor: cursor })
    await f.controller.load(true)
    expect(f.fetch.mock.calls.map(([url]) => url)).toEqual(['/api/v1/session', '/api/v1/session', path, '/api/v1/session', `${path}?cursor=${cursor}`])
    const options = f.fetch.mock.calls.at(-1)?.[1]
    expect(options).toMatchObject({ method: 'GET', cache: 'no-store', redirect: 'error', referrerPolicy: 'no-referrer' })
    expect(new Headers(options?.headers).get('X-OJ-Expected-Subject')).toBe(owner)
    expect(f.controller.getSnapshot().page?.assignments).toEqual([item])
  } finally { f.stop() }
})

it('does not send a previous owner’s cursor after checking a changed account', async () => {
  const f = await fixture()
  try {
    await f.controller.load()
    f.changeSubject()
    await f.controller.load(true)
    expect(f.fetch.mock.calls.map(([url]) => url)).toEqual(['/api/v1/session', '/api/v1/session', path, '/api/v1/session'])
    expect(f.controller.getSnapshot().page).toBeNull()
  } finally { f.stop() }
})

it('discards a stale response and refuses additional fields or duplicate review IDs', async () => {
  const f = await fixture()
  try {
    await f.controller.load()
    const fallback = f.fetch.getMockImplementation()!
    let release!: (r: Response) => void
    f.fetch.mockImplementation((url, options) => url === path
      ? new Promise(resolve => { release = resolve })
      : fallback(url, options))
    const pending = f.controller.load()
    await vi.waitFor(() => expect(f.controller.getSnapshot().busy).toBe(true))
    f.controller.clear()
    release(Response.json({ assignments: [item], next_cursor: '' }))
    await pending
    expect(f.controller.getSnapshot().page).toBeNull()
    f.fetch.mockImplementation((url, options) => url === path
      ? Promise.resolve(Response.json({ code: 'REVIEW_FORBIDDEN' }, { status: 403 }))
      : fallback(url, options))
    await f.controller.load()
    expect(f.controller.getSnapshot()).toMatchObject({ page: null, busy: false })
    expect(f.controller.getSnapshot().notice).toContain('无法读取')
    for (const assignments of [[{ ...item, owner_id: other }], [item, item]]) {
      f.fetch.mockImplementation((url, options) => url === path
        ? Promise.resolve(Response.json({ assignments, next_cursor: '' }))
        : fallback(url, options))
      await f.controller.load()
      expect(f.controller.getSnapshot().page).toBeNull()
      expect(f.controller.getSnapshot().notice).toContain('无法读取')
    }
  } finally { f.stop() }
})
