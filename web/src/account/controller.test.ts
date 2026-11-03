import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AccountController } from './controller'
import * as privateInvalidation from './privateInvalidation'

const view = { subject_id: 'a'.repeat(32), created_at: '2026-11-01T00:00:00Z', idle_expires_at: '2026-11-01T02:00:00Z', absolute_deadline: '2026-11-01T12:00:00Z' }
const problem = (status: number, code: string) => Response.json({ type: 'about:blank', status, code }, { status, headers: { 'Content-Type': 'application/problem+json' } })
const empty = (status = 204) => new Response(null, { status })
const input = { email: 'test@example.test', password: 'only-synthetic-input' }
const change = { current_password: 'synthetic-current', new_password: 'synthetic-replacement' }

function deferred() {
  let resolve!: (response: Response) => void
  const promise = new Promise<Response>(accept => { resolve = accept })
  return { promise, resolve }
}

async function ready(response: Response) {
  const fetch = vi.fn<typeof globalThis.fetch>().mockResolvedValueOnce(response)
  vi.stubGlobal('fetch', fetch)
  const controller = new AccountController()
  controller.connect()
  await vi.waitFor(() => expect(controller.getSnapshot().busy).toBe(false))
  return { controller, fetch }
}

beforeEach(() => vi.stubGlobal('location', { protocol: 'https:' }))
afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })

describe('browser account lifecycle', () => {
  it('invalidates before a credential write and again after an unknown result; reads never broadcast', async () => {
    const notify = vi.spyOn(privateInvalidation, 'invalidatePrivateContent')
    const { controller, fetch } = await ready(Response.json(view))
    expect(notify).not.toHaveBeenCalled()
    let release!: (response: Response) => void
    fetch.mockImplementationOnce(() => new Promise(resolve => { release = resolve }))
    const operation = controller.run('logout')
    expect(notify).toHaveBeenCalledOnce()
    release(problem(503, 'LOGOUT_UNAVAILABLE')); await operation
    expect(notify).toHaveBeenCalledTimes(2)
    expect(controller.getSnapshot().status).toBe('unknown')
    controller.disconnect()
  })
  it('local shortcut uses the normal dependent chain without a browser password', async () => {
    const { controller, fetch } = await ready(problem(401, 'SESSION_REQUIRED'))
    fetch.mockResolvedValueOnce(empty(202)).mockResolvedValueOnce(empty()).mockResolvedValueOnce(Response.json(view))
    await controller.run('local-login')
    expect(fetch.mock.calls.map(call => call[0])).toEqual(['/api/v1/session', '/__local/login', '/api/v1/session/login/complete', '/api/v1/session'])
    expect(fetch.mock.calls[1][1]).not.toHaveProperty('body')
    expect(controller.getSnapshot().status).toBe('authenticated')
  })

  it('uncertain local begin never retries or skips to Complete', async () => {
    const { controller, fetch } = await ready(problem(401, 'SESSION_REQUIRED'))
    fetch.mockResolvedValueOnce(problem(503, 'LOGIN_UNAVAILABLE'))
    await controller.run('local-login')
    expect(fetch.mock.calls).toHaveLength(2)
    expect(controller.getSnapshot()).toMatchObject({ status: 'unknown', notice: { tone: 'uncertain' } })
  })

  it('pending local login continues the original attempt instead of beginning again', async () => {
    const { controller, fetch } = await ready(problem(401, 'SESSION_REQUIRED'))
    fetch.mockResolvedValueOnce(problem(409, 'LOGIN_PENDING'))
    await controller.run('local-login')
    fetch.mockResolvedValueOnce(empty()).mockResolvedValueOnce(Response.json(view))
    await controller.run('complete')
    expect(fetch.mock.calls.filter(call => call[0] === '/__local/login')).toHaveLength(1)
    expect(controller.getSnapshot().status).toBe('authenticated')
  })
  it('loads with a read only; an anonymous browser can still recover a pending HttpOnly attempt', async () => {
    const { controller, fetch } = await ready(problem(401, 'SESSION_REQUIRED'))
    expect(controller.getSnapshot().status).toBe('anonymous')
    expect(controller.getSnapshot().notice).toEqual({ tone: 'neutral', text: '尚未登录。请使用已有账号，或继续确认原尝试。' })
    expect(fetch.mock.calls.map(call => call[0])).toEqual(['/api/v1/session'])
    fetch.mockResolvedValueOnce(empty()).mockResolvedValueOnce(Response.json(view))
    await controller.run('complete')
    expect(controller.getSnapshot().status).toBe('authenticated')
    expect(fetch.mock.calls.map(call => call[0])).toEqual(['/api/v1/session', '/api/v1/session/login/complete', '/api/v1/session'])
  })

  it('holds one synchronous slot through Begin, Complete and the final Read', async () => {
    const { controller, fetch } = await ready(problem(401, 'SESSION_REQUIRED'))
    const begin = deferred()
    fetch.mockReturnValueOnce(begin.promise).mockResolvedValueOnce(empty()).mockResolvedValueOnce(Response.json(view))
    const first = controller.run('login', input)
    await controller.run('login', input)
    await controller.run('logout')
    expect(fetch).toHaveBeenCalledTimes(2)
    begin.resolve(empty(202))
    await first
    expect(fetch.mock.calls.map(call => call[0])).toEqual(['/api/v1/session', '/api/v1/session/login/begin', '/api/v1/session/login/complete', '/api/v1/session'])
    expect(controller.getSnapshot().status).toBe('authenticated')
    expect(JSON.stringify(controller.getSnapshot())).not.toContain(input.password)
  })

  it('does not continue after an unconfirmed Begin or automatically retry the password', async () => {
    const { controller, fetch } = await ready(problem(401, 'SESSION_REQUIRED'))
    fetch.mockResolvedValueOnce(problem(503, 'LOGIN_UNAVAILABLE'))
    await controller.run('login', input)
    expect(fetch).toHaveBeenCalledTimes(2)
    expect(controller.getSnapshot()).toMatchObject({ status: 'unknown', busy: false, notice: { tone: 'uncertain' } })
    expect(controller.getSnapshot().notice.text).toContain('原尝试')
  })

  it('retains confirmed issuance when its subsequent Read fails, and only reads on manual check', async () => {
    const { controller, fetch } = await ready(problem(401, 'SESSION_REQUIRED'))
    fetch.mockResolvedValueOnce(empty(202)).mockResolvedValueOnce(empty()).mockRejectedValueOnce(new Error('lost read'))
    await controller.run('login', input)
    expect(controller.getSnapshot().notice.text).toContain('登录发放已确认')
    fetch.mockResolvedValueOnce(Response.json(view))
    await controller.run('check')
    expect(fetch.mock.calls.filter(call => call[0] === '/api/v1/session/login/complete')).toHaveLength(1)
    expect(fetch.mock.calls.filter(call => call[0] === '/api/v1/session/activity')).toHaveLength(0)
    expect(controller.getSnapshot().status).toBe('authenticated')
  })

  it.each([[401, 'PASSWORD_CHANGE_REJECTED'], [404, 'NOT_FOUND']] as const)('does not treat password %s as ordinary session logout', async (status, code) => {
    const { controller, fetch } = await ready(Response.json(view))
    fetch.mockResolvedValueOnce(problem(status, code))
    await controller.run('password', change)
    expect(controller.getSnapshot()).toMatchObject({ status: 'authenticated', session: view, passwordUnknown: false, passwordAvailable: status !== 404 })
  })

  it('requires a fresh observation after Abandon, without pretending that a candidate cannot revoke its issued session', async () => {
    const { controller, fetch } = await ready(Response.json(view))
    fetch.mockResolvedValueOnce(empty())
    await controller.run('abandon')
    expect(controller.getSnapshot()).toMatchObject({ status: 'unknown', session: view })
    expect(controller.getSnapshot().notice.text).toContain('不等于确认退出')
  })

  it('also treats an unconfirmed Abandon as unknown without automatic read or retry', async () => {
    const { controller, fetch } = await ready(Response.json(view))
    fetch.mockResolvedValueOnce(problem(503, 'LOGIN_UNAVAILABLE'))
    await controller.run('abandon')
    expect(controller.getSnapshot()).toMatchObject({ status: 'unknown', session: view, busy: false })
    expect(fetch.mock.calls.map(call => call[0])).toEqual(['/api/v1/session', '/api/v1/session/login/abandon'])
  })

  it.each([[403, 'ORIGIN_REJECTED'], [405, 'METHOD_NOT_ALLOWED']] as const)('does not leave a rejected initial observation %s permanently checking', async (status, code) => {
    const { controller } = await ready(problem(status, code))
    expect(controller.getSnapshot()).toMatchObject({ status: 'unknown', busy: false, session: null, notice: { tone: 'error' } })
  })

  it('distinguishes an unusable session from a confirmed logout receipt', async () => {
    const { controller, fetch } = await ready(Response.json(view))
    fetch.mockResolvedValueOnce(problem(401, 'SESSION_REQUIRED'))
    await controller.run('logout')
    expect(controller.getSnapshot()).toMatchObject({ status: 'anonymous', session: null })
    expect(controller.getSnapshot().notice.text).toContain('不确认之前的退出操作')
  })

  it('keeps an unknown password result sticky across checks and blocks repeat password submission', async () => {
    const { controller, fetch } = await ready(Response.json(view))
    fetch.mockRejectedValueOnce(new Error('lost password response'))
    await controller.run('password', change)
    expect(controller.getSnapshot()).toMatchObject({ status: 'unknown', passwordUnknown: true })
    fetch.mockResolvedValueOnce(Response.json(view))
    await controller.run('check')
    await controller.run('password', change)
    expect(fetch.mock.calls.filter(call => call[0] === '/api/v1/account/password')).toHaveLength(1)
    expect(controller.getSnapshot().passwordUnknown).toBe(true)
    fetch.mockResolvedValueOnce(empty()).mockResolvedValueOnce(empty(202)).mockResolvedValueOnce(empty()).mockResolvedValueOnce(Response.json(view))
    await controller.run('logout')
    await controller.run('login', input)
    expect(controller.getSnapshot()).toMatchObject({ status: 'authenticated', passwordUnknown: false })
  })

  it('requires an explicit activity action and does not auto-read after confirmed password or logout writes', async () => {
    const { controller, fetch } = await ready(Response.json(view))
    fetch.mockResolvedValueOnce(Response.json(view)).mockResolvedValueOnce(empty())
    await controller.run('activity')
    await controller.run('password', change)
    expect(fetch.mock.calls.map(call => call[0])).toEqual(['/api/v1/session', '/api/v1/session/activity', '/api/v1/account/password'])
    expect(controller.getSnapshot()).toMatchObject({ status: 'anonymous', session: null })
    expect(controller.getSnapshot().notice.text).toContain('密码已修改')
  })

  it('does not report an unknown logout as a confirmed exit', async () => {
    const { controller, fetch } = await ready(Response.json(view))
    fetch.mockResolvedValueOnce(problem(503, 'SESSION_UNAVAILABLE'))
    await controller.run('logout')
    expect(controller.getSnapshot()).toMatchObject({ status: 'unknown', session: view })
    expect(controller.getSnapshot().notice.text).toContain('暂不能视为已退出')
    fetch.mockResolvedValueOnce(empty())
    await controller.run('logout')
    expect(controller.getSnapshot()).toMatchObject({ status: 'anonymous', session: null })
  })

  it('ignores stale results and finally handlers across StrictMode-style cleanup and reconnect', async () => {
    const first = deferred(), second = deferred()
    const fetch = vi.fn<typeof globalThis.fetch>().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
    vi.stubGlobal('fetch', fetch)
    const controller = new AccountController()
    controller.connect()
    controller.disconnect()
    controller.connect()
    first.resolve(Response.json(view))
    await new Promise(resolve => setTimeout(resolve, 0))
    expect(controller.getSnapshot()).toMatchObject({ status: 'checking', busy: true })
    second.resolve(problem(401, 'SESSION_REQUIRED'))
    await vi.waitFor(() => expect(controller.getSnapshot().busy).toBe(false))
    expect(controller.getSnapshot()).toMatchObject({ status: 'anonymous', session: null })
    expect(fetch).toHaveBeenCalledTimes(2)
  })

  it('does not let late password success overwrite a newer observation after unmount', async () => {
    const { controller, fetch } = await ready(Response.json(view))
    const write = deferred()
    fetch.mockReturnValueOnce(write.promise).mockResolvedValueOnce(Response.json(view))
    const changing = controller.run('password', change)
    controller.disconnect()
    controller.connect()
    await vi.waitFor(() => expect(controller.getSnapshot().busy).toBe(false))
    write.resolve(empty())
    await changing
    expect(controller.getSnapshot()).toMatchObject({ status: 'authenticated', session: view, passwordUnknown: true, busy: false })
  })
})
