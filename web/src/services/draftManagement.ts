import { validDraftID } from './privateDraft'

export interface ImportSource { source_id: string; title: string; version: string; import_revision: string }
export interface ImportReceipt {
  draft_id: string; request_key: string; source_id: string; version: string; import_revision: string
  state: 'private-draft'; validation_state: 'not-run'; created_at: string
}
export interface ImportTuple { owner: string; source: string; key: string; revision: string }
export interface OwnDraftPage { drafts: ImportReceipt[]; next_cursor: string }
export const validRevision = (value: unknown): value is string => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value)
export const validKey = (value: unknown): value is string => typeof value === 'string' && /^[A-Za-z0-9_-]{16,128}$/.test(value)
const wellFormed = (value: string) => new TextDecoder('utf-8', { ignoreBOM: true }).decode(new TextEncoder().encode(value)) === value
export const validSource = (value: unknown): value is string => typeof value === 'string' && value.length > 0 && new TextEncoder().encode(value).byteLength <= 128 && wellFormed(value)
export const object = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v)
export class ImportError extends Error {
  constructor(public code = 'UNCONFIRMED') { super('导入结果尚未确认，请保留原记录并手动查询回执。') }
}

export function decodeReceipt(value: unknown, tuple: ImportTuple): ImportReceipt {
  if (!object(value) || Object.keys(value).length !== 8 || typeof value.draft_id !== 'string' || !validDraftID(value.draft_id) ||
    value.request_key !== tuple.key || value.source_id !== tuple.source || value.import_revision !== tuple.revision ||
    !validRevision(value.version) || value.state !== 'private-draft' || value.validation_state !== 'not-run' ||
    typeof value.created_at !== 'string' || !/^\d{4}-\d\d-\d\dT/.test(value.created_at) || !Number.isFinite(Date.parse(value.created_at))) throw new ImportError()
  return value as unknown as ImportReceipt
}

export function decodeSources(value: unknown): ImportSource[] {
  if (!object(value) || Object.keys(value).length !== 1 || !Array.isArray(value.sources) || value.sources.length < 1 || value.sources.length > 10) throw new ImportError()
  const ids = new Set<string>()
  for (const s of value.sources) {
    if (!object(s) || Object.keys(s).length !== 4 || !validSource(s.source_id) || ids.has(s.source_id) ||
      typeof s.title !== 'string' || !s.title || new TextEncoder().encode(s.title).byteLength > 512 || !wellFormed(s.title) ||
      !validRevision(s.version) || !validRevision(s.import_revision)) throw new ImportError()
    ids.add(s.source_id)
  }
  return value.sources as ImportSource[]
}

export function decodeOwnDraftPage(value: unknown, owner: string): OwnDraftPage {
  if (!validDraftID(owner) || !object(value) || Object.keys(value).length !== 2 || !Array.isArray(value.drafts) || value.drafts.length > 20 ||
    typeof value.next_cursor !== 'string' || value.next_cursor !== '' && (!/^[A-Za-z0-9_-]{1,192}$/.test(value.next_cursor) || value.drafts.length !== 20)) throw new ImportError()
  const ids = new Set<string>()
  const keys = new Set<string>()
  const drafts = value.drafts.map((row: unknown) => {
    if (!object(row) || !validKey(row.request_key) || !validSource(row.source_id) || !validRevision(row.import_revision)) throw new ImportError()
    const receipt = decodeReceipt(row, { owner, key: row.request_key, source: row.source_id, revision: row.import_revision })
    if (ids.has(receipt.draft_id) || keys.has(receipt.request_key)) throw new ImportError()
    ids.add(receipt.draft_id)
    keys.add(receipt.request_key)
    return receipt
  })
  return { drafts, next_cursor: value.next_cursor }
}

const base = '/api/v1/problem-draft-management'
// No compatibility fallback: old servers must refuse this namespace rather than
// silently ignoring the subject precondition on an older import endpoint.
async function request(owner: string, path: string, signal: AbortSignal, body?: object): Promise<unknown> {
  if (!validDraftID(owner) || location.protocol !== 'https:') throw new ImportError()
  const method = body ? 'POST' : 'GET'
  const response = await fetch(base + path, {
    method, credentials: 'same-origin', mode: 'same-origin', cache: 'no-store', redirect: 'error',
    // Preserve the browser-generated Origin for writes without sending a page path.
    referrerPolicy: method === 'POST' ? 'origin' : 'no-referrer',
    headers: { Accept: 'application/json', 'X-OJ-Expected-Subject': owner, ...(body ? { 'Content-Type': 'application/json' } : {}) },
    ...(body ? { body: JSON.stringify(body) } : {}), signal: AbortSignal.any([signal, AbortSignal.timeout(15_000)]),
  })
  const reader = response.body?.getReader()
  if (!reader) throw new ImportError()
  const chunks: Uint8Array[] = []; let size = 0
  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      size += value.byteLength
      if (size > 32_768) throw new ImportError()
      chunks.push(value)
    }
  } finally { await reader.cancel().catch(() => undefined); reader.releaseLock() }
  if (response.headers.get('Content-Type')?.split(';')[0].trim() !== (response.status === 200 ? 'application/json' : 'application/problem+json')) throw new ImportError()
  const bytes = new Uint8Array(size); let offset = 0
  for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.length }
  const data: unknown = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes))
  if (response.status !== 200) {
    const known: Record<string, number> = { SESSION_REQUIRED: 401, PROBLEM_AUTHOR_REQUIRED: 403, DRAFT_NOT_FOUND: 404, DRAFT_SUBJECT_CHANGED: 409, DRAFT_SOURCE_CHANGED: 409, DRAFT_REQUEST_CONFLICT: 409, DRAFT_CURSOR_INVALID: 400 }
    throw new ImportError(object(data) && typeof data.code === 'string' && known[data.code] === response.status ? data.code : 'UNCONFIRMED')
  }
  return data
}
export const fetchImportSources = async (owner: string, signal: AbortSignal) => decodeSources(await request(owner, '/sources', signal))
export const fetchImportReceipt = async (tuple: ImportTuple, signal: AbortSignal) => decodeReceipt(await request(tuple.owner, `/imports/${tuple.key}`, signal), tuple)
export const postImport = async (tuple: ImportTuple, signal: AbortSignal) => decodeReceipt(await request(tuple.owner, '/imports', signal, {
  source_id: tuple.source, request_key: tuple.key, expected_revision: tuple.revision,
}), tuple)
export async function fetchOwnDraftPage(owner: string, cursor: string, signal: AbortSignal): Promise<OwnDraftPage> {
  if (cursor !== '' && !/^[A-Za-z0-9_-]{1,192}$/.test(cursor)) throw new ImportError('DRAFT_CURSOR_INVALID')
  // Cursor bytes are opaque: never round the PG microsecond boundary through JS Date/Number.
  const page = decodeOwnDraftPage(await request(owner, '/drafts' + (cursor ? '?cursor=' + cursor : ''), signal), owner)
  if (cursor && page.next_cursor === cursor) throw new ImportError()
  return page
}
