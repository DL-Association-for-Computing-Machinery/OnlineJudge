import { afterEach, expect, it, vi } from 'vitest'
import { readCatalog } from './catalog'
import { fetchTrainingStatus } from './trainingHistory'
import { recoverTraining, submitTraining } from './trainingIntake'

const problem = 'a'.repeat(32)
const version = 'b'.repeat(64)
const identity = { subject: 'c'.repeat(32), key: 'original-request-key' }
const signal = new AbortController().signal
afterEach(() => vi.unstubAllGlobals())

it('keeps catalog reads anonymous and decodes the public statement contract', async () => {
  const statement = { problem_id: problem, version, title: 'Problem', language: 'en', markdown: '# 中文', examples: [] }
  const fetch = vi.fn().mockResolvedValueOnce(Response.json(statement, { headers: { 'X-OJ-Training-Intake': 'enabled' } }))
    .mockResolvedValueOnce(Response.json({ ...statement, draft_id: problem }))
    .mockResolvedValueOnce(new Response(null, { status: 404 }))
  vi.stubGlobal('fetch', fetch)
  await expect(readCatalog(problem, signal)).resolves.toMatchObject({ draft_id: problem, markdown: '# 中文', trainingIntake: true })
  expect(fetch).toHaveBeenNthCalledWith(1, '/api/v1/problems/' + problem,
    expect.objectContaining({ credentials: 'omit', cache: 'no-store' }))
  await expect(readCatalog(problem, signal)).rejects.toThrow('Invalid catalog response')
  await expect(readCatalog(problem, signal)).resolves.toBeNull()
})

it('does not retry a refused write and can read the receipt with the original key', async () => {
  const receipt = { id: 'd'.repeat(32), material_version: 'version-1', language: 'gnu++17',
    created_at: '2026-01-01T00:00:00Z', state: 'queued', verdict: null }
  const fetch = vi.fn().mockResolvedValueOnce(new Response(null, { status: 429 })).mockResolvedValueOnce(Response.json(receipt))
  vi.stubGlobal('fetch', fetch)
  await expect(submitTraining({ ...identity, problemID: problem, version, source: 'int main(){}' }, signal))
    .rejects.toMatchObject({ status: 429 })
  expect(fetch).toHaveBeenCalledOnce()
  const headers = { 'X-OJ-Expected-Subject': identity.subject, 'Idempotency-Key': identity.key }
  const matchingHeaders: unknown = expect.objectContaining(headers)
  expect(fetch).toHaveBeenNthCalledWith(1, '/api/v1/training/submissions',
    expect.objectContaining({ method: 'POST', headers: matchingHeaders }))
  await expect(recoverTraining(identity, signal)).resolves.toEqual(receipt)
  expect(fetch).toHaveBeenNthCalledWith(2, '/api/v1/training/receipt',
    expect.objectContaining({ method: 'GET', headers: matchingHeaders }))
})

it('preserves the status response byte limit and cancels an oversized body', async () => {
  const cancel = vi.fn()
  const body = new ReadableStream<Uint8Array>({ start(c) { c.enqueue(new Uint8Array(4097)) }, cancel })
  vi.stubGlobal('location', { protocol: 'https:' })
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(body, { headers: { 'Content-Type': 'application/json' } })))
  await expect(fetchTrainingStatus(identity.subject, problem, signal)).rejects.toMatchObject({ status: 0 })
  expect(cancel).toHaveBeenCalledOnce()
})
