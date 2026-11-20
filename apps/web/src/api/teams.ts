import { requestJSON } from './client'
import type { components } from './schema'

export type Team = components['schemas']['Team']
export type TeamRole = NonNullable<Team['my_role']>
export type TeamMember = components['schemas']['TeamMember']
export type TeamPermissions = components['schemas']['TeamPermissions']
export type TeamInvitation = components['schemas']['TeamInvitation']
export type InvitationRole = components['schemas']['CreateInvitationRequest']['role']

export async function listMyTeams(signal?: AbortSignal): Promise<Team[]> {
  const payload = await requestJSON<components['schemas']['TeamsEnvelope']>('/api/v1/teams', {
    signal,
  })
  return payload.data
}

export async function createTeam(
  name: string,
  description?: string,
  ownerId?: string,
): Promise<Team> {
  const payload = await requestJSON<components['schemas']['TeamEnvelope']>('/api/v1/teams', {
    method: 'POST',
    body: {
      name,
      description,
      ...(ownerId ? { owner_id: ownerId } : {}),
    } satisfies components['schemas']['CreateTeamRequest'],
  })
  return payload.data
}

export async function getTeam(teamId: string, signal?: AbortSignal): Promise<Team> {
  const payload = await requestJSON<components['schemas']['TeamEnvelope']>(
    `/api/v1/teams/${teamId}`,
    { signal },
  )
  return payload.data
}

export async function updateTeam(teamId: string, name: string, description: string): Promise<Team> {
  const payload = await requestJSON<components['schemas']['TeamEnvelope']>(
    `/api/v1/teams/${teamId}`,
    { method: 'PATCH', body: { name, description } },
  )
  return payload.data
}

export async function getTeamPermissions(
  teamId: string,
  signal?: AbortSignal,
): Promise<TeamPermissions> {
  const payload = await requestJSON<components['schemas']['TeamPermissionsEnvelope']>(
    `/api/v1/teams/${teamId}/permissions`,
    { signal },
  )
  return payload.data
}

export async function listTeamMembers(teamId: string, signal?: AbortSignal): Promise<TeamMember[]> {
  const payload = await requestJSON<components['schemas']['TeamMembersEnvelope']>(
    `/api/v1/teams/${teamId}/members`,
    { signal },
  )
  return payload.data
}

export async function changeMemberRole(
  teamId: string,
  userId: string,
  role: TeamRole,
): Promise<void> {
  await requestJSON<components['schemas']['RoleChangedEnvelope']>(
    `/api/v1/teams/${teamId}/members/${userId}`,
    {
      method: 'PATCH',
      body: { role } satisfies components['schemas']['ChangeMemberRoleRequest'],
    },
  )
}

export async function removeTeamMember(teamId: string, userId: string): Promise<void> {
  await requestJSON<components['schemas']['RemovedEnvelope']>(
    `/api/v1/teams/${teamId}/members/${userId}`,
    { method: 'DELETE' },
  )
}

export async function listTeamInvitations(
  teamId: string,
  signal?: AbortSignal,
): Promise<TeamInvitation[]> {
  const payload = await requestJSON<components['schemas']['TeamInvitationsEnvelope']>(
    `/api/v1/teams/${teamId}/invitations`,
    { signal },
  )
  return payload.data
}

export async function createInvitation(
  teamId: string,
  username: string,
  role: InvitationRole,
  expiresInHours?: number,
): Promise<TeamInvitation> {
  const payload = await requestJSON<components['schemas']['TeamInvitationEnvelope']>(
    `/api/v1/teams/${teamId}/invitations`,
    {
      method: 'POST',
      body: {
        username,
        role,
        expires_in_hours: expiresInHours,
      } satisfies components['schemas']['CreateInvitationRequest'],
    },
  )
  return payload.data
}

export async function cancelInvitation(teamId: string, invitationId: string): Promise<void> {
  await requestJSON<components['schemas']['RevokedEnvelope']>(
    `/api/v1/teams/${teamId}/invitations/${invitationId}`,
    { method: 'DELETE' },
  )
}

export async function listMyInvitations(signal?: AbortSignal): Promise<TeamInvitation[]> {
  const payload = await requestJSON<components['schemas']['TeamInvitationsEnvelope']>(
    '/api/v1/invitations',
    { signal },
  )
  return payload.data
}

export async function acceptInvitation(invitationId: string): Promise<TeamInvitation> {
  const payload = await requestJSON<components['schemas']['TeamInvitationEnvelope']>(
    `/api/v1/invitations/${invitationId}/accept`,
    { method: 'POST' },
  )
  return payload.data
}

export async function declineInvitation(invitationId: string): Promise<void> {
  await requestJSON<components['schemas']['DeclinedEnvelope']>(
    `/api/v1/invitations/${invitationId}/decline`,
    { method: 'POST' },
  )
}
