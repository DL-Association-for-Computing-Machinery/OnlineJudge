import { object, validKey } from './draftManagement'
import { validDraftID } from './privateDraft'

export type ReferenceAction = 'create' | 'read' | 'cancel'
export interface ReferenceReceipt {
  id: string
  request_key: string
  draft_id: string
  state: 'queued' | 'cancelled' | 'reported'
  created_at: string
  cancelled_at: string | null
}
const endpoint = '/api/v1/problem-draft-management/reference-requests'
const codes: Record<string, number> = {
  NOT_FOUND: 404, REFERENCE_NOT_FOUND: 404, REFERENCE_SUBJECT_CHANGED: 409, REFERENCE_KEY_CONFLICT: 409,
  REFERENCE_SOURCE_CHANGED: 409, REFERENCE_QUEUE_FULL: 429, SESSION_REQUIRED: 401, PROBLEM_AUTHOR_REQUIRED: 403,
}
export class ReferenceRequestError extends Error {
  constructor(public code = 'UNCONFIRMED') { super('参考验证申请未确认。') }
}
const timestamp = (v: unknown) => typeof v === 'string' && /^\d{4}-\d\d-\d\dT/.test(v) && Number.isFinite(Date.parse(v))

export function decodeReferenceReceipt(value: unknown, draft: string, key: string): ReferenceReceipt {
  if (!object(value) || Object.keys(value).length !== 6 || typeof value.id !== 'string' || !validDraftID(value.id) ||
    value.draft_id !== draft || value.request_key !== key || !timestamp(value.created_at) ||
    (value.state !== 'queued' && value.state !== 'cancelled' && value.state !== 'reported') ||
    !(value.state === 'cancelled' ? timestamp(value.cancelled_at) : value.cancelled_at === null)) throw new ReferenceRequestError()
  return value as unknown as ReferenceReceipt
}

export async function sendReferenceRequest(action: ReferenceAction, owner: string, draft: string, key: string, signal: AbortSignal) {
  if (!validDraftID(owner) || !validDraftID(draft) || !validKey(key) || globalThis.location.protocol !== 'https:') throw new ReferenceRequestError()
  const path = action === 'create' ? endpoint : `${endpoint}/${key}${action === 'cancel' ? '/cancel' : ''}`
  const response = await fetch(path, {
    method: action === 'read' ? 'GET' : 'POST', credentials: 'same-origin', mode: 'same-origin', cache: 'no-store', redirect: 'error',
    referrerPolicy: 'no-referrer', signal: AbortSignal.any([signal, AbortSignal.timeout(15_000)]),
    headers: { Accept: 'application/json', 'X-OJ-Expected-Subject': owner, ...(action === 'create' ? { 'Content-Type': 'application/json' } : {}) },
    ...(action === 'create' ? { body: JSON.stringify({ request_key: key, draft_id: draft }) } : {}),
  })
  const reader = response.body?.getReader()
  if (!reader) throw new ReferenceRequestError()
  const bytes = new Uint8Array(2048)
  let offset = 0
  try {
    const media = response.status === 200 ? 'application/json' : 'application/problem+json'
    if (response.headers.get('Content-Type')?.split(';')[0].trim() !== media) throw new ReferenceRequestError()
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      if (offset + value.length > bytes.length) throw new ReferenceRequestError()
      bytes.set(value, offset)
      offset += value.length
    }
  } finally { await reader.cancel().catch(() => undefined); reader.releaseLock() }
  const data: unknown = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes.subarray(0, offset)))
  if (response.status !== 200) {
    throw new ReferenceRequestError(object(data) && typeof data.code === 'string' && codes[data.code] === response.status ? data.code : 'UNCONFIRMED')
  }
  return decodeReferenceReceipt(data, draft, key)
}
