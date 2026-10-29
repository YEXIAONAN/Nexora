import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

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

function stubFetch(handler: (input: string) => Response | Promise<Response>) {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL) => Promise.resolve(handler(String(input)))),
  )
}

describe('system store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.restoreAllMocks()
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
  })
})
