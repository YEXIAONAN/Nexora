import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import * as api from '@/api/contributions'
import * as projects from '@/api/projects'
import { getTeam } from '@/api/teams'
import { useAuthStore } from '@/stores/auth'
import ContributionAppraisalView from './ContributionAppraisalView.vue'
vi.mock('@/api/contributions', () => ({
  getContributionEligibility: vi.fn(),
  listContributionAppraisers: vi.fn(),
  listMyAppraisableContributions: vi.fn(),
  grantContributionAppraiser: vi.fn(),
  revokeContributionAppraiser: vi.fn(),
}))
vi.mock('@/api/projects', () => ({ getProject: vi.fn(), listProjectMembers: vi.fn() }))
vi.mock('@/api/teams', () => ({ getTeam: vi.fn() }))
async function setup() {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().$patch({
    user: { id: 'me' } as NonNullable<ReturnType<typeof useAuthStore>['user']>,
  })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      {
        path: '/teams/:teamId/projects/:projectId/contribution-appraisal',
        component: ContributionAppraisalView,
      },
      {
        path: '/teams/:teamId/projects/:projectId/contributions',
        name: 'project-contributions',
        component: { template: '<div />' },
      },
      {
        path: '/teams/:teamId/projects/:projectId/contributions/:caseId',
        name: 'contribution-case',
        component: { template: '<div />' },
      },
    ],
  })
  await router.push('/teams/A/projects/a/contribution-appraisal')
  const wrapper = mount(ContributionAppraisalView, { global: { plugins: [pinia, router] } })
  await flushPromises()
  return { wrapper, router }
}
const grant: api.ContributionAppraiser = {
  user_id: 'member',
  status: 'active',
  version: 3,
  member_authorization_version: 2,
  qualified: true,
  granted_by: 'me',
  changed_by: 'me',
  reason: '明确授权',
  granted_at: '2026-11-19T06:00:00Z',
}
beforeEach(() => {
  vi.resetAllMocks()
  vi.mocked(projects.getProject).mockResolvedValue({
    id: 'a',
    name: '项目 A',
    lead_id: 'me',
    status: 'active',
    can_manage: true,
  } as projects.Project)
  vi.mocked(projects.listProjectMembers).mockResolvedValue([
    { user_id: 'me', active: true, display_name: '负责人' },
    { user_id: 'member', active: true, display_name: '独立成员' },
  ] as projects.ProjectMember[])
  vi.mocked(getTeam).mockResolvedValue({ my_role: 'member' } as Awaited<ReturnType<typeof getTeam>>)
  vi.mocked(api.getContributionEligibility).mockResolvedValue({
    explicit_appraiser: false,
    can_edit_cases: true,
    authorization_version: 0,
  })
  vi.mocked(api.listContributionAppraisers).mockResolvedValue([])
  vi.mocked(api.listMyAppraisableContributions).mockResolvedValue([])
  vi.mocked(api.grantContributionAppraiser).mockResolvedValue({
    id: 'member',
    case_id: null,
    revision: 1,
    client_request_id: 'request',
  })
})
describe('独立核定工作台授权', () => {
  it('负责人无明确授权时不声称可核定，也不能自授', async () => {
    const { wrapper } = await setup()
    expect(wrapper.text()).toContain('未获得当前有效的明确核定授权')
    expect(wrapper.text()).toContain('暂无符合本人独立资格')
    expect(wrapper.find('option[value="me"]').exists()).toBe(false)
    expect(wrapper.find('option[value="member"]').exists()).toBe(true)
    wrapper.unmount()
  })
  it('普通无权成员看不到授权或撤销操作', async () => {
    vi.mocked(projects.getProject).mockResolvedValue({
      id: 'a',
      lead_id: 'other',
      status: 'active',
      can_manage: false,
    } as projects.Project)
    vi.mocked(api.listContributionAppraisers).mockResolvedValue([grant])
    const { wrapper } = await setup()
    expect(wrapper.find('form').exists()).toBe(false)
    expect(wrapper.findAll('button').some((b) => b.text() === '撤销核定权')).toBe(false)
    wrapper.unmount()
  })
  it('人工授权携带服务端版本与请求 ID，成功后重新读资格', async () => {
    const { wrapper } = await setup()
    await wrapper.get('input').setValue('人工独立核定授权')
    await wrapper.get('select').setValue('member')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.grantContributionAppraiser).toHaveBeenCalledWith(
      'A',
      'a',
      {
        user_id: 'member',
        reason: '人工独立核定授权',
        expected_version: 0,
        client_request_id: expect.any(String),
      },
      expect.any(AbortSignal),
    )
    expect(api.getContributionEligibility).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
  it('撤销使用现有授权版本，不会把资格显示变更当作本地事实', async () => {
    vi.mocked(api.listContributionAppraisers).mockResolvedValue([grant])
    const { wrapper } = await setup()
    await wrapper.get('input').setValue('人员更换')
    await wrapper
      .findAll('button')
      .find((b) => b.text() === '撤销核定权')!
      .trigger('click')
    await flushPromises()
    expect(api.revokeContributionAppraiser).toHaveBeenCalledWith(
      'A',
      'a',
      'member',
      { reason: '人员更换', expected_version: 3, client_request_id: expect.any(String) },
      expect.any(AbortSignal),
    )
    wrapper.unmount()
  })
  it('归档项目隐藏授权写操作', async () => {
    vi.mocked(projects.getProject).mockResolvedValue({
      id: 'a',
      lead_id: 'me',
      status: 'archived',
      can_manage: false,
    } as projects.Project)
    vi.mocked(api.listContributionAppraisers).mockResolvedValue([grant])
    const { wrapper } = await setup()
    expect(wrapper.find('form').exists()).toBe(false)
    expect(wrapper.findAll('button').some((b) => b.text() === '撤销核定权')).toBe(false)
    wrapper.unmount()
  })
})
