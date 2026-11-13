export type EmailRequestKind = 'registration' | 'recovery'
export type EmailRequestResult = 'accepted' | 'invalid' | 'closed' | 'unknown'

const endpoints = {
  registration: '/api/v1/account/register',
  recovery: '/api/v1/account/request-password-reset',
} as const

export async function requestEmail(
  kind: EmailRequestKind, email: string, signal: AbortSignal, send: typeof fetch = globalThis.fetch,
): Promise<EmailRequestResult> {
  try {
    const response = await send(endpoints[kind], {
      method: 'POST',
      credentials: 'omit',
      cache: 'no-store',
      redirect: 'error',
      // Firefox may send Origin: null with no-referrer on a POST.
      referrerPolicy: 'origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email }),
      signal,
    })
    if (response.status === 202) return 'accepted'
    if (response.status === 400 || response.status === 413) return 'invalid'
    if (response.status === 404) return 'closed'
  } catch {
    // A missing receipt cannot prove the server discarded the request.
  }
  return 'unknown'
}
