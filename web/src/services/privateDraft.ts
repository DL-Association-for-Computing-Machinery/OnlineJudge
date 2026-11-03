export interface DraftPreview {
  draft_id: string
  version: string
  title: string
  language: 'en'
  markdown: string
  examples: { id: string; input: string; output: string }[]
}
export const validDraftID = (id: string) => /^[a-f0-9]{32}$/.test(id)
const encoder = new TextEncoder()
const decoder = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true })
const invalid = () => new Error('草稿预览暂不可用，请检查权限、服务状态或稍后手动重试。')
const record = (value: unknown): value is Record<string, unknown> => typeof value === 'object' && value !== null && !Array.isArray(value)

function text(value: unknown): value is string {
  return typeof value === 'string' && decoder.decode(encoder.encode(value)) === value
}

export function decodeDraftPreview(value: unknown, id: string): DraftPreview {
  if (!record(value) || Object.keys(value).length !== 6 || value.draft_id !== id || !validDraftID(id) ||
    typeof value.version !== 'string' || !/^[a-f0-9]{64}$/.test(value.version) || !text(value.title) || !value.title || encoder.encode(value.title).byteLength > 512 || value.language !== 'en' ||
    !text(value.markdown) || !Array.isArray(value.examples) || value.examples.length > 1024) throw invalid()
  let bytes = encoder.encode(value.markdown).byteLength
  const ids = new Set<string>()
  for (const example of value.examples) {
    if (!record(example) || Object.keys(example).length !== 3 || typeof example.id !== 'string' || !/^case-\d{4}$/.test(example.id) || ids.has(example.id) || !text(example.input) || !text(example.output)) throw invalid()
    ids.add(example.id)
    bytes += encoder.encode(example.input).byteLength + encoder.encode(example.output).byteLength
  }
  if (bytes > 1_048_576) throw invalid()
  return value as unknown as DraftPreview
}

export async function fetchDraftPreview(id: string, signal: AbortSignal): Promise<DraftPreview> {
  if (!validDraftID(id) || globalThis.location.protocol !== 'https:') throw invalid()
  try {
    const response = await fetch(`/api/v1/problem-drafts/${id}/preview`, {
      method: 'GET', mode: 'same-origin', credentials: 'same-origin', cache: 'no-store', redirect: 'error', referrerPolicy: 'no-referrer',
      headers: { Accept: 'application/json' }, signal: AbortSignal.any([signal, AbortSignal.timeout(15_000)]),
    })
    if (response.status !== 200 || response.headers.get('Content-Type')?.split(';')[0].trim() !== 'application/json' || !response.body) {
      await response.body?.cancel().catch(() => undefined)
      throw invalid()
    }
    const reader = response.body.getReader()
    const chunks: Uint8Array[] = []
    let size = 0
    try {
      for (;;) {
        const { done, value } = await reader.read()
        if (done) break
        size += value.byteLength
        // JSON escaping can expand the API's 1 MiB raw text ceiling sixfold.
        if (size > 7 * 1024 * 1024) throw invalid()
        chunks.push(value)
      }
    } finally {
      await reader.cancel().catch(() => undefined)
      reader.releaseLock()
    }
    const body = new Uint8Array(size)
    let offset = 0
    for (const chunk of chunks) { body.set(chunk, offset); offset += chunk.byteLength }
    return decodeDraftPreview(JSON.parse(decoder.decode(body)) as unknown, id)
  } catch { throw invalid() }
}
