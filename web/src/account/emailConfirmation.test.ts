import { expect, it, vi } from 'vitest'
import { captureEmailConfirmation } from './emailConfirmation'

const token = 'A'.repeat(43)
const location = { pathname: '/account/activate', protocol: 'https:', search: '', hash: '#token=' + token }
const problem = (code: string, extra = {}) => Response.json({ status: 400, code, ...extra }, {
  status: 400, headers: { 'Content-Type': 'application/problem+json' },
})

it('removes the fragment before any write, then posts only on explicit confirmation', async () => {
  const send = vi.fn<typeof fetch>().mockResolvedValueOnce(new Response(null, { status: 204 }))
  const replace = vi.fn()
  const page = captureEmailConfirmation(location, replace, send)
  expect(replace).toHaveBeenCalledExactlyOnceWith('/account/activate')
  expect(send).not.toHaveBeenCalled()
  expect(await page?.confirm?.('  synthetic password  ')).toBe('done')
  expect(send).toHaveBeenCalledExactlyOnceWith('/api/v1/account/activate', expect.objectContaining({
    method: 'POST', redirect: 'error', credentials: 'omit', mode: 'same-origin', referrerPolicy: 'origin', cache: 'no-store',
    body: JSON.stringify({ token, new_password: '  synthetic password  ' }),
  }))
  expect(await page?.confirm?.('another synthetic password')).toBe('unknown')
  expect(send).toHaveBeenCalledOnce()
})

it('allows no write from a URL query, wrong origin scheme, malformed fragment or unsanitized history', () => {
  const replace = vi.fn()
  const send = vi.fn<typeof fetch>()
  for (const input of [
    { search: '?token=fixture' }, { protocol: 'http:' }, { hash: location.hash + '&token=fixture' },
    { hash: location.hash.slice(0, -1) + 'B' }, { hash: '' },
  ]) expect(captureEmailConfirmation({ ...location, ...input }, replace, send)?.confirm).toBeUndefined()
  expect(captureEmailConfirmation(location, () => { throw new Error('fixture') }, send)?.confirm).toBeUndefined()
  expect(captureEmailConfirmation({ ...location, pathname: '/account' }, replace, send)).toBeUndefined()
  expect(send).not.toHaveBeenCalled()
})

it('keeps the recovery link only across a known refusal and suppresses concurrent or unknown repeats', async () => {
  let release!: (response: Response) => void
  const send = vi.fn<typeof fetch>()
    .mockImplementationOnce(() => new Promise(resolve => { release = resolve }))
    .mockRejectedValueOnce(new Error('fixture'))
  const page = captureEmailConfirmation({ ...location, pathname: '/account/reset-password' }, vi.fn(), send)
  expect(await page?.confirm?.('密'.repeat(1400))).toBe('invalid-password')
  expect(send).not.toHaveBeenCalled()
  const first = page?.confirm?.('synthetic password')
  expect(await page?.confirm?.('repeat')).toBe('unknown')
  release(problem('WEAK_PASSWORD'))
  expect(await first).toBe('weak-password')
  expect(await page?.confirm?.('longer synthetic password')).toBe('unknown')
  expect(await page?.confirm?.('retry')).toBe('unknown')
  expect(send.mock.calls.map(([path]) => path)).toEqual(['/api/v1/account/reset-password', '/api/v1/account/reset-password'])
})

it('treats an oversized problem response as unknown even when it contains a recognized code', async () => {
  const send = vi.fn<typeof fetch>().mockResolvedValueOnce(problem('WEAK_PASSWORD', { title: 'x'.repeat(1024) }))
  const page = captureEmailConfirmation(location, vi.fn(), send)
  expect(await page?.confirm?.('synthetic password')).toBe('unknown')
  expect(await page?.confirm?.('retry')).toBe('unknown')
  expect(send).toHaveBeenCalledOnce()
})
