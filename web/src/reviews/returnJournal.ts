import { object, validKey } from '../services/draftManagement'
import { type ReviewMaterials } from '../services/reviewMaterials'
import { type ReviewReturnKey } from '../services/reviewReturn'

export function returnSlot(owner: string, evidence: ReviewMaterials['evidence']) {
  // Keep the original assignment in the record so the same reviewer can read
  // it after reassignment without treating the new assignment as a new write.
  return ['oj.review-return.v1', owner, evidence.review_id, evidence.import_revision, evidence.evidence_sha256].join(':')
}

export function originalReturn(slot: string): ReviewReturnKey | null {
  const raw = sessionStorage.getItem(slot)
  if (raw === null) return null
  const value: unknown = JSON.parse(raw)
  if (!object(value) || Object.keys(value).length !== 2 || !validKey(value.key) ||
    typeof value.assignment !== 'number' || !Number.isSafeInteger(value.assignment) || value.assignment < 1) {
    throw new Error('Invalid original return key')
  }
  return { key: value.key, assignment: value.assignment }
}

export function reserveReturn(slot: string, assignment: number): ReviewReturnKey {
  const original = { key: crypto.randomUUID().replaceAll('-', ''), assignment }
  // Once the browser may have sent the write, only the original GET is safe.
  sessionStorage.setItem(slot, JSON.stringify(original))
  return original
}
