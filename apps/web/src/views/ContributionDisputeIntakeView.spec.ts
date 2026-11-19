import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import { receiveContributionDisputeRequest } from '@/api/contributions'
import { useAuthStore } from '@/stores/auth'
import ContributionDisputeIntakeView from './ContributionDisputeIntakeView.vue'
vi.mock('@/api/contributions', () => ({ receiveContributionDisputeRequest: vi.fn() }))
async function setup() {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().$patch({
    user: { id: 'me' } as NonNullable<ReturnType<typeof useAuthStore>['user']>,
  })
  const wrapper = mount(ContributionDisputeIntakeView, { global: { plugins: [pinia] } })
  for (const input of wrapper.findAll('input'))
    await input.setValue('11111111-1111-4111-8111-111111111111')
  await wrapper.get('textarea').setValue('我的异议声明')
  return wrapper
}
describe('无结果披露的本人请求受理', () => {
  it('只 POST 本人给定引用，显示受理编号且不声称核验成功', async () => {
    vi.mocked(receiveContributionDisputeRequest).mockResolvedValue({
      request_id: 'own-request',
      received: true,
    })
    const wrapper = await setup()
    expect(receiveContributionDisputeRequest).not.toHaveBeenCalled()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(receiveContributionDisputeRequest).toHaveBeenCalledWith(
      expect.objectContaining({
        reason: '我的异议声明',
        client_request_id: expect.any(String),
        confirmation_id: '11111111-1111-4111-8111-111111111111',
      }),
      expect.any(AbortSignal),
    )
    expect(wrapper.get('[role="status"]').text()).toContain('own-request')
    expect(wrapper.get('[role="status"]').text()).toContain('受理不表示核验成功')
    expect(wrapper.text()).toContain('不提供项目、成员或贡献结果查询')
    wrapper.unmount()
  })
  it('注销立即清除声明与受理数据，迟到响应不能恢复', async () => {
    let resolve!: (r: { request_id: string; received: boolean }) => void
    vi.mocked(receiveContributionDisputeRequest).mockReturnValue(
      new Promise((r) => {
        resolve = r
      }),
    )
    const wrapper = await setup()
    await wrapper.get('form').trigger('submit')
    useAuthStore().expire()
    await flushPromises()
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('')
    resolve({ request_id: 'late-secret', received: true })
    await flushPromises()
    expect(wrapper.text()).not.toContain('late-secret')
    expect(wrapper.get('fieldset').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })
})
