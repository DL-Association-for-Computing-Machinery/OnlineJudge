import { readBoundedJSON } from './privateDraft'
import { decodeSummary } from './trainingHistory'

export interface TrainingReceiptKey {
  subject: string
  key: string
}

export interface TrainingAttempt extends TrainingReceiptKey {
  problemID: string
  version: string
  source: string
}

export class TrainingIntakeError extends Error {
  constructor(readonly status: number) { super('暂时无法确认提交') }
}

export function submitTraining(attempt: TrainingAttempt, signal: AbortSignal) {
  return requestTraining(attempt, signal, JSON.stringify({
    problem_id: attempt.problemID, publication_version: attempt.version, source: attempt.source,
  }))
}

export function recoverTraining(key: TrainingReceiptKey, signal: AbortSignal) {
  return requestTraining(key, signal)
}

async function requestTraining(attempt: TrainingReceiptKey, signal: AbortSignal, body?: string) {
  const response = await fetch(body === undefined ? '/api/v1/training/receipt' : '/api/v1/training/submissions', {
    method: body === undefined ? 'GET' : 'POST',
    headers: {
      Accept: 'application/json',
      'X-OJ-Expected-Subject': attempt.subject,
      'Idempotency-Key': attempt.key,
      ...(body !== undefined && { 'Content-Type': 'application/json' }),
    },
    body,
    credentials: 'same-origin', mode: 'same-origin', cache: 'no-store', redirect: 'error', referrerPolicy: 'no-referrer',
    signal: AbortSignal.any([signal, AbortSignal.timeout(15_000)]),
  })
  if (response.status !== 200) {
    await response.body?.cancel().catch(() => undefined)
    throw new TrainingIntakeError(response.status)
  }
  return decodeSummary(await readBoundedJSON(response, 4096))
}
