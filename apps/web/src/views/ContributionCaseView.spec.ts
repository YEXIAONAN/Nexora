import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import * as api from '@/api/contributions'
import * as projects from '@/api/projects'
import { ApiError } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { useTeamsStore } from '@/stores/teams'
import ContributionCaseView from './ContributionCaseView.vue'
vi.mock('@/api/projects', async (original) => ({
  ...(await original<typeof import('@/api/projects')>()),
  listProjectMembers: vi.fn(),
  getProject: vi.fn(),
}))
vi.mock('@/api/contributions', async (original) => ({
  ...(await original<typeof import('@/api/contributions')>()),
  getContribution: vi.fn(),
  getContributionHistory: vi.fn(),
  getContributionEvidence: vi.fn(),
  getContributionEligibility: vi.fn(),
  confirmContribution: vi.fn(),
  submitContribution: vi.fn(),
  saveContributionDraft: vi.fn(),
  raiseContributionDispute: vi.fn(),
  resolveContributionDispute: vi.fn(),
  prepareContributionNotice: vi.fn(),
}))
const timestamp = '2026-11-19T00:00:00Z'
const confirmation: api.ContributionConfirmation = {
  id: 'confirm1',
  case_version_id: 'v1',
  confirmed_by: 'judge',
  rationale: '核定理由',
  version_digest: 'a'.repeat(64),
  membership_snapshot: { person: 'active' },
  confirmed_at: timestamp,
}
const v1: api.ContributionVersion = {
  id: 'v1',
  case_id: 'case1',
  version: 1,
  status: 'confirmed',
  supersedes_version_id: null,
  proposed_by: 'editor',
  summary: '旧正式分配',
  evidence_snapshot: {},
  proposed_at: timestamp,
  submitted_at: timestamp,
  submitted_digest: 'a'.repeat(64),
  shares: [
    {
      user_id: 'person',
      share_bps: 10000,
      basis: '有效依据',
      statement: '正式贡献说明',
      evidence_refs: [],
      membership_state: 'active',
    },
  ],
  confirmation,
  total_bps: 10000,
  ready_to_submit: true,
}
const fixture = (): api.ContributionCaseDetail => ({
  case: {
    id: 'case1',
    team_id: 'A',
    project_id: 'p1',
    task_id: 'task1',
    submission_version_id: 'submission1',
    review_id: 'review1',
    latest_version_id: 'v2',
    effective_confirmed_version_id: 'v1',
    case_workflow_status: 'revising',
    revision: 7,
    created_by: 'editor',
    created_at: timestamp,
    updated_at: timestamp,
    undisputed_confirmed: true,
  },
  latest: {
    ...v1,
    id: 'v2',
    version: 2,
    status: 'draft',
    confirmation: null,
    summary: '新修订草案',
    supersedes_version_id: 'v1',
    shares: [],
    total_bps: 0,
    ready_to_submit: false,
  },
  effective: v1,
  permissions: {
    can_edit: false,
    can_submit: false,
    can_withdraw: false,
    can_confirm: false,
    can_revise: false,
    can_dispute: false,
    can_notify: false,
  },
})
const history = (): api.ContributionHistory => ({
  versions: [v1],
  events: [],
  adjustments: [],
  disputes: [],
  conflicts: [],
  notices: [],
})
beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(projects.listProjectMembers).mockResolvedValue([])
  vi.mocked(projects.getProject).mockResolvedValue({ status: 'active' } as projects.Project)
  vi.mocked(api.getContribution).mockResolvedValue(fixture())
  vi.mocked(api.getContributionHistory).mockResolvedValue(history())
  vi.mocked(api.getContributionEvidence).mockResolvedValue({
    task_id: 'task1',
    task_title: '真实成果',
    task_owner_id: 'person',
    submission_version_id: 'submission1',
    author_id: 'person',
    summary: '验收成果说明',
    evidence_references: [],
    submitted_at: timestamp,
    review_id: 'review1',
    reviewer_id: 'reviewer',
    decision: 'approve',
    comment: '正式审核通过',
    reviewed_at: timestamp,
    attachments: [],
  })
  vi.mocked(api.getContributionEligibility).mockResolvedValue({
    explicit_appraiser: true,
    can_edit_cases: true,
    authorization_version: 1,
  })
})
async function setup() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.$patch({ user: { id: 'editor' } as NonNullable<typeof auth.user>, initialized: true })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      {
        path: '/teams/:teamId/projects/:projectId/contributions/:caseId',
        component: ContributionCaseView,
      },
      {
        path: '/teams/:teamId/projects/:projectId/contributions',
        name: 'project-contributions',
        component: { template: '<div />' },
      },
    ],
  })
  await router.push('/teams/A/projects/p1/contributions/case1')
  const wrapper = mount(ContributionCaseView, { global: { plugins: [pinia, router] } })
  await flushPromises()
  return { router, wrapper, auth, teams: useTeamsStore() }
}
const button = (w: ReturnType<typeof mount>, label: string) =>
  w.findAll('button').find((b) => b.text() === label)
async function fillLabel(w: ReturnType<typeof mount>, label: string, value: string) {
  const item = w.findAll('label').find((l) => l.text() === label)
  await item!.get('input,textarea').setValue(value)
}
function enableConfirm() {
  const d = fixture()
  d.latest = { ...v1, id: 'v2', version: 2, status: 'pending_review', confirmation: null }
  d.permissions.can_confirm = true
  vi.mocked(api.getContribution).mockResolvedValue(d)
}
describe('contribution case authority and version boundaries', () => {
  it('keeps a new draft and old effective result separate', async () => {
    const { wrapper: w } = await setup()
    expect(w.get('[data-testid="contribution-latest"]').text()).toContain('新修订草案')
    expect(w.get('[data-testid="contribution-effective"]').text()).toContain('旧正式分配')
    expect(w.get('[data-testid="contribution-effective"]').text()).not.toContain('新修订草案')
    w.unmount()
  })
  it('hides writes despite general appraiser eligibility when case capabilities deny them', async () => {
    const { wrapper: w } = await setup()
    expect(button(w, '正式核定')).toBeUndefined()
    expect(button(w, '提交待核定')).toBeUndefined()
    expect(w.find('[aria-label="贡献草案编辑"]').exists()).toBe(false)
    w.unmount()
  })
  it('shows editing only from the server case capability', async () => {
    const d = fixture()
    d.permissions.can_edit = true
    vi.mocked(api.getContribution).mockResolvedValue(d)
    const { wrapper: w } = await setup()
    expect(w.find('[aria-label="贡献草案编辑"]').exists()).toBe(true)
    expect(button(w, '提交待核定')).toBeUndefined()
    w.unmount()
  })
  it('requires explicit immutable-history acknowledgement before confirmation', async () => {
    enableConfirm()
    vi.mocked(api.confirmContribution).mockResolvedValue({
      id: 'x',
      case_id: 'case1',
      revision: 8,
      client_request_id: 'key',
    })
    const { wrapper: w } = await setup()
    await fillLabel(w, '操作原因', '独立复核完成')
    expect(button(w, '正式核定')!.attributes('disabled')).toBeDefined()
    await w.get('input[type="checkbox"]').setValue(true)
    await button(w, '正式核定')!.trigger('click')
    await flushPromises()
    expect(api.confirmContribution).toHaveBeenCalledTimes(1)
    expect(vi.mocked(api.confirmContribution).mock.calls[0]?.[3]).toMatchObject({
      expected_version: 7,
      case_version_id: 'v2',
      reason: '独立复核完成',
    })
    w.unmount()
  })
  it('does not issue two confirmations while a request is pending', async () => {
    let resolve!: (v: api.ContributionCommandResult) => void
    vi.mocked(api.confirmContribution).mockReturnValue(
      new Promise((r) => {
        resolve = r
      }),
    )
    enableConfirm()
    const { wrapper: w } = await setup()
    await fillLabel(w, '操作原因', '复核')
    await w.get('input[type="checkbox"]').setValue(true)
    await button(w, '正式核定')!.trigger('click')
    await button(w, '正式核定')!.trigger('click')
    expect(api.confirmContribution).toHaveBeenCalledTimes(1)
    resolve({ id: 'x', case_id: 'case1', revision: 8, client_request_id: 'key' })
    await flushPromises()
    w.unmount()
  })
  it('re-reads authority after 409 without automatically confirming again', async () => {
    enableConfirm()
    vi.mocked(api.confirmContribution).mockRejectedValue(new ApiError('旧版本', 409))
    const { wrapper: w } = await setup()
    await fillLabel(w, '操作原因', '复核')
    await w.get('input[type="checkbox"]').setValue(true)
    await button(w, '正式核定')!.trigger('click')
    await flushPromises()
    expect(api.getContribution).toHaveBeenCalledTimes(2)
    expect(api.confirmContribution).toHaveBeenCalledTimes(1)
    expect(w.get('[role="alert"]').text()).toContain('状态可能已改变')
    w.unmount()
  })
  it('shows dispute progress and preserves overturned decision history', async () => {
    const d = fixture()
    d.case.case_workflow_status = 'disputed'
    d.case.undisputed_confirmed = false
    vi.mocked(api.getContribution).mockResolvedValue(d)
    const h = history()
    const dispute: api.ContributionDispute = {
      id: 'd1',
      case_version_id: 'v1',
      confirmation_id: 'confirm1',
      raised_by: 'person',
      on_behalf_of: null,
      identity_request_id: null,
      reason: '分配异议',
      created_at: timestamp,
      resolution: null,
      can_resolve: false,
      verification_method: '',
      verification_evidence_ref: '',
      verification_note: '',
      identity_request_evidence: null,
    }
    h.disputes = [
      dispute,
      {
        ...dispute,
        id: 'd2',
        resolution: {
          id: 'r1',
          dispute_id: 'd2',
          decision: 'overturn',
          resolved_by: 'arbiter',
          reason: '证据不足',
          resolved_at: timestamp,
        },
      },
    ]
    vi.mocked(api.getContributionHistory).mockResolvedValue(h)
    const { wrapper: w } = await setup()
    expect(w.text()).toContain('等待独立裁定')
    expect(w.text()).toContain('推翻原核定；原记录保留')
    expect(button(w, '推翻原核定')).toBeUndefined()
    w.unmount()
  })
  it('does not equate a prepared notice with real delivery', async () => {
    const d = fixture()
    d.permissions.can_notify = true
    vi.mocked(api.getContribution).mockResolvedValue(d)
    const h = history()
    h.notices = [
      {
        id: 'n1',
        case_id: 'case1',
        case_version_id: 'v1',
        confirmation_id: 'confirm1',
        recipient_id: 'person',
        created_by: 'editor',
        channel: 'authenticated_ack',
        prepared_at: timestamp,
        receipt: null,
      },
    ]
    vi.mocked(api.getContributionHistory).mockResolvedValue(h)
    const { wrapper: w } = await setup()
    expect(w.text()).toContain('仅已准备告知 · 尚无真实收件证据')
    expect(w.text()).not.toContain('已有本人收件凭据')
    w.unmount()
  })
  it('drops a late previous-team response after a route switch', async () => {
    let resolve!: (d: api.ContributionCaseDetail) => void
    vi.mocked(api.getContribution).mockImplementation((team) =>
      team === 'A'
        ? new Promise((r) => {
            resolve = r
          })
        : Promise.resolve({ ...fixture(), latest: { ...v1, summary: 'B团队内容' } }),
    )
    const { wrapper: w, router } = await setup()
    await router.push('/teams/B/projects/p2/contributions/case2')
    await flushPromises()
    resolve(fixture())
    await flushPromises()
    expect(w.text()).toContain('B团队内容')
    expect(w.text()).not.toContain('新修订草案')
    w.unmount()
  })
  it('clears sensitive state on session expiry before route unmount', async () => {
    const { wrapper: w, auth } = await setup()
    auth.expire()
    await flushPromises()
    expect(w.text()).not.toContain('新修订草案')
    expect(w.text()).not.toContain('旧正式分配')
    w.unmount()
  })
  it('clears old Team details as soon as selected Team changes', async () => {
    const { wrapper: w, teams } = await setup()
    teams.$patch({ teams: [{ id: 'A' }, { id: 'B' }] as typeof teams.teams, currentTeamId: 'B' })
    await flushPromises()
    expect(w.text()).not.toContain('新修订草案')
    expect(w.text()).not.toContain('旧正式分配')
    w.unmount()
  })
  it('hides self conflict recording for an archived project', async () => {
    vi.mocked(projects.getProject).mockResolvedValue({ status: 'archived' } as projects.Project)
    const { wrapper: w } = await setup()
    expect(w.text()).not.toContain('补充直接利益冲突记录')
    w.unmount()
  })
})
