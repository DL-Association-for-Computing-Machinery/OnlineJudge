import { object } from './draftManagement'
import { decodeDraftPreview, validDraftID, type DraftPreview } from './privateDraft'

const digest = (value: unknown): value is string => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value)
const id = (value: unknown): value is string => typeof value === 'string' && validDraftID(value)
const invalid = () => new Error('无法读取当前分配的审核材料。')
const encoder = new TextEncoder()
const decoder = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true })
const bodyLimit = 7 * 1024 * 1024
const sourceLimit = 64 * 1024

interface ReviewMaterials {
  evidence: { review_id: string; draft_id: string; content_version: string; assignment_revision: number; import_revision: string; evidence_sha256: string; formal_verdict: string }
  statement: DraftPreview
  standard_solution: { source: string; dependencies?: { name: string; source: string }[] }
}

function decodeEvidence(value: unknown, review: string): ReviewMaterials['evidence'] {
  if (!object(value)) throw invalid()
  const hashes = ['content_version', 'import_revision', 'source_lock_sha256', 'formal_plan_sha256', 'profile_sha256', 'evidence_sha256']
  const fields = ['review_id', 'draft_id', 'source_id', 'reference_request_id', 'assignment_revision', 'formal_verdict', ...hashes]
  if (Object.keys(value).some(key => !fields.includes(key) && key !== 'comparison_plan_sha256' && key !== 'comparison_outcome') ||
    value.review_id !== review || !id(value.review_id) || !id(value.draft_id) || !id(value.reference_request_id) ||
    typeof value.source_id !== 'string' || !value.source_id || encoder.encode(value.source_id).byteLength > 128 ||
    typeof value.assignment_revision !== 'number' || !Number.isSafeInteger(value.assignment_revision) || value.assignment_revision < 1 ||
    typeof value.formal_verdict !== 'string' || !['AC', 'WA', 'CE', 'RE', 'TLE', 'MLE', 'OLE'].includes(value.formal_verdict) ||
    !hashes.every(key => digest(value[key])) ||
    ('comparison_plan_sha256' in value && !digest(value.comparison_plan_sha256)) ||
    ('comparison_outcome' in value && (!digest(value.comparison_plan_sha256) || (value.comparison_outcome !== 'passed' && value.comparison_outcome !== 'mismatch')))) throw invalid()
  return value as unknown as ReviewMaterials['evidence']
}

function sourceBytes(value: unknown) {
  if (typeof value !== 'string' || !value) throw invalid()
  const raw = encoder.encode(value)
  if (raw.byteLength > sourceLimit || decoder.decode(raw) !== value) throw invalid()
  return raw.byteLength
}

export function decodeReviewMaterials(value: unknown, review: string): ReviewMaterials {
  if (!object(value) || Object.keys(value).length !== 3) throw invalid()
  const evidence = decodeEvidence(value.evidence, review)
  const statement = decodeDraftPreview(value.statement, evidence.draft_id)
  if (statement.version !== evidence.content_version) throw invalid()
  const solution = value.standard_solution
  if (!object(solution) || Object.keys(solution).length !== ('dependencies' in solution ? 2 : 1)) throw invalid()
  let bytes = sourceBytes(solution.source)
  if ('dependencies' in solution) {
    if (!Array.isArray(solution.dependencies) || solution.dependencies.length < 1 || solution.dependencies.length > 4) throw invalid()
    const allowed = ['lib/prime.hpp', 'lib/montgomery.hpp', 'lib/util.hpp', 'common/random.h']
    const seen = new Set<string>()
    for (const dep of solution.dependencies) {
      if (!object(dep) || Object.keys(dep).length !== 2 || typeof dep.name !== 'string' || !allowed.includes(dep.name) || seen.has(dep.name)) throw invalid()
      seen.add(dep.name)
      bytes += sourceBytes(dep.source)
      if (bytes > sourceLimit) throw invalid()
    }
  }
  return { evidence, statement, standard_solution: solution as unknown as ReviewMaterials['standard_solution'] }
}

export async function fetchReviewMaterials(review: string, owner: string, signal: AbortSignal): Promise<ReviewMaterials> {
  if (!id(review) || !id(owner) || globalThis.location.protocol !== 'https:') throw invalid()
  const response = await fetch(`/api/v1/problem-review/${review}/materials`, {
    method: 'GET', credentials: 'same-origin', mode: 'same-origin', cache: 'no-store', redirect: 'error', referrerPolicy: 'no-referrer',
    headers: { Accept: 'application/json', 'X-OJ-Expected-Subject': owner }, signal: AbortSignal.any([signal, AbortSignal.timeout(15_000)]),
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
      if (size > bodyLimit) throw invalid()
      chunks.push(value)
    }
  } finally { await reader.cancel().catch(() => undefined); reader.releaseLock() }
  const body = new Uint8Array(size)
  let offset = 0
  for (const chunk of chunks) { body.set(chunk, offset); offset += chunk.byteLength }
  return decodeReviewMaterials(JSON.parse(decoder.decode(body)) as unknown, review)
}

export type { ReviewMaterials }
