import { requestJSON } from './client'
import { projectPath } from './projects'
import type { components } from './schema'

type S = components['schemas']
export type Task = S['Task']
export type TaskTree = S['TaskTree']
export type Dependency = S['Dependency']
export type Snapshot = S['ProgressSnapshot']
export type TaskHistory = S['TaskHistory']
export type CreateTask = S['CreateTaskRequest']
export type EditTask = S['EditTaskRequest']
const path = (team: string, project: string, task?: string) =>
  `${projectPath(team, project)}/tasks${task ? `/${encodeURIComponent(task)}` : ''}`
export async function getTaskTree(
  team: string,
  project: string,
  signal?: AbortSignal,
): Promise<TaskTree> {
  return (await requestJSON<S['TaskTreeEnvelope']>(path(team, project), { signal })).data
}
export async function createTask(team: string, project: string, body: CreateTask): Promise<Task> {
  return (await requestJSON<S['TaskEnvelope']>(path(team, project), { method: 'POST', body })).data
}
export async function editTask(
  team: string,
  project: string,
  task: string,
  body: EditTask,
): Promise<Task> {
  return (await requestJSON<S['TaskEnvelope']>(path(team, project, task), { method: 'PUT', body }))
    .data
}
export async function transitionTask(
  team: string,
  project: string,
  task: Task,
  status: 'in_progress' | 'cancelled',
  reason = '',
): Promise<Task> {
  return (
    await requestJSON<S['TaskEnvelope']>(`${path(team, project, task.id)}/transition`, {
      method: 'POST',
      body: { status, reason, expected_version: task.version },
    })
  ).data
}
export async function recordTaskProgress(
  team: string,
  project: string,
  task: Task,
  actual_progress: number,
): Promise<Task> {
  return (
    await requestJSON<S['TaskEnvelope']>(`${path(team, project, task.id)}/progress`, {
      method: 'PUT',
      body: { actual_progress, expected_version: task.version },
    })
  ).data
}
export async function changeTaskWeight(
  team: string,
  project: string,
  task: Task,
  weight: number,
  reason: string,
): Promise<Task> {
  return (
    await requestJSON<S['TaskEnvelope']>(`${path(team, project, task.id)}/weight`, {
      method: 'PUT',
      body: { weight, reason, expected_version: task.version },
    })
  ).data
}
export async function moveTask(
  team: string,
  project: string,
  task: Task,
  parent_id: string | null,
): Promise<Task> {
  return (
    await requestJSON<S['TaskEnvelope']>(`${path(team, project, task.id)}/move`, {
      method: 'POST',
      body: { parent_id, expected_version: task.version },
    })
  ).data
}
export async function addDependency(
  team: string,
  project: string,
  task: Task,
  predecessor_id: string,
): Promise<Dependency> {
  return (
    await requestJSON<S['DependencyEnvelope']>(`${path(team, project, task.id)}/dependencies`, {
      method: 'POST',
      body: { predecessor_id, expected_version: task.version },
    })
  ).data
}
export async function ignoreDependency(
  team: string,
  project: string,
  task: Task,
  dependency: string,
  reason: string,
): Promise<Dependency> {
  return (
    await requestJSON<S['DependencyEnvelope']>(
      `${path(team, project, task.id)}/dependencies/${encodeURIComponent(dependency)}/ignore`,
      { method: 'POST', body: { reason, expected_version: task.version } },
    )
  ).data
}
export async function getTaskHistory(
  team: string,
  project: string,
  task: string,
): Promise<TaskHistory[]> {
  return (await requestJSON<S['TaskHistoriesEnvelope']>(`${path(team, project, task)}/history`))
    .data
}
export async function getSnapshots(team: string, project: string): Promise<Snapshot[]> {
  return (
    await requestJSON<S['ProgressSnapshotsEnvelope']>(
      `${projectPath(team, project)}/progress/history`,
    )
  ).data
}
export const taskStatusLabel: Record<Task['status'], string> = {
  pending: '待开始',
  in_progress: '进行中',
  awaiting_review: '待验收',
  completed: '已完成',
  rejected: '已驳回',
  cancelled: '已取消',
}
export function percent(bps: number): string {
  return `${(bps / 100).toFixed(2)}%`
}

// This is only an option filter. The server still checks execution history,
// dependencies, qualification and the resulting subtree depth.
export function canSelectParent(task: Task): boolean {
  return task.depth < 3 && (!task.is_leaf || task.status === 'pending')
}
