import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useAuthStore } from './auth'

const rootUser = {
  id: '00000000-0000-4000-8000-000000000001',
  username: 'waiting',
  display_name: 'Waiting',
  system_root: true,
  platform_admin: false,
}

function jsonResponse(payload: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => payload,
  } as Response
}

function stubFetch(handler: (input: string, init?: RequestInit) => Response | Promise<Response>) {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) =>
      Promise.resolve(handler(String(input), init)),
    ),
  )
}

describe('auth store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    window.localStorage.clear()
  })

  afterEach(() => vi.unstubAllGlobals())

  it('initialize resolves the current user from /auth/me', async () => {
    stubFetch((input) =>
      input.includes('/auth/me')
        ? jsonResponse({ data: rootUser })
        : jsonResponse({ error: { code: 'not_found' } }, 404),
    )
    const auth = useAuthStore()
    await auth.initialize()
    expect(auth.isAuthenticated).toBe(true)
    expect(auth.user?.username).toBe('waiting')
    expect(auth.displayName).toBe('Waiting')
    expect(auth.initialized).toBe(true)
  })

  it('initialize fails closed on 401 and does not retry once initialized', async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(jsonResponse({ error: { code: 'unauthorized' } }, 401)),
    )
    vi.stubGlobal('fetch', fetchMock)
    const auth = useAuthStore()
    await auth.initialize()
    await auth.initialize()
    expect(auth.isAuthenticated).toBe(false)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('initialize fails closed when the API is unreachable', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.reject(new TypeError('network down'))),
    )
    const auth = useAuthStore()
    await auth.initialize()
    expect(auth.isAuthenticated).toBe(false)
    expect(auth.initialized).toBe(true)
  })

  it('login stores the user and session expiry from the real envelope', async () => {
    stubFetch((input, init) => {
      expect(input).toContain('/api/v1/auth/login')
      expect(init?.method).toBe('POST')
      expect((init?.headers as Record<string, string>)['X-Nexora-CSRF']).toBe('1')
      return jsonResponse({
        data: { user: rootUser, expires_at: '2026-11-18T00:00:00Z' },
      })
    })
    const auth = useAuthStore()
    await auth.login('waiting', 'correct-password-1')
    expect(auth.isAuthenticated).toBe(true)
    expect(auth.expiresAt).toBe('2026-11-18T00:00:00Z')
  })

  it('login propagates API errors without authenticating', async () => {
    stubFetch(() =>
      jsonResponse({ error: { code: 'unauthorized', message: '用户名或密码错误' } }, 401),
    )
    const auth = useAuthStore()
    await expect(auth.login('waiting', 'wrong-password-1')).rejects.toMatchObject({ status: 401 })
    expect(auth.isAuthenticated).toBe(false)
  })

  it('logout clears the local identity even if revocation fails', async () => {
    stubFetch((input) =>
      input.includes('/auth/me')
        ? jsonResponse({ data: rootUser })
        : jsonResponse({ error: { code: 'internal' } }, 500),
    )
    const auth = useAuthStore()
    await auth.initialize()
    await auth.logout()
    expect(auth.isAuthenticated).toBe(false)
    expect(auth.expiresAt).toBeNull()
  })

  it('expire clears identity without any network call', async () => {
    stubFetch(() => jsonResponse({ data: rootUser }))
    const auth = useAuthStore()
    await auth.initialize()
    auth.expire()
    expect(auth.isAuthenticated).toBe(false)
    expect(auth.user).toBeNull()
  })
})
