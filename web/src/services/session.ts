export interface SessionView {
  subject_id: string
  created_at: string
  idle_expires_at: string
  absolute_deadline: string
}

export type AccountOperation = 'read' | 'activity' | 'logout' | 'begin' | 'local-begin' | 'complete' | 'abandon' | 'password' | 'replace-device'
export type AccountInput = { email: string; password: string; remember?: boolean } | { current_password: string; new_password: string } | { device: string }

const endpoints: Record<AccountOperation, { path: string; method: 'GET' | 'POST'; status: number }> = {
  read: { path: '/api/v1/session', method: 'GET', status: 200 },
  activity: { path: '/api/v1/session/activity', method: 'POST', status: 200 },
  logout: { path: '/api/v1/session/logout', method: 'POST', status: 204 },
  begin: { path: '/api/v1/session/login/begin', method: 'POST', status: 202 },
  'local-begin': { path: '/__local/login', method: 'POST', status: 202 },
  complete: { path: '/api/v1/session/login/complete', method: 'POST', status: 204 },
  abandon: { path: '/api/v1/session/login/abandon', method: 'POST', status: 204 },
  password: { path: '/api/v1/account/password', method: 'POST', status: 204 },
  'replace-device': { path: '/api/v1/session/login/replace-device', method: 'POST', status: 204 },
}

// Messages are local constants. Never render response titles, HTML or caught
// fetch errors, which can contain server details or untrusted reflected input.
const refusals: Record<string, [number, string]> = {
  SESSION_REQUIRED: [401, '会话无效或已过期，请重新登录。'],
  NOT_FOUND: [404, '此入口尚未启用，请联系维护者。'],
  ORIGIN_REJECTED: [403, '当前访问来源不允许，请使用正式 HTTPS 入口。'],
  METHOD_NOT_ALLOWED: [405, '当前入口不支持此操作。'],
  INVALID_LOGIN: [400, '邮箱或密码输入格式不符合要求。'],
  LOGIN_ATTEMPT_INVALID: [400, '未找到可用的登录尝试，请先确认是否需要放弃原尝试。'],
  LOGIN_PENDING: [409, '还有未完成的登录，请继续确认或放弃原尝试。'],
  ALREADY_AUTHENTICATED: [409, '浏览器已有有效会话，请检查当前会话。'],
  SESSION_LIMIT: [409, '有效会话已达上限。请在已登录设备退出，再确认本次原尝试。'],
  LOGIN_ATTEMPT_CONFLICT: [409, '原登录尝试已变化，请勿重新创建相同操作重试。'],
  LOGIN_REJECTED: [401, '邮箱、密码或原登录尝试无效。'],
  LOGIN_RATE_LIMITED: [429, '登录请求过于频繁，请稍后手动操作。'],
  WEAK_PASSWORD: [400, '新密码过于常见，请换一个密码。'],
  INVALID_NEW_PASSWORD: [400, '新密码长度不符合本站要求。'],
  INVALID_PASSWORD_INPUT: [400, '当前或新密码的输入长度不符合要求。'],
  PASSWORD_UNCHANGED: [400, '新密码不能与当前密码相同。'],
  INVALID_PASSWORD_CHANGE: [400, '改密请求格式不符合要求。'],
  PASSWORD_CHANGE_CONFLICT: [409, '账号状态已变化，请退出并重新登录后操作。'],
  PASSWORD_CHANGE_REJECTED: [401, '请核对当前密码；也可能是会话已失效，可检查当前会话。'],
  PASSWORD_CHANGE_RATE_LIMITED: [429, '改密请求过于频繁，请稍后手动操作。'],
  BODY_TOO_LARGE: [413, '输入内容过长，请缩短后操作。'],
  UNSUPPORTED_MEDIA_TYPE: [415, '请求格式不受支持。'],
}

const codes: Record<AccountOperation, readonly string[]> = {
  read: ['SESSION_REQUIRED'], activity: ['SESSION_REQUIRED'], logout: ['SESSION_REQUIRED'],
  begin: ['INVALID_LOGIN', 'LOGIN_ATTEMPT_INVALID', 'LOGIN_PENDING', 'ALREADY_AUTHENTICATED', 'LOGIN_REJECTED', 'LOGIN_RATE_LIMITED', 'BODY_TOO_LARGE', 'UNSUPPORTED_MEDIA_TYPE'],
  'local-begin': ['INVALID_LOGIN', 'LOGIN_ATTEMPT_INVALID', 'LOGIN_PENDING', 'ALREADY_AUTHENTICATED', 'LOGIN_REJECTED', 'LOGIN_RATE_LIMITED', 'BODY_TOO_LARGE', 'UNSUPPORTED_MEDIA_TYPE'],
  complete: ['LOGIN_ATTEMPT_INVALID', 'ALREADY_AUTHENTICATED', 'SESSION_LIMIT', 'LOGIN_ATTEMPT_CONFLICT', 'LOGIN_REJECTED', 'LOGIN_RATE_LIMITED'],
  'replace-device': ['LOGIN_ATTEMPT_INVALID', 'ALREADY_AUTHENTICATED', 'SESSION_LIMIT', 'LOGIN_ATTEMPT_CONFLICT', 'LOGIN_REJECTED', 'LOGIN_RATE_LIMITED'],
  abandon: ['LOGIN_ATTEMPT_INVALID', 'LOGIN_ATTEMPT_CONFLICT', 'LOGIN_REJECTED', 'LOGIN_RATE_LIMITED'],
  password: ['WEAK_PASSWORD', 'INVALID_NEW_PASSWORD', 'INVALID_PASSWORD_INPUT', 'PASSWORD_UNCHANGED', 'INVALID_PASSWORD_CHANGE', 'PASSWORD_CHANGE_CONFLICT', 'PASSWORD_CHANGE_REJECTED', 'PASSWORD_CHANGE_RATE_LIMITED', 'BODY_TOO_LARGE', 'UNSUPPORTED_MEDIA_TYPE'],
}

export class AccountError extends Error {
  constructor(readonly kind: 'unknown' | 'refused' | 'insecure', readonly code: string, message: string) {
    super(message)
    this.name = 'AccountError'
  }
}

function unknownResult(): AccountError {
  return new AccountError('unknown', 'RESULT_UNCONFIRMED', '结果暂时无法确认，请手动检查，勿重复提交。')
}

async function boundedBytes(response: Response, limit = 16_384): Promise<Uint8Array> {
  if (!response.body) return new Uint8Array()
  const reader = response.body.getReader()
  let size = 0
  const chunks: Uint8Array[] = []
  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      size += value.byteLength
      if (size > limit) throw unknownResult()
      chunks.push(value)
    }
  } finally {
    // Cancel unread excess data without letting cancellation mask the result.
    await reader.cancel().catch(() => undefined)
    reader.releaseLock()
  }
  const bytes = new Uint8Array(size)
  let offset = 0
  for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.byteLength }
  return bytes
}

async function boundedJSON(response: Response): Promise<unknown> {
  return JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(await boundedBytes(response))) as unknown
}

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export interface SessionDevice extends Omit<SessionView, 'subject_id'> {
  id: string
  current: boolean
}

export interface SessionDevices {
  subject_id: string
  devices: SessionDevice[]
}

const deviceID = /^[a-f0-9]{64}$/
const devicesPath = '/api/v1/session/devices'

function deviceList(value: unknown): SessionDevices {
  if (!record(value) || Object.keys(value).length !== 2 || typeof value.subject_id !== 'string' ||
      !Array.isArray(value.devices) || value.devices.length < 1 || value.devices.length > 3) throw unknownResult()
  return { subject_id: value.subject_id, devices: deviceRows(value.devices, value.subject_id, 1) }
}

function deviceRows(rows: unknown[], subject: string, currentCount: number): SessionDevice[] {
  const ids = new Set<string>()
  let current = 0
  const devices = rows.map(row => {
    if (!record(row) || Object.keys(row).length !== 5 || typeof row.id !== 'string' ||
        !deviceID.test(row.id) || ids.has(row.id) || typeof row.current !== 'boolean') throw unknownResult()
    ids.add(row.id)
    if (row.current) current++
    const { created_at, idle_expires_at, absolute_deadline } = sessionView({
      subject_id: subject, created_at: row.created_at,
      idle_expires_at: row.idle_expires_at, absolute_deadline: row.absolute_deadline,
    })
    return { id: row.id, current: row.current, created_at, idle_expires_at, absolute_deadline }
  })
  if (current !== currentCount) throw unknownResult()
  return devices
}

export async function pendingDevicesRequest(signal: AbortSignal): Promise<SessionDevice[]> {
  requireHTTPS()
  try {
    const res = await fetch('/api/v1/session/login/devices', accountFetchOptions('POST', signal))
    if (res.status !== 200 || res.headers.get('Content-Type')?.split(';')[0].trim() !== 'application/json') {
      await res.body?.cancel().catch(() => undefined)
      throw unknownResult()
    }
    const result = await boundedJSON(res)
    if (!record(result) || Object.keys(result).length !== 1 || !Array.isArray(result.devices) || result.devices.length !== 3) throw unknownResult()
    return deviceRows(result.devices, '0'.repeat(32), 0)
  } catch {
    throw unknownResult()
  }
}

type DeviceRevocation = { current: string; target?: string }

export function sessionDevicesRequest(signal: AbortSignal): Promise<SessionDevices>
export function sessionDevicesRequest(signal: AbortSignal, revoke: DeviceRevocation): Promise<void>
export async function sessionDevicesRequest(signal: AbortSignal, revoke?: DeviceRevocation): Promise<SessionDevices | void> {
  requireHTTPS()
  const headers: Record<string, string> = { Accept: 'application/json, application/problem+json' }
  if (revoke) {
    if (!deviceID.test(revoke.current) || (revoke.target !== undefined && !deviceID.test(revoke.target))) throw unknownResult()
    headers['X-OJ-Session-Device'] = revoke.current
    if (revoke.target !== undefined) headers['Content-Type'] = 'text/plain'
  }
  try {
    const res = await fetch(devicesPath + (revoke ? (revoke.target ? '/revoke' : '/revoke-others') : ''), {
      ...accountFetchOptions(revoke ? 'POST' : 'GET', signal), headers, body: revoke?.target,
    })
    if (revoke && res.status === 204) {
      await boundedBytes(res, 0)
      return
    }
    const type = res.headers.get('Content-Type')?.split(';')[0].trim()
    if (!revoke && res.status === 200 && type === 'application/json') return deviceList(await boundedJSON(res))
    if (type === 'application/problem+json' && [401, 404, 409].includes(res.status)) {
      const body = await boundedJSON(res)
      if (record(body) && body.type === 'about:blank' && body.status === res.status) {
        if (res.status === 401 && body.code === 'SESSION_REQUIRED') throw new AccountError('refused', body.code, refusals.SESSION_REQUIRED[1])
        if (res.status === 404 && body.code === 'NOT_FOUND') throw new AccountError('refused', body.code, '此入口尚未启用管理其他会话。')
        if (revoke && ((res.status === 409 && body.code === 'DEVICE_SESSION_CHANGED') ||
            (res.status === 404 && body.code === 'DEVICE_NOT_FOUND'))) {
          throw new AccountError('refused', body.code, '会话已变化，请重新读取列表。')
        }
      }
    } else {
      await res.body?.cancel().catch(() => undefined)
    }
    throw unknownResult()
  } catch (error) {
    if (error instanceof AccountError) throw error
    throw unknownResult()
  }
}

function requireHTTPS() {
  if (globalThis.location.protocol !== 'https:') {
    throw new AccountError('insecure', 'HTTPS_REQUIRED', '账号操作需要 HTTPS，请通过安全入口访问。')
  }
}

function accountFetchOptions(method: 'GET' | 'POST', signal: AbortSignal): RequestInit {
  return {
    method, mode: 'same-origin', credentials: 'same-origin', cache: 'no-store', redirect: 'error',
    // Firefox needs origin policy for a non-null Origin on same-origin POST.
    referrerPolicy: method === 'POST' ? 'origin' : 'no-referrer',
    signal: AbortSignal.any([signal, AbortSignal.timeout(10_000)]),
  }
}

function sessionView(value: unknown): SessionView {
  if (!record(value) || Object.keys(value).length !== 4 ||
      typeof value.subject_id !== 'string' || !/^[a-f0-9]{32}$/.test(value.subject_id)) throw unknownResult()
  const fields = ['created_at', 'idle_expires_at', 'absolute_deadline'] as const
  for (const field of fields) {
    const date = value[field]
    if (typeof date !== 'string' || date.length > 40 ||
        !/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(date) || !Number.isFinite(Date.parse(date))) throw unknownResult()
  }
  const result = value as unknown as SessionView
  if (Date.parse(result.created_at) >= Date.parse(result.idle_expires_at) ||
      Date.parse(result.idle_expires_at) > Date.parse(result.absolute_deadline)) throw unknownResult()
  return result
}

export async function accountRequest(operation: 'read' | 'activity', signal: AbortSignal): Promise<SessionView>
export async function accountRequest(operation: Exclude<AccountOperation, 'read' | 'activity'>, signal: AbortSignal, input?: AccountInput): Promise<void>
export async function accountRequest(operation: AccountOperation, signal: AbortSignal, input?: AccountInput): Promise<SessionView | void> {
  // A localhost exception for Secure cookies is not permission to send passwords
  // over HTTP. Gate before constructing or submitting a body.
  requireHTTPS()
  const endpoint = endpoints[operation]
  const replacement = operation === 'replace-device'
  const device = input && 'device' in input ? input.device : undefined
  if (replacement && (!device || !deviceID.test(device))) throw unknownResult()
  try {
    const response = await fetch(endpoint.path, {
      ...accountFetchOptions(endpoint.method, signal),
      headers: input ? { 'Content-Type': replacement ? 'text/plain' : 'application/json', Accept: 'application/json, application/problem+json' } : { Accept: 'application/json, application/problem+json' },
      ...(input ? { body: replacement ? device : JSON.stringify(input) } : {}),
    })
    if (response.status === endpoint.status) {
      if (operation !== 'read' && operation !== 'activity') {
        if ((await boundedBytes(response, 0)).length !== 0) throw unknownResult()
        return
      }
      if (response.headers.get('Content-Type')?.split(';')[0].trim() !== 'application/json') {
        await response.body?.cancel().catch(() => undefined)
        throw unknownResult()
      }
      return sessionView(await boundedJSON(response))
    }
    // Every 5xx and unexpected successful response remains uncertain, without
    // trusting a body that happens to name a deterministic refusal.
    if (response.status >= 500 || response.ok || response.headers.get('Content-Type')?.split(';')[0].trim() !== 'application/problem+json') {
      await response.body?.cancel().catch(() => undefined)
      throw unknownResult()
    }
    const body = await boundedJSON(response)
    if (record(body) && body.type === 'about:blank' && body.status === response.status && typeof body.code === 'string' && Object.hasOwn(refusals, body.code) &&
        (codes[operation].includes(body.code) || ['NOT_FOUND', 'ORIGIN_REJECTED', 'METHOD_NOT_ALLOWED'].includes(body.code))) {
      const [status, message] = refusals[body.code]
      if (status === response.status) throw new AccountError('refused', body.code, message)
    }
    throw unknownResult()
  } catch (error) {
    if (error instanceof AccountError) throw error
    throw unknownResult()
  }
}
