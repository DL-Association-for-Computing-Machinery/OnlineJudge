import { decodeStatement, validStatementID } from '../statement/payload'
import { readBoundedJSON, sameOriginJSONOptions } from '../transport/json'

const record = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v)
const version = (v: unknown): v is string => typeof v === 'string' && /^[a-f0-9]{64}$/.test(v)
const invalid = () => new Error('Invalid catalog response')

function request(path: string, signal: AbortSignal) {
  return fetch(path, sameOriginJSONOptions(signal, 'omit'))
}

export async function listCatalog(query: string, after: string, signal: AbortSignal) {
  const params = new URLSearchParams({ ...(query ? { q: query } : {}), ...(after ? { after } : {}) })
  const suffix = params.size ? '?' + params.toString() : ''
  const data = await readBoundedJSON(await request('/api/v1/problems' + suffix, signal), 64 * 1024, invalid)
  if (!record(data) || Object.keys(data).length !== 2 || !Array.isArray(data.problems) || data.problems.length > 20 ||
    typeof data.next !== 'string' || (data.next !== '' && (!validStatementID(data.next) || data.problems.length !== 20))) throw invalid()
  const entries = (data.problems as unknown[]).map(row => {
    if (!record(row) || Object.keys(row).length !== 3 || typeof row.id !== 'string' || !validStatementID(row.id) ||
      !version(row.version) || typeof row.title !== 'string' || !row.title || new TextEncoder().encode(row.title).length > 512) throw invalid()
    return { id: row.id, version: row.version, title: row.title }
  })
  if (entries.some((row, i) => row.id <= (entries[i - 1]?.id ?? after)) || (data.next && data.next !== entries.at(-1)?.id)) throw invalid()
  return { entries, next: data.next }
}

export async function readCatalog(id: string, signal: AbortSignal) {
  if (!validStatementID(id)) return null
  const response = await request('/api/v1/problems/' + id, signal)
  if (response.status === 404) return null
  const data = await readBoundedJSON(response, 7 * 1024 * 1024, invalid)
  const { problem_id, ...display } = decodeStatement(data, id, 'problem_id', invalid)
  return { ...display, draft_id: problem_id,
    trainingIntake: response.headers.get('X-OJ-Training-Intake') === 'enabled' }
}
