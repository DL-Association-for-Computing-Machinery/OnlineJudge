const activation = '/account/activate'
const recovery = '/account/reset-password'

export type EmailLinkKind = 'activation' | 'recovery'
export type ConfirmationResult = 'done' | 'invalid-link' | 'invalid-password' | 'weak-password' | 'limited' | 'closed' | 'unknown'

export type EmailConfirmationPageInput = {
  kind: EmailLinkKind
  confirm?: (password: string) => Promise<ConfirmationResult>
}

// Run before React or the router sees the location. Retain the fragment only
// in a closure, and only while a known rejection allows an explicit retry.
export function captureEmailConfirmation(
  location: Pick<Location, 'pathname' | 'protocol' | 'search' | 'hash'>,
  replace: (path: string) => void,
  send: typeof fetch = globalThis.fetch,
): EmailConfirmationPageInput | undefined {
  const { pathname, protocol, search, hash } = location
  if (pathname !== activation && pathname !== recovery) return
  const kind = pathname === activation ? 'activation' : 'recovery'
  if (hash || search) {
    try {
      replace(pathname)
    } catch {
      return { kind }
    }
  }
  // The server issues a canonical base64url encoding of exactly 32 bytes.
  if (protocol !== 'https:' || search || hash.length !== 50 || !/^#token=[A-Za-z0-9_-]{42}[AEIMQUYcgkosw048]$/.test(hash)) return { kind }
  let token = hash.slice('#token='.length)
  let pending = false
  return {
    kind,
    async confirm(password) {
      if (!token || pending) return 'unknown'
      pending = true
      try {
        const result = await postConfirmation(send, '/api/v1' + pathname, token, password)
        if (result !== 'weak-password' && result !== 'invalid-password' && result !== 'limited') token = ''
        return result
      } finally {
        pending = false
      }
    },
  }
}

async function postConfirmation(send: typeof fetch, endpoint: string, token: string, password: string): Promise<ConfirmationResult> {
  try {
    const body = JSON.stringify({ token, new_password: password })
    if (new TextEncoder().encode(body).byteLength > 4096) return 'invalid-password'
    const response = await send(endpoint, {
      method: 'POST',
      mode: 'same-origin',
      credentials: 'omit',
      cache: 'no-store',
      redirect: 'error',
      // Firefox may send Origin: null with no-referrer on a POST.
      referrerPolicy: 'origin',
      headers: { 'Content-Type': 'application/json' },
      body,
      signal: AbortSignal.timeout(12_000),
    })
    if (response.status === 204) return 'done'
    if (response.status === 429) return 'limited'
    if (response.status === 404) return 'closed'
    if (response.status === 413) return 'invalid-password'
    if (response.status === 400) {
      const code = await problemCode(response)
      if (code === 'EMAIL_LINK_UNAVAILABLE') return 'invalid-link'
      if (code === 'WEAK_PASSWORD') return 'weak-password'
      if (code === 'INVALID_NEW_PASSWORD') return 'invalid-password'
    }
    return 'unknown'
  } catch {
    return 'unknown'
  }
}

async function problemCode(response: Response): Promise<unknown> {
  if (response.headers.get('Content-Type')?.split(';')[0].trim() !== 'application/problem+json' || !response.body) return
  const bytes = new Uint8Array(1024)
  const reader = response.body.getReader()
  let offset = 0
  try {
    for (;;) {
      const chunk = await reader.read()
      if (chunk.done) break
      if (chunk.value.length > bytes.length - offset) return
      bytes.set(chunk.value, offset)
      offset += chunk.value.length
    }
    const value: unknown = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes.subarray(0, offset)))
    return typeof value === 'object' && value !== null && 'code' in value && 'status' in value && value.status === 400 ? value.code : undefined
  } finally {
    void reader.cancel().catch(() => undefined)
    reader.releaseLock()
  }
}
