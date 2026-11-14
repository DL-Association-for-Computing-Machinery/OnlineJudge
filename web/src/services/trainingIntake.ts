import { readBoundedJSON, sameOriginJSONOptions } from '../transport/json'
import { decodeSummary, type TrainingLanguage } from './trainingHistory'

export interface TrainingReceiptKey {
  subject: string
  key: string
}

export interface TrainingAttempt extends TrainingReceiptKey {
  problemID: string
  version: string
  language: TrainingLanguage
  source: string
}

export class TrainingIntakeError extends Error {
  constructor(readonly status: number) { super('暂时无法确认提交') }
}

export function submitTraining(attempt: TrainingAttempt, signal: AbortSignal) {
  return requestTraining(attempt, signal, JSON.stringify({
    problem_id: attempt.problemID, publication_version: attempt.version, language: attempt.language, source: attempt.source,
  }))
}

export function recoverTraining(key: TrainingReceiptKey, signal: AbortSignal) {
  return requestTraining(key, signal)
}

async function requestTraining(attempt: TrainingReceiptKey, signal: AbortSignal, body?: string) {
  const options = sameOriginJSONOptions(signal, 'same-origin')
  const response = await fetch(body === undefined ? '/api/v1/training/receipt' : '/api/v1/training/submissions', {
    ...options,
    method: body === undefined ? 'GET' : 'POST',
    headers: {
      ...options.headers,
      'X-OJ-Expected-Subject': attempt.subject,
      'Idempotency-Key': attempt.key,
      ...(body !== undefined && { 'Content-Type': 'application/json' }),
    },
    body,
  })
  return decodeSummary(await readBoundedJSON(response, 4096, status => new TrainingIntakeError(status ?? 0)))
}
