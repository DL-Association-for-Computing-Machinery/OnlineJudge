export interface ProcessHealth {
  status: 'ok'
  service: 'api'
  scope: 'process'
}

// This checks only the API process; it must not imply that judging is available.
export async function getProcessHealth(): Promise<ProcessHealth> {
  const response = await fetch('/api/v1/healthz', {
    signal: AbortSignal.timeout(5000),
    cache: 'no-store',
    credentials: 'same-origin',
  })
  if (!response.ok) throw new Error('API 暂时不可用，请检查服务后重试。')
  const data: unknown = await response.json()
  if (typeof data !== 'object' || data === null ||
      !('status' in data) || data.status !== 'ok' ||
      !('service' in data) || data.service !== 'api' ||
      !('scope' in data) || data.scope !== 'process') {
    throw new Error('API 返回了不符合健康检查契约的响应。')
  }
  return { status: 'ok', service: 'api', scope: 'process' }
}
