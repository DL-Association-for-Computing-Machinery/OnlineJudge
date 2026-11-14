import { object, validKey, validRevision } from './draftManagement'
import { validDraftID } from './privateDraft'
import { readReviewJSON, type ReviewMaterials } from './reviewMaterials'
import { validReturnReason } from './reviewReturnReason'

type Evidence = ReviewMaterials['evidence']
export interface ReviewReturnKey { key: string; assignment: number }

export async function requestReviewReturn(owner: string, evidence: Evidence, original: ReviewReturnKey, signal: AbortSignal, reason?: string) {
  if (!validDraftID(evidence.review_id) || !validKey(original.key) || !Number.isSafeInteger(original.assignment) || original.assignment < 1 ||
    !validRevision(evidence.import_revision) || !validRevision(evidence.evidence_sha256) ||
    (reason !== undefined && (original.assignment !== evidence.assignment_revision || !validReturnReason(reason)))) {
    throw new Error('Invalid return request binding')
  }
  const binding = {
    request_key: original.key, assignment_revision: original.assignment,
    import_revision: evidence.import_revision, evidence_sha256: evidence.evidence_sha256,
  }
  const path = `/api/v1/problem-review/${evidence.review_id}/return`
  const receipt = await readReviewJSON(reason === undefined ? `${path}/${original.key}` : path, owner, 4096, signal,
    reason === undefined ? undefined : { ...binding, reason })
  if (!object(receipt) || Object.keys(receipt).length !== 7 || receipt.review_id !== evidence.review_id ||
    Object.entries(binding).some(([field, value]) => receipt[field] !== value) ||
    !validReturnReason(receipt.reason) || (reason !== undefined && receipt.reason !== reason) ||
    typeof receipt.returned_at !== 'string' || !/^\d{4}-\d\d-\d\dT/.test(receipt.returned_at) ||
    !Number.isFinite(Date.parse(receipt.returned_at))) throw new Error('Review return receipt does not match')
}
