import { afterEach, describe, expect, it, vi } from 'vitest'

import { requestJSON } from './client'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('request cancellation', () => {
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
