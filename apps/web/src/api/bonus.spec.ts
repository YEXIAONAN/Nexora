import { afterEach, describe, expect, it, vi } from 'vitest'
import * as api from './bonus'

const command = { expected_revision: 7, client_request_id: 'request-id', reason: '人工核对' }
const base = '/api/v1/teams/team%2FA/projects/project%20B/bonus-rounds/round%2FC'
function mockFetch() {
  const fetcher = vi
    .fn()
    .mockResolvedValue(
      new Response(JSON.stringify({ data: { round_id: 'round/C', revision: 8 } }), { status: 200 }),
    )
  vi.stubGlobal('fetch', fetcher)
  return fetcher
}
afterEach(() => vi.unstubAllGlobals())

describe('bonus command API', () => {
  it.each(['submit', 'return', 'confirm', 'publish', 'revise'] as const)(
    'posts %s through the CSRF client',
    async (action) => {
      const fetcher = mockFetch()
      const result = await api.transitionBonusRound(
        'team/A',
        'project B',
        'round/C',
        action,
        command,
      )
      expect(result.revision).toBe(8)
      const [path, options] = fetcher.mock.calls[0]!
      expect(path).toBe(`${base}/${action}`)
      expect(options.method).toBe('POST')
      expect(options.headers['X-Nexora-CSRF']).toBe('1')
      expect(options.credentials).toBe('same-origin')
      expect(JSON.parse(options.body)).toEqual(command)
    },
  )
  it('saves exact integer fen using PUT', async () => {
    const fetcher = mockFetch()
    const body = { ...command, total_budget_minor: 29, sources: [] }
    await api.saveBonusPlan('team/A', 'project B', 'round/C', body)
    const [path, options] = fetcher.mock.calls[0]!
    expect(path).toBe(`${base}/plan`)
    expect(options.method).toBe('PUT')
    expect(JSON.parse(options.body)).toEqual(body)
  })
  it('sends only the subject and reference for simulated payment, never an amount', async () => {
    const fetcher = mockFetch()
    await api.recordBonusPayment('team/A', 'project B', 'round/C', {
      ...command,
      user_id: 'member',
      reference: 'SIM-01',
    })
    const [path, options] = fetcher.mock.calls[0]!
    expect(path).toBe(`${base}/payments`)
    expect(JSON.parse(options.body)).toEqual({ ...command, user_id: 'member', reference: 'SIM-01' })
    expect(options.headers['X-Nexora-CSRF']).toBe('1')
  })
  it.each([false, true])('sends explicit grant revoke=%s', async (revoke) => {
    const fetcher = mockFetch()
    await api.changeBonusGrant('team/A', 'project B', 'round/C', {
      ...command,
      subject_id: 'member',
      role_code: 'budget',
      revoke,
    })
    expect(fetcher.mock.calls[0]![0]).toBe(`${base}/grants`)
    expect(JSON.parse(fetcher.mock.calls[0]![1].body)).toEqual({
      ...command,
      subject_id: 'member',
      role_code: 'budget',
      revoke,
    })
  })
  it('posts initial round creation without assigning a budget', async () => {
    const fetcher = mockFetch()
    const body = {
      funding_key: 'award-2026',
      funding_label: '专项奖金',
      funding_source: '独立拨款',
      manager_id: 'manager',
      reviewer_id: 'reviewer',
      reason: '新资金',
      client_request_id: 'new-id',
    }
    await api.createBonusRound('team/A', 'project B', body)
    expect(fetcher.mock.calls[0]![0]).toBe(
      '/api/v1/teams/team%2FA/projects/project%20B/bonus-rounds',
    )
    expect(JSON.parse(fetcher.mock.calls[0]![1].body)).toEqual(body)
  })
})
