import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import type { ProjectMember } from '@/api/projects'
import type { Task } from '@/api/tasks'
import TaskEditor from './TaskEditor.vue'
import TaskPanel from './TaskPanel.vue'

const member = (user_id: string, username: string, active: boolean): ProjectMember => ({
  user_id,
  username,
  display_name: username,
  active,
  role: 'participant',
})
function task(overrides: Partial<Task> = {}): Task {
  return {
    id: 'task',
    team_id: 'team',
    project_id: 'project',
    parent_id: null,
    depth: 1,
    title: '任务',
    description: '',
    owner_id: 'owner',
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
    created_by: 'owner',
    created_at: '2026-11-17T00:00:00Z',
    updated_at: '2026-11-17T00:00:00Z',
    ...overrides,
  }
}
const owner = member('owner', 'Owner', true)
const candidates = [
  task({ id: 'summary', title: '执行中的汇总任务', is_leaf: false, status: 'in_progress' }),
  task({ id: 'running', title: '执行中的叶子', status: 'in_progress' }),
  task({ id: 'pending', title: '待开始的叶子' }),
  task({ id: 'deep', depth: 3, is_leaf: false }),
]

describe('task editing after execution and membership changes', () => {
  it('offers an existing in-progress summary as a parent when creating a child', () => {
    const wrapper = mount(TaskEditor, {
      props: { tasks: candidates, members: [owner], defaultOwner: owner.user_id },
    })
    const select = wrapper.get('select[aria-label="父任务"]')
    expect(select.find('option[value="summary"]').exists()).toBe(true)
    expect(select.find('option[value="pending"]').exists()).toBe(true)
    expect(select.find('option[value="running"]').exists()).toBe(false)
    expect(select.find('option[value="deep"]').exists()).toBe(false)
  })
  it('offers an in-progress summary as a subtree move destination', () => {
    const wrapper = mount(TaskPanel, {
      props: {
        task: task(),
        tasks: candidates,
        dependencies: [],
        members: [owner],
        canManage: true,
        history: [],
        historyLoading: false,
      },
    })
    const select = wrapper.get('select[aria-label="移动到父任务"]')
    expect(select.find('option[value="summary"]').exists()).toBe(true)
    expect(select.find('option[value="running"]').exists()).toBe(false)
  })
  it('can explicitly remove an inactive collaborator when no selectable collaborators remain', async () => {
    const wrapper = mount(TaskEditor, {
      props: {
        task: task({ collaborator_ids: ['former'] }),
        tasks: [],
        members: [owner, member('former', 'Alice', false)],
        defaultOwner: owner.user_id,
      },
    })
    await wrapper.get('button[aria-label="移除失效协作者 Alice"]').trigger('click')
    await wrapper.get('input').setValue('修复后的标题')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('save')?.[0]?.[0]).toMatchObject({
      title: '修复后的标题',
      owner_id: 'owner',
      collaborator_ids: [],
    })
  })
  it('preserves active collaborators while explicitly removing inactive ones', async () => {
    const wrapper = mount(TaskEditor, {
      props: {
        task: task({ collaborator_ids: ['former', 'active'] }),
        tasks: [],
        members: [owner, member('former', 'Alice', false), member('active', 'Bob', true)],
        defaultOwner: owner.user_id,
      },
    })
    await wrapper.get('button[aria-label="移除失效协作者 Alice"]').trigger('click')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('save')?.[0]?.[0]).toMatchObject({ collaborator_ids: ['active'] })
  })
})
