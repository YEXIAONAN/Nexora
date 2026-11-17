import { requestJSON } from './client'
import type { components } from './schema'

type S = components['schemas']
export type Project = S['Project']
export type ProjectMember = S['ProjectMember']
export type Milestone = S['Milestone']
export type CreateProject = S['CreateProjectRequest']
export type EditProject = S['EditProjectRequest']
export type MilestoneInput = S['CreateMilestoneRequest']
export const projectBase = (team: string) => `/api/v1/teams/${encodeURIComponent(team)}/projects`
export const projectPath = (team: string, project: string) =>
  `${projectBase(team)}/${encodeURIComponent(project)}`
export async function listProjects(team: string): Promise<Project[]> {
  return (await requestJSON<S['ProjectsEnvelope']>(projectBase(team))).data
}
export async function createProject(team: string, body: CreateProject): Promise<Project> {
  return (await requestJSON<S['ProjectEnvelope']>(projectBase(team), { method: 'POST', body })).data
}
export async function getProject(team: string, project: string): Promise<Project> {
  return (await requestJSON<S['ProjectEnvelope']>(projectPath(team, project))).data
}
export async function editProject(
  team: string,
  project: string,
  body: EditProject,
): Promise<Project> {
  return (
    await requestJSON<S['ProjectEnvelope']>(projectPath(team, project), { method: 'PATCH', body })
  ).data
}
export async function archiveProject(
  team: string,
  project: string,
  expected_version: number,
): Promise<Project> {
  return (
    await requestJSON<S['ProjectEnvelope']>(`${projectPath(team, project)}/archive`, {
      method: 'POST',
      body: { expected_version },
    })
  ).data
}
export async function handoffProject(
  team: string,
  project: string,
  body: S['HandoffRequest'],
): Promise<Project> {
  return (
    await requestJSON<S['ProjectEnvelope']>(`${projectPath(team, project)}/handoff`, {
      method: 'POST',
      body,
    })
  ).data
}
export async function listProjectMembers(team: string, project: string): Promise<ProjectMember[]> {
  return (await requestJSON<S['ProjectMembersEnvelope']>(`${projectPath(team, project)}/members`))
    .data
}
export async function addProjectMember(
  team: string,
  project: string,
  user_id: string,
): Promise<ProjectMember[]> {
  return (
    await requestJSON<S['ProjectMembersEnvelope']>(`${projectPath(team, project)}/members`, {
      method: 'POST',
      body: { user_id },
    })
  ).data
}
export async function removeProjectMember(
  team: string,
  project: string,
  user: string,
): Promise<ProjectMember[]> {
  return (
    await requestJSON<S['ProjectMembersEnvelope']>(
      `${projectPath(team, project)}/members/${encodeURIComponent(user)}`,
      { method: 'DELETE' },
    )
  ).data
}
export async function listMilestones(team: string, project: string): Promise<Milestone[]> {
  return (await requestJSON<S['MilestonesEnvelope']>(`${projectPath(team, project)}/milestones`))
    .data
}
export async function saveMilestone(
  team: string,
  project: string,
  body: MilestoneInput,
  existing?: Milestone,
): Promise<Milestone> {
  return (
    await requestJSON<S['MilestoneEnvelope']>(
      `${projectPath(team, project)}/milestones${existing ? `/${existing.id}` : ''}`,
      {
        method: existing ? 'PATCH' : 'POST',
        body: existing ? { ...body, expected_version: existing.version } : body,
      },
    )
  ).data
}
