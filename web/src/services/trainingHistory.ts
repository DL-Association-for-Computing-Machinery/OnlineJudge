const path = '/api/v1/training/submissions'
const idPattern = /^[a-f0-9]{32}$/
const cursorPattern = /^[A-Za-z0-9_-]{80}$/
const timePattern = /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,6})?Z$/
const verdicts = ['AC', 'WA', 'CE', 'RE', 'TLE', 'MLE', 'OLE', 'SE']

export interface SubmissionSummary {
  id: string
  material_version: string
  language: 'gnu++17'
  created_at: string
  state: 'queued' | 'running' | 'finished' | 'failed'
  verdict: string | null
}

export interface HistoryPage {
  submissions: SubmissionSummary[]
  next_cursor: string
}

export class HistoryError extends Error {
  constructor(readonly status = 0) { super('无法读取训练提交历史。') }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export function decodeHistoryPage(value: unknown): HistoryPage {
  if (!isRecord(value) || Object.keys(value).length !== 2 || !Array.isArray(value.submissions) ||
    value.submissions.length > 20 || typeof value.next_cursor !== 'string' ||
    (value.next_cursor !== '' && (!cursorPattern.test(value.next_cursor) || value.submissions.length !== 20))) throw new HistoryError()
  const ids = new Set<string>()
  const submissions = value.submissions.map((row: unknown) => {
    if (!isRecord(row) || Object.keys(row).length !== 6 || typeof row.id !== 'string' || !idPattern.test(row.id) || ids.has(row.id) ||
      typeof row.material_version !== 'string' || row.material_version === '' || new TextEncoder().encode(row.material_version).length > 128 ||
      row.language !== 'gnu++17' || typeof row.created_at !== 'string' || !timePattern.test(row.created_at) ||
      !Number.isFinite(Date.parse(row.created_at)) ||
      (row.state === 'finished' ? typeof row.verdict !== 'string' || !verdicts.includes(row.verdict) :
        (row.state !== 'queued' && row.state !== 'running' && row.state !== 'failed') || row.verdict !== null)) throw new HistoryError()
    ids.add(row.id)
    return row as unknown as SubmissionSummary
  })
  return { submissions, next_cursor: value.next_cursor }
}

export async function fetchTrainingHistory(subject: string, cursor: string, signal: AbortSignal): Promise<HistoryPage> {
  if (location.protocol !== 'https:' || !idPattern.test(subject) || (cursor && !cursorPattern.test(cursor))) throw new HistoryError()
  // The server cursor preserves PostgreSQL microseconds; JavaScript dates do not.
  const response = await fetch(path + (cursor ? '?cursor=' + cursor : ''), {
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
      if (size > 32_768) throw new HistoryError()
      text += decoder.decode(value, { stream: true })
    }
    const page = decodeHistoryPage(JSON.parse(text + decoder.decode()) as unknown)
    if (cursor && page.next_cursor === cursor) throw new HistoryError()
    return page
  } finally {
    await reader.cancel().catch(() => undefined)
    reader.releaseLock()
  }
}
