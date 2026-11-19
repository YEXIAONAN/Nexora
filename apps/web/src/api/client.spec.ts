import { afterEach, describe, expect, it, vi } from 'vitest'

import { ApiError, requestJSON, SESSION_EXPIRED_EVENT } from './client'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

function jsonResponse(payload: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => payload,
  } as Response
}

describe('request cancellation', () => {
  it('does not expire the current session when a canceled old response body rejects', async () => {
    const caller = new AbortController()
    let rejectBody!: (reason: unknown) => void
    const body = new Promise<unknown>((_resolve, reject) => {
      rejectBody = reject
    })
    const parse = vi.fn(() => body)
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ ok: false, status: 401, json: parse })),
    )
    const listener = vi.fn()
    window.addEventListener(SESSION_EXPIRED_EVENT, listener)
    try {
      const pending = requestJSON('/api/v1/teams/old-team/contributions', { signal: caller.signal })
      await Promise.resolve()
      expect(parse).toHaveBeenCalledTimes(1)
      caller.abort()
      rejectBody(new DOMException('body canceled after context switch', 'AbortError'))
      await expect(pending).rejects.toBeInstanceOf(ApiError)
      expect(listener).not.toHaveBeenCalled()
    } finally {
      window.removeEventListener(SESSION_EXPIRED_EVENT, listener)
    }
  })

  it('forwards a signal that was already aborted before the request', async () => {
    vi.useFakeTimers()
    const caller = new AbortController()
    caller.abort()
    vi.stubGlobal(
      'fetch',
      vi.fn((_input, init: RequestInit) => {
        expect(init.signal?.aborted).toBe(true)
        return Promise.reject(new DOMException('synthetic cancellation', 'AbortError'))
      }),
    )
    await expect(requestJSON('/readyz', { signal: caller.signal })).rejects.toThrow()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('forwards cancellation during a pending request and clears its timeout', async () => {
    vi.useFakeTimers()
    const caller = new AbortController()
    vi.stubGlobal(
      'fetch',
      vi.fn(
        (_input, init: RequestInit) =>
          new Promise((_resolve, reject) => {
            init.signal?.addEventListener('abort', () =>
              reject(new DOMException('synthetic cancellation', 'AbortError')),
            )
          }),
      ),
    )
    const pending = requestJSON('/readyz', { signal: caller.signal })
    caller.abort()
    await expect(pending).rejects.toThrow()
    expect(vi.getTimerCount()).toBe(0)
  })
})

describe('mutating requests', () => {
  it('sends the CSRF header and JSON body on POST', async () => {
    const fetchMock = vi.fn((_input: RequestInfo | URL, _init?: RequestInit) =>
      Promise.resolve(jsonResponse({ data: { ok: true } })),
    )
    vi.stubGlobal('fetch', fetchMock)

    await requestJSON('/api/v1/teams', { method: 'POST', body: { name: 'ZERO_R' } })

    const init = fetchMock.mock.calls[0]?.[1]
    expect(init?.method).toBe('POST')
    expect((init?.headers as Record<string, string>)['X-Nexora-CSRF']).toBe('1')
    expect((init?.headers as Record<string, string>)['Content-Type']).toBe('application/json')
    expect(init?.body).toBe(JSON.stringify({ name: 'ZERO_R' }))
    expect(init?.credentials).toBe('same-origin')
  })

  it('sends multipart bytes with CSRF and the browser-generated boundary', async () => {
    const fetchMock = vi.fn((_input: RequestInfo | URL, _init?: RequestInit) =>
      Promise.resolve(jsonResponse({ data: { id: 'file' } })),
    )
    vi.stubGlobal('fetch', fetchMock)
    const body = new FormData()
    body.append('file', new File(['evidence'], 'result.txt'))
    await requestJSON('/api/v1/attachments', { method: 'POST', body })
    const init = fetchMock.mock.calls[0]?.[1]
    expect(init?.body).toBe(body)
    expect((init?.headers as Record<string, string>)['Content-Type']).toBeUndefined()
    expect((init?.headers as Record<string, string>)['X-Nexora-CSRF']).toBe('1')
  })

  it('sends the CSRF header on DELETE without a body', async () => {
    const fetchMock = vi.fn((_input: RequestInfo | URL, _init?: RequestInit) =>
      Promise.resolve(jsonResponse({ data: { revoked: true } })),
    )
    vi.stubGlobal('fetch', fetchMock)

    await requestJSON('/api/v1/auth/session', { method: 'DELETE' })

    const init = fetchMock.mock.calls[0]?.[1]
    expect((init?.headers as Record<string, string>)['X-Nexora-CSRF']).toBe('1')
    expect(init?.body).toBeUndefined()
  })

  it('does not send the CSRF header on GET', async () => {
    const fetchMock = vi.fn((_input: RequestInfo | URL, _init?: RequestInit) =>
      Promise.resolve(jsonResponse({ data: [] })),
    )
    vi.stubGlobal('fetch', fetchMock)

    await requestJSON('/api/v1/teams')

    const init = fetchMock.mock.calls[0]?.[1]
    expect((init?.headers as Record<string, string>)['X-Nexora-CSRF']).toBeUndefined()
  })
})

describe('session expiry signalling', () => {
  it('dispatches the session-expired event on 401', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          jsonResponse({ error: { code: 'unauthorized', message: 'no session' } }, 401),
        ),
      ),
    )
    const listener = vi.fn()
    window.addEventListener(SESSION_EXPIRED_EVENT, listener)

    await expect(requestJSON('/api/v1/teams')).rejects.toMatchObject({ status: 401 })
    expect(listener).toHaveBeenCalledTimes(1)
    window.removeEventListener(SESSION_EXPIRED_EVENT, listener)
  })

  it('does not dispatch the event when the caller opts out (login failures)', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          jsonResponse({ error: { code: 'unauthorized', message: 'bad credentials' } }, 401),
        ),
      ),
    )
    const listener = vi.fn()
    window.addEventListener(SESSION_EXPIRED_EVENT, listener)

    await expect(
      requestJSON('/api/v1/auth/login', { method: 'POST', body: {}, skipSessionExpiry: true }),
    ).rejects.toMatchObject({ status: 401 })
    expect(listener).not.toHaveBeenCalled()
    window.removeEventListener(SESSION_EXPIRED_EVENT, listener)
  })

  it('exposes field errors from the error envelope', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          jsonResponse(
            {
              error: {
                code: 'validation_failed',
                message: 'invalid input',
                fields: { username: 'too short' },
                request_id: 'req-1',
              },
            },
            422,
          ),
        ),
      ),
    )

    const error = await requestJSON('/api/v1/auth/login', { method: 'POST', body: {} }).catch(
      (reason: unknown) => reason,
    )
    expect(error).toBeInstanceOf(ApiError)
    expect((error as ApiError).fields).toEqual({ username: 'too short' })
    expect((error as ApiError).requestId).toBe('req-1')
  })
})
