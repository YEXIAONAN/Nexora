import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { ApiError } from '@/api/client'
import * as api from '@/api/contributions'
import * as projects from '@/api/projects'
import { useAuthStore } from '@/stores/auth'
import { useTeamsStore } from '@/stores/teams'
import MyContributionResultsView from './MyContributionResultsView.vue'

vi.mock('@/api/contributions', () => ({
  listMyContributionResults: vi.fn(),
  getContribution: vi.fn(),
  getContributionEvidence: vi.fn(),
  listContributionVersions: vi.fn(),
  acknowledgeContributionReceipt: vi.fn(),
}))
vi.mock('@/api/projects', () => ({ listProjects: vi.fn(), listProjectMembers: vi.fn() }))
const timestamp = '2026-11-19T06:00:00Z'
const confirmation: api.ContributionConfirmation = {
  id: 'confirmation',
  case_version_id: 'version',
  confirmed_by: 'appraiser',
  rationale: '独立核对通过',
  version_digest: 'a'.repeat(64),
  membership_snapshot: { me: 'active' },
  confirmed_at: timestamp,
}
const result: api.ContributionResult = {
  notice: {
    id: 'notice',
    case_id: 'case',
    case_version_id: 'version',
    confirmation_id: 'confirmation',
    recipient_id: 'me',
    created_by: 'lead',
    channel: 'authenticated_ack',
    prepared_at: timestamp,
    receipt: null,
  },
  confirmation,
  summary: '验收成果正式分配',
  shares: [
    {
      user_id: 'me',
      share_bps: 10000,
      basis: '交付核心成果',
      statement: '人工说明',
      evidence_refs: [],
      membership_state: 'active',
    },
  ],
}
const version: api.ContributionVersion = {
  id: 'version',
  case_id: 'case',
  version: 1,
  status: 'confirmed',
  supersedes_version_id: null,
  proposed_by: 'lead',
  summary: result.summary,
  evidence_snapshot: {},
  proposed_at: timestamp,
  submitted_at: timestamp,
  submitted_digest: confirmation.version_digest,
  shares: result.shares,
  confirmation,
  total_bps: 10000,
  ready_to_submit: false,
}
const detail: api.ContributionCaseDetail = {
  case: {
    id: 'case',
    team_id: 'A',
    project_id: 'project',
    task_id: 'task',
    submission_version_id: 'submission',
    review_id: 'review',
    latest_version_id: 'version',
    effective_confirmed_version_id: 'version',
    case_workflow_status: 'confirmed',
    revision: 8,
    created_by: 'lead',
    created_at: timestamp,
    updated_at: timestamp,
    undisputed_confirmed: true,
  },
  latest: version,
  effective: version,
  permissions: {
    can_edit: false,
    can_submit: false,
    can_withdraw: false,
    can_confirm: false,
    can_revise: false,
    can_dispute: true,
    can_notify: false,
  },
}
const evidence: api.ContributionEvidence = {
  task_id: 'task',
  task_title: '真实验收成果',
  task_owner_id: 'me',
  submission_version_id: 'submission',
  author_id: 'me',
  summary: '成果说明',
  evidence_references: [],
  submitted_at: timestamp,
  review_id: 'review',
  reviewer_id: 'reviewer',
  decision: 'approve',
  comment: '验收通过',
  reviewed_at: timestamp,
  attachments: [],
}
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((r) => {
    resolve = r
  })
  return { promise, resolve }
}
async function setup() {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().$patch({
    user: { id: 'me' } as NonNullable<ReturnType<typeof useAuthStore>['user']>,
    initialized: true,
  })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/teams/:teamId/contribution-results', component: MyContributionResultsView },
      { path: '/teams/:teamId/projects', name: 'projects', component: { template: '<div />' } },
      {
        path: '/teams/:teamId/projects/:projectId/contributions/:caseId',
        name: 'contribution-case',
        component: { template: '<div />' },
      },
    ],
  })
  await router.push('/teams/A/contribution-results')
  const wrapper = mount(MyContributionResultsView, { global: { plugins: [pinia, router] } })
  await flushPromises()
  return { wrapper, router }
}
beforeEach(() => {
  vi.resetAllMocks()
  vi.mocked(projects.listProjects).mockResolvedValue([
    { id: 'project', name: '项目 A', status: 'active' } as projects.Project,
  ])
  vi.mocked(projects.listProjectMembers).mockResolvedValue([
    { user_id: 'appraiser', display_name: '独立核定人', active: true },
  ] as projects.ProjectMember[])
  vi.mocked(api.listMyContributionResults).mockResolvedValue(structuredClone([result]))
  vi.mocked(api.getContribution).mockResolvedValue(structuredClone(detail))
  vi.mocked(api.getContributionEvidence).mockResolvedValue(structuredClone(evidence))
  vi.mocked(api.listContributionVersions).mockResolvedValue(structuredClone([version]))
  vi.mocked(api.acknowledgeContributionReceipt).mockResolvedValue({
    id: 'receipt',
    case_id: 'case',
    revision: 9,
    client_request_id: 'request',
  })
})
describe('本人贡献结果与独立收件', () => {
  it('深链接先于团队列表加载时，列表到达后仍保持路由对应团队', async () => {
    const { wrapper } = await setup()
    const teams = useTeamsStore()
    teams.teams = [
      { id: 'B', name: '其他团队' },
      { id: 'A', name: '路由团队' },
    ] as typeof teams.teams
    await flushPromises()
    expect(teams.currentTeamId).toBe('A')
    expect(wrapper.text()).toContain('真实验收成果')
    wrapper.unmount()
  })

  it('选择其他 Team 后在路由等待期间立即隐藏旧数据与操作', async () => {
    const { wrapper } = await setup()
    useTeamsStore().$patch({ currentTeamId: 'B' })
    await flushPromises()
    expect(wrapper.text()).not.toContain('真实验收成果')
    expect(wrapper.find('form').exists()).toBe(false)
    expect(api.acknowledgeContributionReceipt).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('展示具体核定结果与未收件状态，GET 不自动收件', async () => {
    const { wrapper } = await setup()
    expect(wrapper.text()).toContain('100.00%')
    expect(wrapper.text()).toContain('真实验收成果')
    expect(wrapper.text()).toContain(confirmation.version_digest)
    expect(wrapper.text()).toContain('尚无可信收件凭据')
    expect(wrapper.text()).toContain('确认收到，仅表示你已收到此结果，不代表同意贡献分配。')
    expect(api.acknowledgeContributionReceipt).not.toHaveBeenCalled()
    expect(wrapper.get('form button').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })
  it('显式阅读后只 POST 一次，绑定展示的版本与摘要', async () => {
    const pending = deferred<api.ContributionCommandResult>()
    vi.mocked(api.acknowledgeContributionReceipt).mockReturnValue(pending.promise)
    const { wrapper } = await setup()
    await wrapper.get('input[type="checkbox"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    await wrapper.get('form').trigger('submit')
    expect(api.acknowledgeContributionReceipt).toHaveBeenCalledTimes(1)
    expect(api.acknowledgeContributionReceipt).toHaveBeenCalledWith(
      'A',
      'project',
      'case',
      'notice',
      expect.objectContaining({
        case_version_id: 'version',
        confirmation_id: 'confirmation',
        content_digest: confirmation.version_digest,
        expected_version: 8,
        received: true,
        client_request_id: expect.any(String),
      }),
      expect.any(AbortSignal),
    )
    pending.resolve({ id: 'receipt', case_id: 'case', revision: 9, client_request_id: 'request' })
    await flushPromises()
    wrapper.unmount()
  })
  it('匹配版本的真实凭据显示已收到，仍说明不代表同意', async () => {
    const received = structuredClone(result)
    received.notice.receipt = {
      id: 'receipt',
      notice_id: 'notice',
      case_version_id: 'version',
      confirmation_id: 'confirmation',
      subject_id: 'me',
      content_digest: confirmation.version_digest,
      verified_at: timestamp,
      sent_at: timestamp,
      acknowledged_at: timestamp,
    }
    vi.mocked(api.listMyContributionResults).mockResolvedValue([received])
    const { wrapper } = await setup()
    expect(wrapper.text()).toContain('已确认收到（不代表同意）')
    expect(wrapper.find('form').exists()).toBe(false)
    expect(api.acknowledgeContributionReceipt).not.toHaveBeenCalled()
    wrapper.unmount()
  })
  it('409 重新读取并清除阅读确认，不自动重发', async () => {
    vi.mocked(api.acknowledgeContributionReceipt).mockRejectedValue(
      new ApiError('stale revision', 409),
    )
    const { wrapper } = await setup()
    await wrapper.get('input[type="checkbox"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.listMyContributionResults).toHaveBeenCalledTimes(2)
    expect(api.acknowledgeContributionReceipt).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[role="alert"]').text()).toContain('状态可能已改变')
    expect((wrapper.get('input').element as HTMLInputElement).checked).toBe(false)
    wrapper.unmount()
  })
  it('Team 切换清除旧结果，迟到写响应不刷新新 Team', async () => {
    const pending = deferred<api.ContributionCommandResult>()
    vi.mocked(api.acknowledgeContributionReceipt).mockReturnValue(pending.promise)
    const { wrapper, router } = await setup()
    await wrapper.get('input[type="checkbox"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    vi.mocked(api.listMyContributionResults).mockResolvedValue([])
    await router.push('/teams/B/contribution-results')
    await flushPromises()
    expect(wrapper.text()).not.toContain('真实验收成果')
    expect(wrapper.text()).toContain('暂无可查看')
    pending.resolve({ id: 'receipt', case_id: 'case', revision: 9, client_request_id: 'request' })
    await flushPromises()
    expect(api.listMyContributionResults).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
  it('同一案例的多个核定只读取一次详情、证据和版本，并显示真实核定人', async () => {
    const confirmation2 = {
      ...confirmation,
      id: 'confirmation2',
      case_version_id: 'version2',
    }
    const version2 = {
      ...version,
      id: 'version2',
      version: 2,
      confirmation: confirmation2,
    }
    const result2 = structuredClone(result)
    result2.notice = {
      ...result2.notice,
      id: 'notice2',
      case_version_id: 'version2',
      confirmation_id: 'confirmation2',
    }
    result2.confirmation = confirmation2
    vi.mocked(api.listMyContributionResults).mockResolvedValue([
      structuredClone(result),
      structuredClone(result2),
    ])
    vi.mocked(api.listContributionVersions).mockResolvedValue([version, version2])
    const { wrapper } = await setup()
    expect(wrapper.findAll('[data-testid="contribution-result"]')).toHaveLength(2)
    expect(api.getContribution).toHaveBeenCalledTimes(1)
    expect(api.getContributionEvidence).toHaveBeenCalledTimes(1)
    expect(api.listContributionVersions).toHaveBeenCalledTimes(1)
    expect(projects.listProjectMembers).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('独立核定人')
    wrapper.unmount()
  })

  it('被推翻的核定仍显示原始结果和历史语义', async () => {
    vi.mocked(api.listContributionVersions).mockResolvedValue([
      { ...version, status: 'overturned' },
    ])
    vi.mocked(api.getContribution).mockResolvedValue({
      ...detail,
      case: { ...detail.case, effective_confirmed_version_id: null, undisputed_confirmed: false },
    })
    const { wrapper } = await setup()
    expect(wrapper.text()).toContain('已被推翻')
    expect(wrapper.text()).toContain('历史核定版本')
    expect(wrapper.text()).toContain('当前不计入无争议正式贡献')
    wrapper.unmount()
  })
  it('权限撤销后清除敏感结果并解释不可访问', async () => {
    const { wrapper } = await setup()
    vi.mocked(api.getContribution).mockRejectedValue(new ApiError('removed', 404))
    await wrapper.get('header button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).not.toContain('真实验收成果')
    expect(wrapper.text()).toContain('当前没有贡献读取权限')
    wrapper.unmount()
  })
})
