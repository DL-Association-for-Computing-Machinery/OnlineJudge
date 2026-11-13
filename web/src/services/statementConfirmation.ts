import { object, validKey, validRevision } from './draftManagement'
import { validDraftID } from './privateDraft'
import type { ReviewMaterials } from './reviewMaterials'

const checklistVersion = 'oj.statement-semantics.v1'
type Evidence = ReviewMaterials['evidence']

export async function requestStatementConfirmation(
  action: 'confirm' | 'read', owner: string, evidence: Evidence, key: string, signal: AbortSignal,
) {
  if (!validDraftID(owner) || !validDraftID(evidence.review_id) || !validKey(key) ||
    !validRevision(evidence.import_revision) || !validRevision(evidence.evidence_sha256) ||
    !Number.isSafeInteger(evidence.assignment_revision) || evidence.assignment_revision < 1 ||
    location.protocol !== 'https:') throw new Error('Invalid statement confirmation binding')
  const intent = {
    request_key: key, assignment_revision: evidence.assignment_revision, import_revision: evidence.import_revision,
    evidence_sha256: evidence.evidence_sha256, checklist_version: checklistVersion,
  }
  const write = action === 'confirm'
  const path = `/api/v1/problem-review/${evidence.review_id}/statement-confirmation`
  const response = await fetch(write ? path : `${path}/${key}`, {
    method: write ? 'POST' : 'GET', credentials: 'same-origin', mode: 'same-origin', cache: 'no-store',
    redirect: 'error', referrerPolicy: write ? 'origin' : 'no-referrer',
    headers: { Accept: 'application/json', 'X-OJ-Expected-Subject': owner, ...(write ? { 'Content-Type': 'application/json' } : {}) },
    ...(write ? { body: JSON.stringify(intent) } : {}), signal: AbortSignal.any([signal, AbortSignal.timeout(15_000)]),
  })
  if (response.status !== 200 || response.headers.get('Content-Type')?.split(';')[0].trim() !== 'application/json' || !response.body) {
    await response.body?.cancel().catch(() => undefined)
    throw new Error('Statement confirmation receipt unavailable')
  }
  const reader = response.body.getReader()
  const bytes = new Uint8Array(2048)
  let length = 0
  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      if (length + value.length > bytes.length) throw new Error('Statement confirmation receipt exceeds limit')
      bytes.set(value, length)
      length += value.length
    }
  } finally { await reader.cancel().catch(() => undefined); reader.releaseLock() }
  const receipt: unknown = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes.subarray(0, length)))
  if (!object(receipt) || Object.keys(receipt).length !== 7 || receipt.review_id !== evidence.review_id ||
    Object.entries(intent).some(([field, value]) => receipt[field] !== value) ||
    typeof receipt.confirmed_at !== 'string' || !/^\d{4}-\d\d-\d\dT/.test(receipt.confirmed_at) ||
    !Number.isFinite(Date.parse(receipt.confirmed_at))) throw new Error('Statement confirmation receipt does not match')
}
