const path = '/api/v1/training/submissions'
const idPattern = /^[a-f0-9]{32}$/
const cursorPattern = /^[A-Za-z0-9_-]{80}$/
const timePattern = /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,6})?Z$/
const verdicts = ['AC', 'WA', 'CE', 'RE', 'TLE', 'MLE', 'OLE', 'SE']
const maxSourceBytes = 65_536
const encoder = new TextEncoder()

export const trainingLanguageLabel = { 'gnu++17': 'GNU++17', c17: 'C17', 'gnu++20': 'GNU++20', java21: 'Java 21', python3: 'Python 3.14' } as const
export type TrainingLanguage = keyof typeof trainingLanguageLabel

export function isTrainingLanguage(value: unknown): value is TrainingLanguage {
  return typeof value === 'string' && Object.hasOwn(trainingLanguageLabel, value)
}

export interface SubmissionSummary {
  id: string
  material_version: string
  language: TrainingLanguage
  created_at: string
  state: 'queued' | 'running' | 'finished' | 'failed'
  verdict: string | null
}

export interface HistoryPage {
  submissions: SubmissionSummary[]
  next_cursor: string
}

export interface SubmissionDetail extends SubmissionSummary {
  source: string
}

export class HistoryError extends Error {
  constructor(readonly status = 0) { super('无法读取训练提交。') }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export function decodeHistoryPage(value: unknown): HistoryPage {
  if (!isRecord(value) || Object.keys(value).length !== 2 || !Array.isArray(value.submissions) ||
    value.submissions.length > 20 || typeof value.next_cursor !== 'string' ||
    (value.next_cursor !== '' && (!cursorPattern.test(value.next_cursor) || value.submissions.length !== 20))) throw new HistoryError()
  const ids = new Set<string>()
  const submissions = value.submissions.map(value => {
    const row = decodeSummary(value)
    if (ids.has(row.id)) throw new HistoryError()
    ids.add(row.id)
    return row
  })
  return { submissions, next_cursor: value.next_cursor }
}

export function decodeSummary(row: unknown): SubmissionSummary {
  if (!isRecord(row) || Object.keys(row).length !== 6 || typeof row.id !== 'string' || !idPattern.test(row.id) ||
    typeof row.material_version !== 'string' || row.material_version === '' || encoder.encode(row.material_version).length > 128 ||
    !isTrainingLanguage(row.language) || typeof row.created_at !== 'string' || !timePattern.test(row.created_at) ||
    !Number.isFinite(Date.parse(row.created_at)) ||
    (row.state === 'finished' ? typeof row.verdict !== 'string' || !verdicts.includes(row.verdict) :
      (row.state !== 'queued' && row.state !== 'running' && row.state !== 'failed') || row.verdict !== null)) throw new HistoryError()
  return row as unknown as SubmissionSummary
}

export async function fetchTrainingHistory(subject: string, cursor: string, signal: AbortSignal): Promise<HistoryPage> {
  if (cursor && !cursorPattern.test(cursor)) throw new HistoryError()
  // The server cursor preserves PostgreSQL microseconds; JavaScript dates do not.
  const page = decodeHistoryPage(await readTrainingJSON(path + (cursor ? '?cursor=' + cursor : ''), subject, signal, 32_768))
  if (cursor && page.next_cursor === cursor) throw new HistoryError()
  return page
}

export async function fetchTrainingDetail(subject: string, id: string, signal: AbortSignal): Promise<SubmissionDetail> {
  if (!idPattern.test(id)) throw new HistoryError()
  // Control characters may each take six bytes in JSON, plus the receipt fields.
  const value = await readTrainingJSON(path + '/' + id, subject, signal, maxSourceBytes * 6 + 4096)
  if (!isRecord(value)) throw new HistoryError()
  const { source, ...summary } = value
  const row = decodeSummary(summary)
  if (row.id !== id || typeof source !== 'string' || !source || source.includes('\0') ||
    encoder.encode(source).length > maxSourceBytes) throw new HistoryError()
  return { ...row, source }
}

export async function fetchTrainingStatus(subject: string, id: string, signal: AbortSignal): Promise<SubmissionSummary> {
  if (!idPattern.test(id)) throw new HistoryError()
  const row = decodeSummary(await readTrainingJSON(path + '/' + id + '?view=status', subject, signal, 4096))
  if (row.id !== id) throw new HistoryError()
  return row
}

async function readTrainingJSON(url: string, subject: string, signal: AbortSignal, limit: number): Promise<unknown> {
  if (location.protocol !== 'https:' || !idPattern.test(subject)) throw new HistoryError()
  const response = await fetch(url, {
    credentials: 'same-origin', mode: 'same-origin', cache: 'no-store', redirect: 'error', referrerPolicy: 'no-referrer',
    headers: { Accept: 'application/json', 'X-OJ-Expected-Subject': subject },
    signal: AbortSignal.any([signal, AbortSignal.timeout(15_000)]),
  })
  if (response.status !== 200 || response.headers.get('Content-Type')?.split(';')[0].trim() !== 'application/json' || !response.body) {
    await response.body?.cancel().catch(() => undefined)
    throw new HistoryError(response.status)
  }
  const reader = response.body.getReader()
  let size = 0
  const decoder = new TextDecoder('utf-8', { fatal: true })
  let text = ''
  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      size += value.byteLength
      if (size > limit) throw new HistoryError()
      text += decoder.decode(value, { stream: true })
    }
    return JSON.parse(text + decoder.decode()) as unknown
  } finally {
    await reader.cancel().catch(() => undefined)
    reader.releaseLock()
  }
}
