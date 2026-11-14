import { decodeDraftPreview, readBoundedJSON, validDraftID } from './privateDraft'

const record = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v)
const version = (v: unknown): v is string => typeof v === 'string' && /^[a-f0-9]{64}$/.test(v)
const invalid = () => new Error('Invalid catalog response')

function request(path: string, signal: AbortSignal) {
  return fetch(path, { mode: 'same-origin', credentials: 'omit', cache: 'no-store', redirect: 'error', referrerPolicy: 'no-referrer',
    headers: { Accept: 'application/json' }, signal: AbortSignal.any([signal, AbortSignal.timeout(15_000)]) })
}

export async function listCatalog(query: string, after: string, signal: AbortSignal) {
  const params = new URLSearchParams({ ...(query ? { q: query } : {}), ...(after ? { after } : {}) })
  const suffix = params.size ? '?' + params.toString() : ''
  const data = await readBoundedJSON(await request('/api/v1/problems' + suffix, signal), 64 * 1024)
  if (!record(data) || Object.keys(data).length !== 2 || !Array.isArray(data.problems) || data.problems.length > 20 ||
    typeof data.next !== 'string' || (data.next !== '' && (!validDraftID(data.next) || data.problems.length !== 20))) throw invalid()
  const entries = (data.problems as unknown[]).map(row => {
    if (!record(row) || Object.keys(row).length !== 3 || typeof row.id !== 'string' || !validDraftID(row.id) ||
      !version(row.version) || typeof row.title !== 'string' || !row.title || new TextEncoder().encode(row.title).length > 512) throw invalid()
    return { id: row.id, version: row.version, title: row.title }
  })
  if (entries.some((row, i) => row.id <= (entries[i - 1]?.id ?? after)) || (data.next && data.next !== entries.at(-1)?.id)) throw invalid()
  return { entries, next: data.next }
}

export async function readCatalog(id: string, signal: AbortSignal) {
  if (!validDraftID(id)) return null
  const response = await request('/api/v1/problems/' + id, signal)
  if (response.status === 404) return null
  const data = await readBoundedJSON(response, 7 * 1024 * 1024)
  if (!record(data) || data.problem_id !== id || 'draft_id' in data) throw invalid()
  const { problem_id, ...display } = data
  return decodeDraftPreview({ ...display, draft_id: problem_id }, id)
}
