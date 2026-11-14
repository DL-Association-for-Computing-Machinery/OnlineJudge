export function sameOriginJSONOptions(signal: AbortSignal, credentials: 'omit' | 'same-origin') {
  return {
    credentials, mode: 'same-origin', cache: 'no-store', redirect: 'error', referrerPolicy: 'no-referrer',
    headers: { Accept: 'application/json' },
    signal: AbortSignal.any([signal, AbortSignal.timeout(15_000)]),
  } satisfies RequestInit
}

// Endpoints own their error mapping and byte limits. Keep the caller's BOM policy.
export async function readBoundedJSON(
  response: Response, limit: number, invalid: (status?: number) => Error, ignoreBOM = true,
): Promise<unknown> {
  if (response.status !== 200 || response.headers.get('Content-Type')?.split(';')[0].trim() !== 'application/json' || !response.body) {
    await response.body?.cancel().catch(() => undefined)
    throw invalid(response.status)
  }
  const reader = response.body.getReader()
  const decoder = new TextDecoder('utf-8', { fatal: true, ignoreBOM })
  let size = 0
  let text = ''
  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      size += value.byteLength
      if (size > limit) throw invalid()
      text += decoder.decode(value, { stream: true })
    }
    return JSON.parse(text + decoder.decode()) as unknown
  } finally {
    await reader.cancel().catch(() => undefined)
    reader.releaseLock()
  }
}
