import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useSystemStore } from './system'

const meta = {
  name: 'Nexora',
  version: '0.1.0',
  commit: 'abc1234',
  build_time: '2026-10-29T12:00:00Z',
  environment: 'test',
  instance_id: '00000000-0000-4000-8000-000000000001',
  server_time: '2026-10-29T12:00:01Z',
}

const readiness = {
  status: 'ready',
  checks: {
    database: { status: 'up' },
    schema: { status: 'up', detail: 'version 1' },
  },
  checked_at: '2026-10-29T12:00:01Z',
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

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((onResolve, onReject) => {
    resolve = onResolve
    reject = onReject
  })
  return { promise, resolve, reject }
}

describe('system store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.restoreAllMocks()
  })

  afterEach(() => vi.unstubAllGlobals())

  it('starts with an unknown status', () => {
    const store = useSystemStore()
    expect(store.status).toBe('unknown')
    expect(store.statusLabel).toBe('未知')
    expect(store.isReady).toBe(false)
  })

  it('loads meta and readiness from the API', async () => {
    stubFetch((input) =>
      input.includes('/api/v1/meta') ? jsonResponse({ data: meta }) : jsonResponse(readiness),
    )

    const store = useSystemStore()
    await store.refresh()

    expect(store.meta?.version).toBe('0.1.0')
    expect(store.readiness?.status).toBe('ready')
    expect(store.isReady).toBe(true)
    expect(store.error).toBeNull()
    expect(store.loading).toBe(false)
    expect(store.status).toBe('ready')
  })

  it('accepts a 503 readiness response as a real check result', async () => {
    const unavailable = {
      ...readiness,
      status: 'unavailable',
      checks: {
        database: { status: 'down', detail: 'connection failed' },
        schema: { status: 'down', detail: 'not migrated' },
      },
    }
    stubFetch((input) =>
      input.includes('/api/v1/meta')
        ? jsonResponse({ data: meta })
        : jsonResponse(unavailable, 503),
    )

    const store = useSystemStore()
    await store.refresh()

    expect(store.readiness?.status).toBe('unavailable')
    expect(store.isReady).toBe(false)
    expect(store.error).toBeNull()
    expect(store.status).toBe('unavailable')
  })

  it('reports a network failure without inventing data', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.reject(new TypeError('Failed to fetch'))),
    )

    const store = useSystemStore()
    await store.refresh()

    expect(store.meta).toBeNull()
    expect(store.readiness).toBeNull()
    expect(store.error).toBe('无法连接到 Nexora API')
    expect(store.loading).toBe(false)
    expect(store.status).toBe('error')
    expect(store.isReady).toBe(false)
  })

  it('clears a previous successful result while loading and after a failure', async () => {
    stubFetch((input) =>
      input.includes('/meta') ? jsonResponse({ data: meta }) : jsonResponse(readiness),
    )
    const store = useSystemStore()
    await store.refresh()
    const pending = deferred<Response>()
    stubFetch(() => pending.promise)
    const refresh = store.refresh()
    expect(store.status).toBe('loading')
    expect(store.meta).toBeNull()
    expect(store.readiness).toBeNull()
    expect(store.lastCheckedAt).toBeNull()
    expect(store.isReady).toBe(false)
    pending.reject(new TypeError('synthetic network failure'))
    await refresh
    expect(store.status).toBe('error')
    expect(store.readiness).toBeNull()
    expect(store.meta).toBeNull()
    expect(store.isReady).toBe(false)
    expect(store.lastCheckedAt).toBeInstanceOf(Date)
  })

  it('recovers from a failed attempt without retaining errors', async () => {
    stubFetch(() => Promise.reject(new TypeError('synthetic network failure')))
    const store = useSystemStore()
    await store.refresh()
    stubFetch((input) =>
      input.includes('/meta') ? jsonResponse({ data: meta }) : jsonResponse(readiness),
    )
    await store.refresh()
    expect(store.status).toBe('ready')
    expect(store.error).toBeNull()
    expect(store.metaError).toBeNull()
    expect(store.readinessError).toBeNull()
  })

  it('keeps only current metadata when readiness fails', async () => {
    stubFetch((input) =>
      input.includes('/meta')
        ? jsonResponse({ data: meta })
        : Promise.reject(new TypeError('synthetic readiness failure')),
    )
    const store = useSystemStore()
    await store.refresh()
    expect(store.meta).toEqual(meta)
    expect(store.apiState).toBe('up')
    expect(store.readiness).toBeNull()
    expect(store.readinessError).not.toBeNull()
    expect(store.status).toBe('error')
    expect(store.isReady).toBe(false)
  })

  it('does not call the whole instance ready when metadata fails', async () => {
    stubFetch((input) =>
      input.includes('/meta')
        ? Promise.reject(new TypeError('synthetic meta failure'))
        : jsonResponse(readiness),
    )
    const store = useSystemStore()
    await store.refresh()
    expect(store.readiness?.status).toBe('ready')
    expect(store.meta).toBeNull()
    expect(store.apiState).toBe('down')
    expect(store.status).toBe('error')
    expect(store.isReady).toBe(false)
  })

  it('aborts older attempts and ignores late successful responses', async () => {
    const oldMeta = deferred<Response>()
    const oldReadiness = deferred<Response>()
    const signals: AbortSignal[] = []
    stubFetch((input, init) => {
      signals.push(init?.signal as AbortSignal)
      return input.includes('/meta') ? oldMeta.promise : oldReadiness.promise
    })
    const store = useSystemStore()
    const older = store.refresh()
    const currentMeta = { ...meta, commit: 'latest-response' }
    stubFetch((input) =>
      input.includes('/meta')
        ? jsonResponse({ data: currentMeta })
        : jsonResponse({ ...readiness, status: 'unavailable' }, 503),
    )
    await store.refresh()
    expect(signals.every((signal) => signal.aborted)).toBe(true)
    oldMeta.resolve(jsonResponse({ data: meta }))
    oldReadiness.resolve(jsonResponse(readiness))
    await older
    expect(store.meta?.commit).toBe('latest-response')
    expect(store.status).toBe('unavailable')
    expect(store.isReady).toBe(false)
  })

  it('does not let an older completion stop a newer pending refresh', async () => {
    const old = deferred<Response>()
    const currentMeta = deferred<Response>()
    const currentReadiness = deferred<Response>()
    stubFetch(() => old.promise)
    const store = useSystemStore()
    const older = store.refresh()
    stubFetch((input) => (input.includes('/meta') ? currentMeta.promise : currentReadiness.promise))
    const current = store.refresh()
    old.reject(new TypeError('late old failure'))
    await older
    expect(store.status).toBe('loading')
    expect(store.error).toBeNull()
    currentReadiness.resolve(jsonResponse(readiness))
    currentMeta.resolve(jsonResponse({ data: meta }))
    await current
    expect(store.status).toBe('ready')
  })

  it('ignores an older failure after the newest success', async () => {
    const old = deferred<Response>()
    stubFetch(() => old.promise)
    const store = useSystemStore()
    const older = store.refresh()
    stubFetch((input) =>
      input.includes('/meta') ? jsonResponse({ data: meta }) : jsonResponse(readiness),
    )
    await store.refresh()
    old.reject(new TypeError('late old failure'))
    await older
    expect(store.status).toBe('ready')
    expect(store.error).toBeNull()
  })
})
