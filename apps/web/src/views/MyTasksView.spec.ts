import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { ApiError } from '@/api/client'
import { listProjects, type Project } from '@/api/projects'
import { getTaskTree, type TaskTree } from '@/api/tasks'
import { listTeamMembers } from '@/api/teams'
import { useAuthStore } from '@/stores/auth'
import { useTeamsStore } from '@/stores/teams'
import MyTasksView from './MyTasksView.vue'

vi.mock('@/api/projects', () => ({ listProjects: vi.fn() }))
vi.mock('@/api/tasks', async (original) => ({
  ...(await original<typeof import('@/api/tasks')>()),
  getTaskTree: vi.fn(),
}))
vi.mock('@/api/teams', async (original) => ({
  ...(await original<typeof import('@/api/teams')>()),
  listTeamMembers: vi.fn(),
}))

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => {
    resolve = done
  })
  return { promise, resolve }
}

const project = (team: string): Project => ({
  id: `project-${team}`,
  team_id: team,
  name: `Project ${team}`,
  description: '',
  lead_id: 'me',
  status: 'active',
  planned_start_at: null,
  planned_end_at: null,
  archived_at: null,
  version: 1,
  created_by: 'me',
  created_at: '2026-11-17T00:00:00Z',
  updated_at: '2026-11-17T00:00:00Z',
  can_manage: false,
  can_handoff: false,
})
const tree = (team: string): TaskTree => ({
  tasks: [
    {
      id: `task-${team}`,
      team_id: team,
      project_id: `project-${team}`,
      parent_id: null,
      depth: 1,
      title: `Task ${team}`,
      description: '',
      owner_id: 'me',
      collaborator_ids: [],
      priority: 'normal',
      planned_start_at: null,
      planned_end_at: null,
      status: 'pending',
      actual_progress: 0,
      effective_progress_bps: 0,
      weight: 1,
      is_leaf: true,
      blocked: false,
      can_execute: true,
      can_cancel: false,
      cancellation_reason: '',
      version: 1,
      created_by: 'me',
      created_at: '2026-11-17T00:00:00Z',
      updated_at: '2026-11-17T00:00:00Z',
    },
  ],
  dependencies: [],
  progress: { progress_bps: 0, weighted_sum: '0', total_weight: '1', leaf_count: 1 },
})

beforeEach(() => {
  vi.resetAllMocks()
  vi.mocked(listProjects).mockImplementation(async (team) => [project(team)])
  vi.mocked(getTaskTree).mockImplementation(async (team) => tree(team))
  vi.mocked(listTeamMembers).mockImplementation(async (team) => [
    {
      user_id: 'me',
      username: 'me',
      display_name: `Member ${team}`,
      role: 'member',
      joined_at: '2026-11-17T00:00:00Z',
    },
  ])
})

async function setup(pendingTeams?: Promise<void>) {
  const pinia = createPinia()
  const teams = useTeamsStore(pinia)
  teams.currentTeamId = 'A'
  vi.spyOn(teams, 'loadTeams').mockReturnValue(pendingTeams ?? Promise.resolve())
  useAuthStore(pinia).user = {
    id: 'me',
    username: 'me',
    display_name: 'Me',
    system_root: false,
    platform_admin: false,
  }
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:pathMatch(.*)*', component: MyTasksView }],
  })
  await router.push('/my-tasks')
  const wrapper = mount(MyTasksView, { global: { plugins: [pinia, router] } })
  return { wrapper, teams }
}

describe('my tasks context isolation', () => {
  for (const stage of ['projects', 'tree'] as const) {
    it.each([null, 'B'])(
      `discards a delayed A ${stage} response after switching to %s`,
      async (team) => {
        const pendingProjects = deferred<Project[]>()
        const pendingTree = deferred<TaskTree>()
        if (stage === 'projects') {
          vi.mocked(listProjects).mockImplementation((id) =>
            id === 'A' ? pendingProjects.promise : Promise.resolve([project(id)]),
          )
        } else {
          vi.mocked(getTaskTree).mockImplementation((id) =>
            id === 'A' ? pendingTree.promise : Promise.resolve(tree(id)),
          )
        }
        const { wrapper, teams } = await setup()
        await flushPromises()
        const signal = vi.mocked(listProjects).mock.calls[0]![1]!
        expect(signal.aborted).toBe(false)
        if (stage === 'tree') {
          expect(vi.mocked(getTaskTree).mock.calls[0]![2]).toBe(signal)
          await wrapper.get('select').setValue('project-A')
        }
        teams.currentTeamId = team
        await flushPromises()
        expect(signal.aborted).toBe(true)
        expect((wrapper.get('select').element as HTMLSelectElement).value).toBe('')
        pendingProjects.resolve([project('A')])
        pendingTree.resolve(tree('A'))
        await flushPromises()
        expect(wrapper.text()).not.toContain('Task A')
        expect(wrapper.text()).not.toContain('Project A')
        expect(wrapper.text()).not.toContain('Member A')
        expect(wrapper.text()).not.toContain('暂时没有分配给你的任务')
        if (team) {
          expect(wrapper.text()).toContain('Task B')
          expect(wrapper.text()).toContain('Member B')
        } else {
          expect(wrapper.findAll('option')).toHaveLength(1)
          expect(wrapper.text()).toContain('还没有团队')
          expect(wrapper.text()).toContain('邀请')
          expect(wrapper.text()).not.toContain('创建或加入')
        }
        if (stage === 'projects') {
          expect(vi.mocked(getTaskTree).mock.calls.some(([id]) => id === 'A')).toBe(false)
        }
        wrapper.unmount()
      },
    )
  }

  it('clears loaded tasks, project filters and member names while the next team is pending', async () => {
    const { wrapper, teams } = await setup()
    await flushPromises()
    expect(wrapper.text()).toContain('Task A')
    expect(wrapper.text()).toContain('Member A')
    await wrapper.get('select').setValue('project-A')
    const pending = deferred<Project[]>()
    vi.mocked(listProjects).mockReturnValue(pending.promise)
    vi.mocked(listTeamMembers).mockRejectedValue(new ApiError('members unavailable', 500))
    teams.currentTeamId = 'B'
    await nextTick()
    expect(wrapper.findAll('option')).toHaveLength(1)
    expect((wrapper.get('select').element as HTMLSelectElement).value).toBe('')
    expect(wrapper.text()).not.toContain('Task A')
    expect(wrapper.text()).not.toContain('Member A')
    pending.resolve([project('B')])
    await flushPromises()
    expect(wrapper.text()).toContain('Task B')
    expect(wrapper.text()).toContain('负责人 未知')
    expect(wrapper.text()).not.toContain('Member A')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('aborts and invalidates pending requests on unmount', async () => {
    const pending = deferred<Project[]>()
    vi.mocked(listProjects).mockReturnValue(pending.promise)
    const { wrapper } = await setup()
    await flushPromises()
    const signal = vi.mocked(listProjects).mock.calls[0]![1]!
    wrapper.unmount()
    expect(signal.aborted).toBe(true)
    pending.resolve([project('A')])
    await flushPromises()
    expect(getTaskTree).not.toHaveBeenCalled()
  })

  it('does not start loading after unmount while team initialization is pending', async () => {
    const pending = deferred<void>()
    const { wrapper } = await setup(pending.promise)
    wrapper.unmount()
    pending.resolve()
    await flushPromises()
    expect(listProjects).not.toHaveBeenCalled()
  })
})

describe('my tasks loading errors', () => {
  it.each(['projects', 'tree'] as const)(
    'shows a retryable error instead of an empty success for %s failure',
    async (stage) => {
      if (stage === 'projects') {
        vi.mocked(listProjects).mockRejectedValueOnce(new ApiError('项目加载失败', 500))
      } else {
        vi.mocked(listProjects).mockResolvedValue([project('A'), project('B')])
        vi.mocked(getTaskTree).mockImplementation(async (_team, id) => {
          if (id === 'project-B') throw new ApiError('任务服务暂时不可用', 500)
          return { ...tree('A'), tasks: [] }
        })
      }
      const { wrapper } = await setup()
      await flushPromises()
      expect(wrapper.get('[role="alert"]').text()).toContain(
        stage === 'projects' ? '项目加载失败' : '任务服务暂时不可用',
      )
      expect(wrapper.text()).not.toContain('暂时没有分配给你的任务')
      expect(wrapper.text()).not.toContain('没有符合筛选条件的任务')
      expect(wrapper.find('table').exists()).toBe(false)
      vi.mocked(getTaskTree).mockImplementation(async (_team, id) =>
        tree(id === 'project-B' ? 'B' : 'A'),
      )
      await wrapper
        .findAll('button')
        .find((button) => button.text() === '重试')!
        .trigger('click')
      await flushPromises()
      expect(wrapper.find('[role="alert"]').exists()).toBe(false)
      expect(wrapper.text()).toContain('Task A')
      expect(listProjects).toHaveBeenCalledTimes(2)
      wrapper.unmount()
    },
  )

  it.each([403, 404])(
    'ignores an inaccessible project returning %s without hiding visible tasks',
    async (status) => {
      vi.mocked(listProjects).mockResolvedValue([project('A'), project('hidden')])
      vi.mocked(getTaskTree).mockImplementation(async (_team, id) => {
        if (id === 'project-hidden') throw new ApiError('不可见项目', status)
        return tree('A')
      })
      const { wrapper } = await setup()
      await flushPromises()
      expect(wrapper.find('[role="alert"]').exists()).toBe(false)
      expect(wrapper.text()).not.toContain('不可见项目')
      expect(wrapper.text()).toContain('Task A')
      expect(wrapper.findAll('tbody tr')).toHaveLength(1)
      wrapper.unmount()
    },
  )
})
