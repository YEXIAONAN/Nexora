import { requestJSON } from './client'
import type { components } from './schema'

export type User = components['schemas']['User'] & {
  email?: string | null
  is_active?: boolean
  created_at?: string
}
export type LoginResult = {
  user: User
  expires_at: string
}

export interface AdminUserPage {
  items: User[]
  total: number
  limit: number
  offset: number
}

export interface RegisterInput {
  username: string
  display_name: string
  email: string
  password: string
}

export type CreateUserInput = RegisterInput

export interface UpdateUserInput {
  display_name?: string
  email?: string
  is_active?: boolean
}

export async function login(username: string, password: string): Promise<LoginResult> {
  const payload = await requestJSON<components['schemas']['LoginEnvelope']>('/api/v1/auth/login', {
    method: 'POST',
    body: { username, password } satisfies components['schemas']['LoginRequest'],
    skipSessionExpiry: true,
  })
  return payload.data as LoginResult
}

export async function register(input: RegisterInput): Promise<User> {
  const payload = await requestJSON<{ data: User }>('/api/v1/auth/register', {
    method: 'POST',
    body: input,
    skipSessionExpiry: true,
  })
  return payload.data
}

export async function fetchMe(signal?: AbortSignal): Promise<User> {
  const payload = await requestJSON<components['schemas']['UserEnvelope']>('/api/v1/auth/me', {
    signal,
  })
  return payload.data as User
}

export async function updateProfile(displayName: string, email: string): Promise<User> {
  const payload = await requestJSON<{ data: User }>('/api/v1/auth/me', {
    method: 'PATCH',
    body: { display_name: displayName, email },
  })
  return payload.data
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  await requestJSON('/api/v1/auth/me/password', {
    method: 'POST',
    body: { current_password: currentPassword, new_password: newPassword },
  })
}

export async function logout(): Promise<void> {
  await requestJSON<components['schemas']['RevokedEnvelope']>('/api/v1/auth/session', {
    method: 'DELETE',
    skipSessionExpiry: true,
  })
}

export async function listUsers(
  query: string,
  limit = 25,
  offset = 0,
  signal?: AbortSignal,
): Promise<AdminUserPage> {
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) })
  if (query.trim()) params.set('q', query.trim())
  const payload = await requestJSON<{ data: AdminUserPage }>(`/api/v1/admin/users?${params}`, {
    signal,
  })
  return payload.data
}

export async function createUser(input: CreateUserInput): Promise<User> {
  const payload = await requestJSON<{ data: User }>('/api/v1/admin/users', {
    method: 'POST',
    body: input,
  })
  return payload.data
}

export async function updateUser(userId: string, input: UpdateUserInput): Promise<User> {
  const payload = await requestJSON<{ data: User }>(`/api/v1/admin/users/${userId}`, {
    method: 'PATCH',
    body: input,
  })
  return payload.data
}

export async function grantPlatformAdmin(userId: string): Promise<void> {
  await requestJSON('/api/v1/admin/platform-roles', {
    method: 'POST',
    body: { user_id: userId },
  })
}

export async function revokePlatformAdmin(userId: string): Promise<void> {
  await requestJSON(`/api/v1/admin/platform-roles/${userId}`, { method: 'DELETE' })
}

export async function revokeUserSessions(userId: string): Promise<number> {
  const payload = await requestJSON<{ data: { revoked_sessions: number } }>(
    `/api/v1/admin/users/${userId}/sessions`,
    { method: 'DELETE' },
  )
  return payload.data.revoked_sessions
}
