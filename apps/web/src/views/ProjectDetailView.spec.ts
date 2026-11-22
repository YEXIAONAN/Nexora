import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { nextTick } from 'vue'
import { listTeamMembers } from '@/api/teams'
import { ApiError } from '@/api/client'
import * as projects from '@/api/projects'
import * as submissions from '@/api/submissions'
import * as tasks from '@/api/tasks'
import TaskDetail from '@/components/projects/TaskDetail.vue'
import TaskEditor from '@/components/projects/TaskEditor.vue'
import ProjectDetailView from './ProjectDetailView.vue'

vi.mock('@/api/submissions', () => ({
  listVersions: vi.fn().mockResolvedValue([]),
  attachmentURL: vi.fn(),
}))
vi.mock('@/api/projects', () => ({
  getProject: vi.fn(),
  listProjectMembers: vi.fn().mockResolvedValue([]),
  listMilestones: vi.fn().mockResolvedValue([]),
}))
vi.mock('@/api/tasks', async (original) => ({
  ...(await original<typeof import('@/api/tasks')>()),
  getTaskTree: vi.fn(),
  getSnapshots: vi.fn().mockResolvedValue([]),
  transitionTask: vi.fn(),
  createTask: vi.fn(),
  listTaskProgressReports: vi.fn().mockResolvedValue([]),
}))
vi.mock('@/api/teams', () => ({ listTeamMembers: vi.fn().mockResolvedValue([]) }))
function deferred<T>() {
  let resolve!: (v: T) => void, reject!: (e: Error) => void
  const promise = new Promise<T>((a, b) => {
    resolve = a
    reject = b
  })
  return { promise, resolve, reject }
}
const fixture = (team: string, id: string): projects.Project => ({
  id,
  team_id: team,
  name: `Project ${id}`,
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
  can_manage: true,
  can_handoff: true,
})
const task = (team: string, project: string): tasks.Task => ({
  id: `task-${project}`,
  team_id: team,
  project_id: project,
  parent_id: null,
  depth: 1,
  title: `Task ${project}`,
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
  can_cancel: true,
  cancellation_reason: '',
  version: 1,
  created_by: 'me',
  created_at: '2026-11-17T00:00:00Z',
  updated_at: '2026-11-17T00:00:00Z',
})
beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(submissions.listVersions).mockResolvedValue([])
  vi.mocked(projects.listProjectMembers).mockResolvedValue([])
  vi.mocked(projects.listMilestones).mockResolvedValue([])
  vi.mocked(tasks.getSnapshots).mockResolvedValue([])
  vi.mocked(listTeamMembers).mockResolvedValue([])
  vi.mocked(projects.listProjectMembers).mockResolvedValue([])
  vi.mocked(projects.listMilestones).mockResolvedValue([])
  vi.mocked(listTeamMembers).mockResolvedValue([])
  vi.mocked(tasks.getSnapshots).mockResolvedValue([])
  vi.mocked(tasks.listTaskProgressReports).mockResolvedValue([])
  vi.mocked(projects.getProject).mockImplementation(async (team, id) => fixture(team, id))
  vi.mocked(tasks.getTaskTree).mockImplementation(async (team, id) => ({
    tasks: [task(team, id)],
    dependencies: [],
    progress: { progress_bps: 0, weighted_sum: '0', total_weight: '1', leaf_count: 1 },
  }))
})
async function setup(path = '/teams/A/projects/a') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/teams/:teamId/projects/:projectId', component: ProjectDetailView },
      {
        path: '/teams/:teamId/projects/:projectId/contributions',
        component: { template: '<div />' },
      },
      { path: '/teams/:teamId/projects', name: 'projects', component: { template: '<div />' } },
    ],
  })
  await router.push(path)
  const wrapper = mount(ProjectDetailView, {
    global: {
      plugins: [createPinia(), router],
      stubs: { TaskDetail: true, TaskEditor: true, ProjectForm: true, teleport: true },
    },
  })
  return { router, wrapper }
}
describe('project context isolation', () => {
  it('discards an older team response after the new team is loaded', async () => {
    const pending = deferred<projects.Project>()
    vi.mocked(projects.getProject).mockImplementation((team, id) =>
      team === 'A' ? pending.promise : Promise.resolve(fixture(team, id)),
    )
    const { wrapper, router } = await setup()
    await router.push('/teams/B/projects/b')
    await flushPromises()
    expect(wrapper.text()).toContain('Project b')
    pending.resolve(fixture('A', 'a'))
    await flushPromises()
    expect(wrapper.text()).toContain('Project b')
    expect(wrapper.text()).not.toContain('Project a')
    wrapper.unmount()
  })
  it('does not show an old team error on the new project', async () => {
    const pending = deferred<projects.Project>()
    vi.mocked(projects.getProject).mockImplementation((team, id) =>
      team === 'A' ? pending.promise : Promise.resolve(fixture(team, id)),
    )
    const { wrapper, router } = await setup()
    await router.push('/teams/B/projects/b')
    await flushPromises()
    pending.reject(new ApiError('old denied', 404))
    await flushPromises()
    expect(wrapper.findAll('[role="alert"]').map((a) => a.text())).toEqual([])
    expect(wrapper.text()).toContain('Project b')
    wrapper.unmount()
  })
  it('removes old controls immediately when a new context is pending', async () => {
    const { wrapper, router } = await setup()
    await flushPromises()
    const panel = wrapper.findComponent(TaskDetail)
    const pending = deferred<projects.Project>()
    vi.mocked(projects.getProject).mockReturnValue(pending.promise)
    await router.push('/teams/B/projects/b')
    await nextTick()
    panel.vm.$emit('start', task('A', 'a'))
    await flushPromises()
    expect(tasks.transitionTask).not.toHaveBeenCalled()
    expect(wrapper.findComponent(TaskDetail).exists()).toBe(false)
    pending.resolve(fixture('B', 'b'))
    await flushPromises()
    wrapper.unmount()
  })
  it('binds a write to its original context and ignores its late error', async () => {
    const pending = deferred<tasks.Task>()
    vi.mocked(tasks.transitionTask).mockReturnValue(pending.promise)
    const { wrapper, router } = await setup()
    await flushPromises()
    wrapper.findComponent(TaskDetail).vm.$emit('start', task('A', 'a'))
    await flushPromises()
    expect(tasks.transitionTask).toHaveBeenCalledWith('A', 'a', task('A', 'a'), 'in_progress')
    await router.push('/teams/B/projects/b')
    await flushPromises()
    pending.reject(new ApiError('old write failed', 409))
    await flushPromises()
    expect(wrapper.text()).toContain('Project b')
    expect(wrapper.findAll('[role="alert"]').map((a) => a.text())).toEqual([])
    wrapper.unmount()
  })
  it('does not revive an earlier mutation after A → B → A', async () => {
    const pending = deferred<tasks.Task>()
    vi.mocked(tasks.transitionTask).mockReturnValue(pending.promise)
    const { wrapper, router } = await setup()
    await flushPromises()
    wrapper.findComponent(TaskDetail).vm.$emit('start', task('A', 'a'))
    await flushPromises()
    await router.push('/teams/B/projects/b')
    await flushPromises()
    await router.push('/teams/A/projects/a')
    await flushPromises()
    pending.reject(new ApiError('stale A error', 409))
    await flushPromises()
    expect(wrapper.findAll('[role="alert"]').map((a) => a.text())).toEqual([])
    wrapper.unmount()
  })

  it.each(['resolve', 'reject'] as const)(
    'ignores reports that %s after unmount',
    async (outcome) => {
      const pending = deferred<tasks.ProgressReport[]>()
      vi.mocked(tasks.listTaskProgressReports).mockReturnValue(pending.promise)
      const { wrapper, router } = await setup()
      await flushPromises()
      expect(tasks.listTaskProgressReports).toHaveBeenCalled()
      // Keep the route alive so only the view's unmount invalidation guards these responses.
      const host = mount({ template: '<div />' }, { global: { plugins: [router] } })
      const state = wrapper.vm as unknown as { reports: tasks.ProgressReport[]; error: string }
      const reportsBeforeUnmount = state.reports
      wrapper.unmount()
      if (outcome === 'resolve') pending.resolve([])
      else pending.reject(new ApiError('late report failure', 500))
      await flushPromises()
      expect(state.reports).toBe(reportsBeforeUnmount)
      expect(state.error).toBe('')
      host.unmount()
    },
  )
})

describe('project task query navigation', () => {
  it('consumes createTask once and keeps the new selection and success after saving and refreshing', async () => {
    const { wrapper, router } = await setup(
      '/teams/A/projects/a?createTask=1&task=task-a&filter=open&tag=one&tag=two#details',
    )
    await flushPromises()
    expect(wrapper.findComponent(TaskEditor).exists()).toBe(true)
    expect(router.currentRoute.value.query).toEqual({
      task: 'task-a',
      filter: 'open',
      tag: ['one', 'two'],
    })
    expect(router.currentRoute.value.hash).toBe('#details')
    expect(projects.getProject).toHaveBeenCalledTimes(1)
    const created = { ...task('A', 'a'), id: 'new-task', title: 'New task' }
    const input: tasks.CreateTask = { title: created.title, owner_id: 'me', weight: 1 }
    vi.mocked(tasks.createTask).mockResolvedValue(created)
    vi.mocked(tasks.getTaskTree).mockResolvedValue({
      tasks: [task('A', 'a'), created],
      dependencies: [],
      progress: { progress_bps: 0, weighted_sum: '0', total_weight: '2', leaf_count: 2 },
    })
    wrapper.findComponent(TaskEditor).vm.$emit('save', input)
    await flushPromises()
    expect(tasks.createTask).toHaveBeenCalledWith('A', 'a', input)
    expect(wrapper.findComponent(TaskEditor).exists()).toBe(false)
    expect(wrapper.findComponent(TaskDetail).props('task').id).toBe(created.id)
    expect(wrapper.get('[role="status"]').text()).toContain('操作已保存')
    await wrapper
      .findAll('button')
      .find((button) => button.text() === '刷新工作区')!
      .trigger('click')
    await flushPromises()
    expect(wrapper.findComponent(TaskEditor).exists()).toBe(false)
    expect(wrapper.findComponent(TaskDetail).props('task').id).toBe(created.id)
    expect(wrapper.get('[role="status"]').text()).toContain('操作已保存')
    expect(router.currentRoute.value.query.createTask).toBeUndefined()
    expect(projects.getProject).toHaveBeenCalledTimes(3)
    wrapper.unmount()
  })

  it('selects only visible query tasks on the same project without reloading it', async () => {
    const second = { ...task('A', 'a'), id: 'second-task', title: 'Second task' }
    vi.mocked(tasks.getTaskTree).mockResolvedValue({
      tasks: [task('A', 'a'), second],
      dependencies: [],
      progress: { progress_bps: 0, weighted_sum: '0', total_weight: '2', leaf_count: 2 },
    })
    const { wrapper, router } = await setup('/teams/A/projects/a?task=second-task')
    await flushPromises()
    expect(wrapper.findComponent(TaskDetail).props('task').id).toBe(second.id)
    await wrapper
      .findAll('button')
      .find((button) => button.text() === '项目概览')!
      .trigger('click')
    await router.push({ query: { task: 'task-a' } })
    await flushPromises()
    expect(wrapper.findComponent(TaskDetail).props('task').id).toBe('task-a')
    await router.push({ query: { task: second.id } })
    await flushPromises()
    expect(wrapper.findComponent(TaskDetail).props('task').id).toBe(second.id)
    for (const taskQuery of ['hidden-task', '', null, ['task-a', second.id], undefined]) {
      await router.push({ query: { task: taskQuery } })
      await flushPromises()
      expect(wrapper.findComponent(TaskDetail).props('task').id).toBe(second.id)
    }
    expect(projects.getProject).toHaveBeenCalledTimes(1)
    expect(tasks.getTaskTree).toHaveBeenCalledTimes(1)
    expect(tasks.listTaskProgressReports).not.toHaveBeenCalledWith('A', 'a', 'hidden-task')
    wrapper.unmount()
  })
})
