import { readBoundedJSON } from './privateDraft'
import { decodeSummary } from './trainingHistory'

export interface TrainingAttempt {
  subject: string
  key: string
  problemID: string
  version: string
  source: string
}

export class TrainingIntakeError extends Error {
  constructor(readonly status: number) { super('暂时无法确认提交') }
}

export async function requestTraining(attempt: TrainingAttempt, recover: boolean, signal: AbortSignal) {
  const response = await fetch(recover ? '/api/v1/training/receipt' : '/api/v1/training/submissions', {
    method: recover ? 'GET' : 'POST',
    headers: {
      Accept: 'application/json',
      'X-OJ-Expected-Subject': attempt.subject,
      'Idempotency-Key': attempt.key,
      ...(!recover && { 'Content-Type': 'application/json' }),
    },
    body: recover ? undefined : JSON.stringify({
      problem_id: attempt.problemID, publication_version: attempt.version, source: attempt.source,
    }),
    credentials: 'same-origin', mode: 'same-origin', cache: 'no-store', redirect: 'error', referrerPolicy: 'no-referrer',
    signal: AbortSignal.any([signal, AbortSignal.timeout(15_000)]),
  })
  if (response.status !== 200) {
    await response.body?.cancel().catch(() => undefined)
    throw new TrainingIntakeError(response.status)
  }
  return decodeSummary(await readBoundedJSON(response, 4096))
}
