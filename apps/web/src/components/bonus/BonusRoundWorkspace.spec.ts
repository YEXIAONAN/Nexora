import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type {
  BonusAction,
  BonusProjectWorkspace,
  BonusRoundDetail,
  BonusWorkspace,
} from '@/api/bonus'
import { useAuthStore } from '@/stores/auth'
import BonusRoundWorkspace from './BonusRoundWorkspace.vue'

const at = '2026-11-20T02:00:00Z'
let metadata: BonusRoundDetail
let workspace: BonusWorkspace
let project: BonusProjectWorkspace
let forbidden: boolean
let postError: { status: number; message: string } | null
let calls: { path: string; options: RequestInit; body: Record<string, unknown> }[]
let mounted: VueWrapper[] = []
function response(data: unknown) {
  return new Response(JSON.stringify({ data }), { status: 200 })
}
function deny(status: number, message: string) {
  return new Response(JSON.stringify({ error: { message } }), { status })
}
function button(wrapper: VueWrapper, text: string) {
  const result = wrapper.findAll('button').find((b) => b.text() === text)
  if (!result) throw new Error(`Missing button ${text}`)
  return result
}
async function field(wrapper: VueWrapper, label: string, value: string | boolean) {
  const node = wrapper.findAll('label').find((l) => l.text().startsWith(label))
  if (!node) throw new Error(`Missing field ${label}`)
  await node.find('input,select,textarea').setValue(value)
}
async function render() {
  const wrapper = mount(BonusRoundWorkspace, {
    props: { teamId: 'A', projectId: 'project', roundId: 'round' },
  })
  mounted.push(wrapper)
  await flushPromises()
  return wrapper
}
function writes() {
  return calls.filter((c) => c.options.method !== 'GET')
}

beforeEach(() => {
  setActivePinia(createPinia())
  useAuthStore().setUser({
    id: 'creator',
    username: 'creator',
    display_name: '创建者',
    system_root: true,
  } as Parameters<ReturnType<typeof useAuthStore>['setUser']>[0])
  metadata = {
    id: 'round',
    project_id: 'project',
    funding_key: 'award',
    funding_label: '专项奖励',
    funding_source: '独立资金',
    revision: 7,
    status: 'draft',
    created_at: at,
    latest_version: null,
    effective_confirmed_version: null,
    amounts_visible: false,
    versions: [],
  }
  workspace = {
    round: { ...metadata },
    version: {
      id: 'v1',
      version: 1,
      status: 'draft',
      total_budget_minor: 3000,
      plan_unallocated_minor: 0,
      rationale: '人工评估',
      published_at: null,
      sources: [
        {
          confirmation_id: 'source',
          task_title: '接口开发',
          source_budget_minor: 3000,
          source_unallocated_minor: 0,
          reason: '已确认来源',
          lines: [
            {
              user_id: 'beneficiary',
              display_name: '小林',
              share_bps: 10000,
              eligible: true,
              amount_minor: 3000,
              reason: '人工奖金理由',
            },
          ],
        },
      ],
    },
    permissions: {
      edit: false,
      submit: false,
      return: false,
      confirm: false,
      publish: false,
      revise: false,
      record_payment: false,
    },
    grants: [],
    events: [],
    payments: [],
  }
  project = {
    can_create: false,
    members: [
      { user_id: 'creator', display_name: '创建者' },
      { user_id: 'other', display_name: '小周' },
    ],
    sources: [],
  }
  forbidden = false
  postError = null
  calls = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (path: string, options: RequestInit) => {
      const body = options.body ? (JSON.parse(String(options.body)) as Record<string, unknown>) : {}
      calls.push({ path, options, body })
      if (options.method !== 'GET') {
        if (postError) return deny(postError.status, postError.message)
        metadata.revision++
        workspace.round.revision = metadata.revision
        if (path.endsWith('/payments'))
          workspace.payments.push({
            user_id: String(body.user_id),
            amount_minor: 3000,
            reference: String(body.reference),
            recorded_at: at,
          })
        return response({ round_id: 'round', revision: metadata.revision })
      }
      if (path.endsWith('/bonus-workspace')) return response(project)
      if (path.endsWith('/workspace'))
        return forbidden ? deny(403, '显式授权不足') : response(workspace)
      return response(metadata)
    }),
  )
})
afterEach(() => {
  mounted.forEach((w) => w.unmount())
  mounted = []
  vi.unstubAllGlobals()
})

describe('real bonus workspace command wiring', () => {
  it('does not infer write or grant permissions from ROOT', async () => {
    const wrapper = await render()
    expect(wrapper.text()).toContain('当前状态下你没有核定或发布操作权限')
    expect(wrapper.text()).not.toContain('编辑人工方案')
    expect(wrapper.find('form').exists()).toBe(false)
    expect(writes()).toHaveLength(0)
  })

  it.each([
    ['submit', '提交独立核定'],
    ['return', '退回修改'],
    ['confirm', '独立核定'],
    ['publish', '发布正式结果'],
    ['revise', '新建修订版本'],
  ] as [BonusAction, string][])(
    'sends %s only after reason and inline confirmation',
    async (action, label) => {
      workspace.permissions[action] = true
      const wrapper = await render()
      await button(wrapper, label).trigger('click')
      await field(wrapper, '操作理由', '独立核对已完成')
      await field(wrapper, '我已核对当前方案', true)
      await button(wrapper, `确认${label}`).trigger('submit')
      await flushPromises()
      expect(writes()).toHaveLength(1)
      expect(writes()[0]!.path).toBe(
        `/api/v1/teams/A/projects/project/bonus-rounds/round/${action}`,
      )
      expect(writes()[0]!.body).toMatchObject({ expected_revision: 7, reason: '独立核对已完成' })
      expect(writes()[0]!.body.client_request_id).toMatch(/^[0-9a-f-]{36}$/)
      expect(writes()[0]!.options.headers).toMatchObject({ 'X-Nexora-CSRF': '1' })
      expect(wrapper.text()).toContain(`${label}成功`)
    },
  )

  it.each([false, true])(
    'allows creator grant management despite amount workspace 403, revoke=%s',
    async (revoke) => {
      forbidden = true
      project.can_create = true
      const wrapper = await render()
      expect(wrapper.text()).toContain('尚未获得此批次的金额访问授权')
      expect(wrapper.text()).not.toContain('¥30.00')
      expect(wrapper.findAll('option').map((o) => o.text())).not.toContain('创建者')
      await field(wrapper, '授权成员', 'other')
      await field(wrapper, '角色', 'budget')
      if (revoke) await wrapper.findAll('select')[2]!.setValue('true')
      await field(wrapper, '授权变更理由', '职责调整')
      await wrapper.find('form').trigger('submit')
      await flushPromises()
      expect(writes()[0]!.path).toContain('/grants')
      expect(writes()[0]!.body).toMatchObject({
        expected_revision: 7,
        subject_id: 'other',
        role_code: 'budget',
        revoke,
        reason: '职责调整',
      })
    },
  )

  it('saves hand-entered fen and refuses overspending before making a request', async () => {
    workspace.permissions.edit = true
    const wrapper = await render()
    await button(wrapper, '编辑人工方案').trigger('click')
    await field(wrapper, '人工金额（元）', '30.01')
    await field(wrapper, '本次保存理由', '调整人工方案')
    await wrapper.find('form').trigger('submit')
    expect(wrapper.text()).toContain('成员金额超出来源预算')
    expect(writes()).toHaveLength(0)
    await field(wrapper, '人工金额（元）', '0.29')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(writes()[0]!.options.method).toBe('PUT')
    expect(writes()[0]!.path).toContain('/plan')
    expect(writes()[0]!.body).toMatchObject({
      expected_revision: 7,
      total_budget_minor: 3000,
      sources: [
        {
          confirmation_id: 'source',
          source_budget_minor: 3000,
          reason: '已确认来源',
          lines: [
            { user_id: 'beneficiary', eligible: true, amount_minor: 29, reason: '人工奖金理由' },
          ],
        },
      ],
    })
    expect(wrapper.text()).toContain('人工方案保存成功')
  })

  it('records simulated payment without transmitting an amount and prevents duplicates', async () => {
    workspace.permissions.record_payment = true
    workspace.version!.published_at = at
    workspace.version!.status = 'confirmed'
    const wrapper = await render()
    expect(wrapper.text()).toContain('已发布')
    await button(wrapper, '记录 小林 的模拟发放').trigger('click')
    await field(wrapper, '模拟发放参考号', 'SIM-2026-01')
    await field(wrapper, '记录理由', '模拟核对')
    await field(wrapper, '我确认这仅是模拟发放记录', true)
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(writes()[0]!.body).toMatchObject({ user_id: 'beneficiary', reference: 'SIM-2026-01' })
    expect(writes()[0]!.body).not.toHaveProperty('amount_minor')
    expect(wrapper.text()).toContain('已记录模拟发放 · 不可重复')
    expect(wrapper.findAll('button').some((b) => b.text() === '记录 小林 的模拟发放')).toBe(false)
    expect(wrapper.text()).toContain('SIM-2026-01')
  })

  it('explains conflicts, blocks stale writes, then refreshes the revision and request ID', async () => {
    workspace.permissions.submit = true
    postError = { status: 409, message: 'revision conflict' }
    const wrapper = await render()
    await button(wrapper, '提交独立核定').trigger('click')
    await field(wrapper, '操作理由', '提交理由')
    await field(wrapper, '我已核对当前方案', true)
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('revision conflict')
    expect(wrapper.text()).toContain('请刷新后重新检查方案及权限')
    expect(wrapper.find('form').exists()).toBe(false)
    postError = null
    metadata.revision = 12
    workspace.round.revision = 12
    await button(wrapper, '刷新工作区').trigger('click')
    await flushPromises()
    await button(wrapper, '提交独立核定').trigger('click')
    await field(wrapper, '操作理由', '刷新后提交')
    await field(wrapper, '我已核对当前方案', true)
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(writes()[1]!.body.expected_revision).toBe(12)
    expect(writes()[1]!.body.client_request_id).not.toBe(writes()[0]!.body.client_request_id)
  })

  it('surfaces the backend independence rejection without changing the draft', async () => {
    workspace.permissions.confirm = true
    postError = { status: 403, message: 'reviewer is a beneficiary' }
    const wrapper = await render()
    await button(wrapper, '独立核定').trigger('click')
    await field(wrapper, '操作理由', '核对')
    await field(wrapper, '我已核对当前方案', true)
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('reviewer is a beneficiary')
    expect(wrapper.text()).toContain('核定人不能是方案编辑者、受益人或贡献提案核定人')
    expect(wrapper.text()).not.toContain('独立核定成功')
  })

  it('prevents a second request while a command is pending', async () => {
    workspace.permissions.submit = true
    const original = vi.mocked(fetch).getMockImplementation()!
    let complete!: (response: Response) => void
    vi.mocked(fetch).mockImplementation((path, options) => {
      if (options?.method === 'POST')
        return new Promise<Response>((resolve) => {
          complete = resolve
        })
      return original(path, options)
    })
    const wrapper = await render()
    await button(wrapper, '提交独立核定').trigger('click')
    await field(wrapper, '操作理由', '核对完毕')
    await field(wrapper, '我已核对当前方案', true)
    await wrapper.find('form').trigger('submit')
    await wrapper.find('form').trigger('submit')
    expect(
      vi.mocked(fetch).mock.calls.filter(([, options]) => options?.method === 'POST'),
    ).toHaveLength(1)
    expect(wrapper.find('fieldset').attributes('disabled')).toBeDefined()
    complete(response({ round_id: 'round', revision: 8 }))
    await flushPromises()
  })

  it('aborts and ignores old team data even when its fetch resolves after navigation', async () => {
    let resolveOld!: (value: Response) => void
    let oldSignal: AbortSignal | undefined
    const oldData = structuredClone(workspace)
    oldData.version!.rationale = 'A团队机密'
    const fetcher = vi.mocked(fetch)
    fetcher.mockImplementation(async (path, options) => {
      if (String(path).includes('/teams/A/') && String(path).endsWith('/workspace')) {
        oldSignal = options?.signal ?? undefined
        return new Promise<Response>((resolve) => {
          resolveOld = resolve
        })
      }
      if (String(path).endsWith('/bonus-workspace')) return response(project)
      if (String(path).endsWith('/workspace')) return deny(403, 'forbidden')
      return response(metadata)
    })
    const wrapper = mount(BonusRoundWorkspace, {
      props: { teamId: 'A', projectId: 'project', roundId: 'round' },
    })
    mounted.push(wrapper)
    await flushPromises()
    expect(wrapper.text()).toContain('正在加载授权')
    await wrapper.setProps({ teamId: 'B' })
    await flushPromises()
    resolveOld(response(oldData))
    await flushPromises()
    expect(oldSignal?.aborted).toBe(true)
    expect(wrapper.text()).not.toContain('A团队机密')
    expect(wrapper.text()).not.toContain('¥30.00')
    expect(wrapper.text()).toContain('尚未获得此批次的金额访问授权')
  })
})
