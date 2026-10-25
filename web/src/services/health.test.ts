import { afterEach, describe, expect, it, vi } from 'vitest'
import { getProcessHealth } from './health'

afterEach(() => vi.unstubAllGlobals())

describe('API connectivity', () => {
  it('accepts only the process-liveness contract', async () => {
    const request = vi.fn().mockResolvedValue(Response.json({ status: 'ok', service: 'api', scope: 'process' }))
    vi.stubGlobal('fetch', request)
    await expect(getProcessHealth()).resolves.toEqual({ status: 'ok', service: 'api', scope: 'process' })
    expect(request).toHaveBeenCalledWith('/api/v1/healthz', expect.objectContaining({ cache: 'no-store' }))
  })

  it('does not report HTTP errors as healthy', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('', { status: 503 })))
    await expect(getProcessHealth()).rejects.toThrow('暂时不可用')
  })

  it.each([null, {}, { status: 'ok', service: 'worker', scope: 'process' }])('rejects an unrelated response: %j', async (body) => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json(body)))
    await expect(getProcessHealth()).rejects.toThrow('不符合健康检查契约')
  })

  it('propagates disconnection instead of inventing success', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('network unavailable')))
    await expect(getProcessHealth()).rejects.toThrow('network unavailable')
  })
})
