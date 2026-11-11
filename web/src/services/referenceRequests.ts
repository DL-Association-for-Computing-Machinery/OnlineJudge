import { object, validKey } from './draftManagement'
import { validDraftID } from './privateDraft'

export type ReferenceAction = 'create' | 'read' | 'cancel'
export type ReferencePreparationAction = 'prepare' | 'plan'
export interface ReferenceReceipt {
  id: string
  request_key: string
  draft_id: string
  state: 'queued' | 'cancelled' | 'reported'
  created_at: string
  cancelled_at: string | null
}
export interface ReferenceReport {
  request_id: string
  request_key: string
  draft_id: string
  content_version: string
  formal: { verdict: 'AC' | 'CE' | 'WA' | 'RE' | 'TLE' | 'MLE' | 'OLE'; passed: number; total: number }
  comparison: {
    status: 'passed' | 'mismatch'
    matched: number
    total: number
    counterexample: { case: number; input: string; expected: number[]; actual: number[] } | null
  } | null
}
const endpoint = '/api/v1/problem-draft-management/reference-requests'
const codes: Record<string, number> = {
  NOT_FOUND: 404, REFERENCE_NOT_FOUND: 404, REFERENCE_SUBJECT_CHANGED: 409, REFERENCE_KEY_CONFLICT: 409,
  REFERENCE_SOURCE_CHANGED: 409, REFERENCE_QUEUE_FULL: 429, SESSION_REQUIRED: 401, PROBLEM_AUTHOR_REQUIRED: 403,
  REFERENCE_PREPARATION_BUSY: 429, REFERENCE_PREPARATION_DENIED: 409,
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
  if (!validDraftID(draft) || !validKey(key)) throw new ReferenceRequestError()
  const path = action === 'create' ? endpoint : `${endpoint}/${key}${action === 'cancel' ? '/cancel' : ''}`
  const body = action === 'create' ? { request_key: key, draft_id: draft } : undefined
  return decodeReferenceReceipt(await requestReference(path, owner, action === 'read' ? 'GET' : 'POST', signal, body), draft, key)
}

export async function sendReferencePreparation(action: ReferencePreparationAction, owner: string, key: string, signal: AbortSignal) {
  if (!validKey(key)) throw new ReferenceRequestError()
  const data = await requestReference(`${endpoint}/${key}/${action}`, owner, action === 'plan' ? 'GET' : 'POST', signal, undefined, action === 'prepare')
  if (!object(data) || Object.keys(data).length !== 1 || typeof data.ready !== 'boolean') throw new ReferenceRequestError()
  return data.ready
}

const integer = (value: unknown, min: number, max: number): value is number =>
  typeof value === 'number' && Number.isSafeInteger(value) && value >= min && value <= max
const coefficientVector = (value: unknown): value is number[] =>
  Array.isArray(value) && value.length >= 1 && value.length <= 31 && value.every(x => integer(x, 0, 998244352))

export function decodeReferenceReport(value: unknown, receipt: ReferenceReceipt): ReferenceReport {
  if (!object(value) || Object.keys(value).length !== 6 || value.request_id !== receipt.id ||
    value.request_key !== receipt.request_key || value.draft_id !== receipt.draft_id || receipt.state !== 'reported' ||
    typeof value.content_version !== 'string' || !/^[0-9a-f]{64}$/.test(value.content_version)) throw new ReferenceRequestError()
  const formal = value.formal
  if (!object(formal) || Object.keys(formal).length !== 3 || !integer(formal.total, 1, 64) || !integer(formal.passed, 0, formal.total) ||
    typeof formal.verdict !== 'string' || !['AC', 'CE', 'WA', 'RE', 'TLE', 'MLE', 'OLE'].includes(formal.verdict) ||
    (formal.verdict === 'AC') !== (formal.passed === formal.total) || (formal.verdict === 'CE' && formal.passed !== 0)) throw new ReferenceRequestError()
  const comparison = value.comparison
  if (comparison !== null) {
    if (formal.verdict !== 'AC' || !object(comparison) || Object.keys(comparison).length !== 4 ||
      comparison.total !== 24 || !integer(comparison.matched, 0, 24)) throw new ReferenceRequestError()
    const diff = comparison.counterexample
    if (comparison.status === 'passed') {
      if (comparison.matched !== 24 || diff !== null) throw new ReferenceRequestError()
    } else if (comparison.status !== 'mismatch' || comparison.matched === 24 || !object(diff) || Object.keys(diff).length !== 4 ||
      diff.case !== comparison.matched + 1 || typeof diff.input !== 'string' || diff.input.length < 1 || diff.input.length > 512 ||
      !/^[0-9 \n]+$/.test(diff.input) || !coefficientVector(diff.expected) || !coefficientVector(diff.actual) ||
      diff.expected.length !== diff.actual.length || diff.expected.every((x, i) => x === (diff.actual as number[])[i])) throw new ReferenceRequestError()
  }
  return value as unknown as ReferenceReport
}

export async function sendReferenceReport(owner: string, receipt: ReferenceReceipt, signal: AbortSignal) {
  if (receipt.state !== 'reported' || !validKey(receipt.request_key) || !validDraftID(receipt.draft_id) || !validDraftID(receipt.id)) throw new ReferenceRequestError()
  return decodeReferenceReport(await requestReference(`${endpoint}/${receipt.request_key}/report`, owner, 'GET', signal), receipt)
}

async function requestReference(path: string, owner: string, method: 'GET' | 'POST', signal: AbortSignal, body?: object, acceptsWork = false) {
  if (!validDraftID(owner) || globalThis.location.protocol !== 'https:') throw new ReferenceRequestError()
  const response = await fetch(path, {
    method, credentials: 'same-origin', mode: 'same-origin', cache: 'no-store', redirect: 'error',
    referrerPolicy: 'no-referrer', signal: AbortSignal.any([signal, AbortSignal.timeout(15_000)]),
    headers: { Accept: 'application/json', 'X-OJ-Expected-Subject': owner, ...(body ? { 'Content-Type': 'application/json' } : {}) },
    ...(body ? { body: JSON.stringify(body) } : {}),
  })
  const success = response.status === 200 || (acceptsWork && response.status === 202)
  const reader = response.body?.getReader()
  if (!reader) throw new ReferenceRequestError()
  const bytes = new Uint8Array(2048)
  let offset = 0
  try {
    const media = success ? 'application/json' : 'application/problem+json'
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
  if (!success) {
    throw new ReferenceRequestError(object(data) && typeof data.code === 'string' && codes[data.code] === response.status ? data.code : 'UNCONFIRMED')
  }
  if (response.status === 202 && (!object(data) || data.ready !== false)) throw new ReferenceRequestError()
  return data
}
