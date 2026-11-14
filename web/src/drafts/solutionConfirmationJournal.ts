import { object, validRevision, type AuthorSolutionBinding, type ReviewIntakeIntent } from '../services/draftManagement'

export function solutionJournalSlot(owner: string, intent: ReviewIntakeIntent) {
  return ['oj.author-solution-confirmation.v1', owner, intent.draft_id, intent.request_key, intent.import_revision].join(':')
}

export function readSolutionBinding(slot: string, intent: ReviewIntakeIntent): AuthorSolutionBinding | null {
  const raw = sessionStorage.getItem(slot)
  if (raw === null) return null
  const data: unknown = JSON.parse(raw)
  if (!object(data) || Object.keys(data).length !== 2 || data.import_revision !== intent.import_revision ||
    !validRevision(data.formal_plan_sha256)) throw new Error('Invalid original solution binding')
  return { import_revision: intent.import_revision, formal_plan_sha256: data.formal_plan_sha256 }
}

export function reserveSolutionConfirmation(slot: string, intent: ReviewIntakeIntent, binding: AuthorSolutionBinding) {
  // Do not replace the first observed plan, even if a later preview differs.
  if (readSolutionBinding(slot, intent)) return false
  sessionStorage.setItem(slot, JSON.stringify(binding))
  return true
}
