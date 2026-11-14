import { object, validKey, validRevision } from './draftManagement'
import { validDraftID } from './privateDraft'
import { readReviewJSON } from './reviewMaterials'
import { validReturnReason } from './reviewReturnReason'

export interface ApprovalKey { key: string; snapshot: string }

export function decodeApprovalKey(value: unknown): ApprovalKey {
  if (!object(value) || Object.keys(value).length !== 2 || !validKey(value.key) || !validRevision(value.snapshot)) {
    throw new Error('Invalid original approval binding')
  }
  return { key: value.key, snapshot: value.snapshot }
}

export async function requestReviewApproval(owner: string, id: string, assignment: number, original: ApprovalKey, signal: AbortSignal, reason?: string) {
  if (!validDraftID(id) || !Number.isSafeInteger(assignment) || assignment < 1 ||
    (reason !== undefined && !validReturnReason(reason))) throw new Error('Invalid approval request')
  const { key, snapshot } = decodeApprovalKey(original)
  const binding = { request_key: key, snapshot_sha256: snapshot, assignment_revision: assignment }
  const path = `/api/v1/problem-review/${id}/approve`
  const receipt = await readReviewJSON(reason === undefined ? `${path}/${key}` : path, owner, 4096, signal,
    reason === undefined ? undefined : { ...binding, reason })
  if (!object(receipt) || Object.keys(receipt).length !== 5 || receipt.review_id !== id ||
    Object.entries(binding).some(([field, value]) => receipt[field] !== value) ||
    typeof receipt.approved_at !== 'string' || !/^\d{4}-\d\d-\d\dT/.test(receipt.approved_at) ||
    !Number.isFinite(Date.parse(receipt.approved_at))) throw new Error('Approval receipt does not match')
}
