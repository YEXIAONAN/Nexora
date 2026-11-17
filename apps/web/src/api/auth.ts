import { requestJSON } from './client'
import type { components } from './schema'

export type User = components['schemas']['User']
export type LoginResult = components['schemas']['LoginEnvelope']['data']

export async function login(username: string, password: string): Promise<LoginResult> {
  const payload = await requestJSON<components['schemas']['LoginEnvelope']>('/api/v1/auth/login', {
    method: 'POST',
    body: { username, password } satisfies components['schemas']['LoginRequest'],
    skipSessionExpiry: true,
  })
  return payload.data
}

export async function fetchMe(signal?: AbortSignal): Promise<User> {
  const payload = await requestJSON<components['schemas']['UserEnvelope']>('/api/v1/auth/me', {
    signal,
  })
  return payload.data
}

export async function logout(): Promise<void> {
  await requestJSON<components['schemas']['RevokedEnvelope']>('/api/v1/auth/session', {
    method: 'DELETE',
    skipSessionExpiry: true,
  })
}
