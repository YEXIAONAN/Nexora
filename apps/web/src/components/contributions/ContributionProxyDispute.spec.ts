import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import * as api from '@/api/contributions'
import { ApiError } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import ContributionProxyDispute from './ContributionProxyDispute.vue'
vi.mock('@/api/contributions', () => ({ getVerifiedContributionDisputeRequest: vi.fn() }))
const evidence: api.ContributionIdentityEvidence = {
  id: 'request1',
  subject_id: 'former-member',
  case_version_id: 'version1',
  confirmation_id: 'confirmation1',
  statement: '本人明确请求复核此分配',
  content_digest: 'a'.repeat(64),
  verified_at: '2026-11-19T00:00:00Z',
}
const detail = {
  case: { id: 'case1', revision: 7 },
  effective: { id: 'version1', confirmation: { id: 'confirmation1' } },
} as api.ContributionCaseDetail
function setup() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.$patch({ user: { id: 'editor' } as NonNullable<typeof auth.user> })
  return mount(ContributionProxyDispute, {
    props: { team: 'team1', project: 'project1', detail, busy: false },
    global: { plugins: [pinia] },
  })
}
async function lookup(w: ReturnType<typeof setup>) {
  await w.get('input:not([type])').setValue('request1')
  await w.get('form').trigger('submit')
  await flushPromises()
}
beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(api.getVerifiedContributionDisputeRequest).mockResolvedValue({ ...evidence })
})
describe('authorized proxy dispute credential', () => {
  it('requires an explicit lookup and a matching existing credential', async () => {
    const w = setup()
    expect(api.getVerifiedContributionDisputeRequest).not.toHaveBeenCalled()
    await lookup(w)
    expect(api.getVerifiedContributionDisputeRequest).toHaveBeenCalledWith(
      'team1',
      'project1',
      'case1',
      'request1',
      expect.any(AbortSignal),
    )
    expect(w.text()).toContain(evidence.statement)
    expect(w.text()).toContain('服务端核验时间')
    expect(w.emitted('submit')).toBeUndefined()
    w.unmount()
  })
  it('submits the exact verified statement and subject only after explicit review', async () => {
    const w = setup()
    await lookup(w)
    expect(w.findAll('form')[1]!.get('button').attributes('disabled')).toBeDefined()
    await w.get('input[type="checkbox"]').setValue(true)
    await w.findAll('form')[1]!.trigger('submit')
    expect(w.emitted('submit')?.[0]?.[0]).toMatchObject({
      expected_version: 7,
      reason: evidence.statement,
      case_version_id: 'version1',
      confirmation_id: 'confirmation1',
      on_behalf_of: 'former-member',
      identity_request_id: 'request1',
    })
    w.unmount()
  })
  it('rejects evidence bound to a different confirmation without a text fallback', async () => {
    vi.mocked(api.getVerifiedContributionDisputeRequest).mockResolvedValue({
      ...evidence,
      confirmation_id: 'old-confirmation',
    })
    const w = setup()
    await lookup(w)
    expect(w.text()).toContain('凭据不匹配当前有效核定')
    expect(w.find('input[type="checkbox"]').exists()).toBe(false)
    expect(w.find('textarea').exists()).toBe(false)
    expect(w.emitted('submit')).toBeUndefined()
    w.unmount()
  })
  it('clears the proof and acknowledgement after the authoritative revision changes', async () => {
    const w = setup()
    await lookup(w)
    await w.get('input[type="checkbox"]').setValue(true)
    await w.setProps({ detail: { ...detail, case: { ...detail.case, revision: 8 } } })
    await flushPromises()
    expect(w.text()).not.toContain(evidence.statement)
    expect(w.find('input[type="checkbox"]').exists()).toBe(false)
    w.unmount()
  })
  it('allows explicit retry of the same request after a transient read failure', async () => {
    vi.mocked(api.getVerifiedContributionDisputeRequest)
      .mockRejectedValueOnce(new ApiError('暂时不可用', 500))
      .mockResolvedValueOnce({ ...evidence })
    const w = setup()
    await lookup(w)
    expect(w.get('[role="alert"]').text()).toContain('服务暂时不可用')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(api.getVerifiedContributionDisputeRequest).toHaveBeenCalledTimes(2)
    expect(w.text()).toContain(evidence.statement)
    w.unmount()
  })
})
