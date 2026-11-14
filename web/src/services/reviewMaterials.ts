import { object } from './draftManagement'
import { decodeFrozenSolution, type FrozenSolution } from './frozenSolution'
import { decodeDraftPreview, validDraftID, type DraftPreview } from './privateDraft'

const digest = (value: unknown): value is string => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value)
const id = (value: unknown): value is string => typeof value === 'string' && validDraftID(value)
const invalid = () => new Error('无法读取当前分配的审核材料。')
const encoder = new TextEncoder()
const decoder = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true })
const bodyLimit = 7 * 1024 * 1024
const evidenceHashes = ['content_version', 'import_revision', 'source_lock_sha256', 'formal_plan_sha256', 'profile_sha256', 'evidence_sha256'] as const
const evidenceFields = ['review_id', 'draft_id', 'source_id', 'reference_request_id', 'assignment_revision', 'formal_verdict',
  'comparison_plan_sha256', 'comparison_outcome', ...evidenceHashes] as const

type ReviewEvidence = Record<typeof evidenceHashes[number], string> & {
  review_id: string; draft_id: string; source_id: string; reference_request_id: string; assignment_revision: number; formal_verdict: string
  comparison_plan_sha256?: string; comparison_outcome?: 'passed' | 'mismatch'
}

interface ReviewMaterials {
  evidence: ReviewEvidence
  statement: DraftPreview
  standard_solution: FrozenSolution
}

export const reviewBlockerLabels = {
  materials_returned: '同一修订的材料已被退回',
  policy_unavailable: '来源或验证档位不符合当前策略',
  formal_failed: '正式参考验证未通过',
  comparison_missing: '缺少本站对拍结果',
  comparison_mismatch: '本站对拍结果不一致',
  independent_correctness_missing: '缺少独立正确性证据',
  statement_quality_missing: '题意确认或可信渲染证据未齐备',
  test_coverage_missing: '缺少正式数据覆盖证据',
  input_validator_missing: '缺少输入校验证据',
  checker_admission_missing: '缺少生产检查器准入证据',
  target_runtime_missing: '缺少目标运行环境验收证据',
  solution_confirmation_missing: '缺少作者标程确认',
} as const

export interface ReviewEligibility {
  blockers: (keyof typeof reviewBlockerLabels)[]
  statementConfirmed: boolean
}

function decodeEvidence(value: unknown, review: string): ReviewMaterials['evidence'] {
  if (!object(value)) throw invalid()
  if (Object.keys(value).some(key => !evidenceFields.some(field => field === key)) ||
    value.review_id !== review || !id(value.review_id) || !id(value.draft_id) || !id(value.reference_request_id) ||
    typeof value.source_id !== 'string' || !value.source_id || encoder.encode(value.source_id).byteLength > 128 ||
    typeof value.assignment_revision !== 'number' || !Number.isSafeInteger(value.assignment_revision) || value.assignment_revision < 1 ||
    typeof value.formal_verdict !== 'string' || !['AC', 'WA', 'CE', 'RE', 'TLE', 'MLE', 'OLE'].includes(value.formal_verdict) ||
    !evidenceHashes.every(key => digest(value[key])) ||
    ('comparison_plan_sha256' in value && !digest(value.comparison_plan_sha256)) ||
    ('comparison_outcome' in value && (!digest(value.comparison_plan_sha256) || (value.comparison_outcome !== 'passed' && value.comparison_outcome !== 'mismatch')))) throw invalid()
  return value as unknown as ReviewMaterials['evidence']
}

export function decodeReviewMaterials(value: unknown, review: string): ReviewMaterials {
  if (!object(value) || Object.keys(value).length !== 3) throw invalid()
  const evidence = decodeEvidence(value.evidence, review)
  const statement = decodeDraftPreview(value.statement, evidence.draft_id)
  if (statement.version !== evidence.content_version) throw invalid()
  return { evidence, statement, standard_solution: decodeFrozenSolution(value.standard_solution) }
}

export async function fetchReviewMaterials(review: string, owner: string, signal: AbortSignal): Promise<ReviewMaterials> {
  if (!id(review)) throw invalid()
  return decodeReviewMaterials(await readReviewJSON(`/api/v1/problem-review/${review}/materials`, owner, bodyLimit, signal), review)
}

export async function fetchReviewEligibility(evidence: ReviewMaterials['evidence'], owner: string, signal: AbortSignal): Promise<ReviewEligibility> {
  if (!id(evidence.review_id)) throw invalid()
  const value = await readReviewJSON(`/api/v1/problem-review/${evidence.review_id}/eligibility`, owner, 32 * 1024, signal)
  if (!object(value) || value.policy_version !== 'oj.review-approval.v14' || value.eligible !== false ||
    ('statement_semantics_confirmed' in value && value.statement_semantics_confirmed !== true)) throw invalid()
  const actual = decodeEvidence(value.evidence, evidence.review_id)
  if (evidenceFields.some(field => actual[field] !== evidence[field])) throw invalid()
  const blockers: unknown = value.blockers
  if (!Array.isArray(blockers) || blockers.length === 0 || blockers.length > Object.keys(reviewBlockerLabels).length ||
    new Set(blockers).size !== blockers.length || !blockers.every((code): code is keyof typeof reviewBlockerLabels =>
      typeof code === 'string' && Object.hasOwn(reviewBlockerLabels, code))) throw invalid()
  return { blockers, statementConfirmed: value.statement_semantics_confirmed === true }
}

export interface ReviewAssignmentPage {
  assignments: { review_id: string; source_id: string; import_revision: string; assignment_revision: number }[]
  next_cursor: string
}

const reviewCursor = /^[A-Za-z0-9_-]{91}$/

export async function fetchReviewAssignments(owner: string, cursor: string, signal: AbortSignal): Promise<ReviewAssignmentPage> {
  if (cursor !== '' && !reviewCursor.test(cursor)) throw invalid()
  const path = '/api/v1/problem-review/assignments' + (cursor ? `?cursor=${cursor}` : '')
  const data = await readReviewJSON(path, owner, 20480, signal)
  if (!object(data) || Object.keys(data).length !== 2 || typeof data.next_cursor !== 'string' ||
    (data.next_cursor !== '' && !reviewCursor.test(data.next_cursor)) ||
    !Array.isArray(data.assignments) || data.assignments.length > 20) throw invalid()
  let last = ''
  for (const item of data.assignments) {
    if (!object(item) || Object.keys(item).length !== 4 || !id(item.review_id) || item.review_id <= last ||
      typeof item.source_id !== 'string' || !item.source_id || encoder.encode(item.source_id).byteLength > 128 ||
      !digest(item.import_revision) || typeof item.assignment_revision !== 'number' ||
      !Number.isSafeInteger(item.assignment_revision) || item.assignment_revision < 1) throw invalid()
    last = item.review_id
  }
  return data as unknown as ReviewAssignmentPage
}

export async function readReviewJSON(path: string, owner: string, limit: number, signal: AbortSignal, intent?: object): Promise<unknown> {
  if (!id(owner) || globalThis.location.protocol !== 'https:') throw invalid()
  const response = await fetch(path, {
    method: intent ? 'POST' : 'GET', credentials: 'same-origin', mode: 'same-origin', cache: 'no-store', redirect: 'error',
    referrerPolicy: intent ? 'origin' : 'no-referrer',
    headers: { Accept: 'application/json', 'X-OJ-Expected-Subject': owner, ...(intent ? { 'Content-Type': 'application/json' } : {}) },
    ...(intent ? { body: JSON.stringify(intent) } : {}), signal: AbortSignal.any([signal, AbortSignal.timeout(15_000)]),
  })
  if (response.status !== 200 || response.headers.get('Content-Type')?.split(';')[0].trim() !== 'application/json' || !response.body) {
    await response.body?.cancel().catch(() => undefined)
    throw invalid()
  }
  const reader = response.body.getReader()
  const chunks: Uint8Array[] = []
  let size = 0
  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      size += value.byteLength
      if (size > limit) throw invalid()
      chunks.push(value)
    }
  } finally { await reader.cancel().catch(() => undefined); reader.releaseLock() }
  const body = new Uint8Array(size)
  let offset = 0
  for (const chunk of chunks) { body.set(chunk, offset); offset += chunk.byteLength }
  return JSON.parse(decoder.decode(body)) as unknown
}

export type { ReviewMaterials }
