import { createPinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { useSystemStore } from '@/stores/system'
import OverviewView from './OverviewView.vue'
import StatusView from './StatusView.vue'

function serveChecks() {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL) => ({
      ok: true,
      status: 200,
      json: async () =>
        String(input).includes('/meta')
          ? {
              data: {
                name: 'Nexora',
                version: 'test',
                environment: 'test',
                commit: 'test',
                instance_id: null,
                build_time: 'unknown',
                server_time: '2026-11-06T00:00:00Z',
              },
            }
          : {
              status: 'ready',
              checks: { database: { status: 'up' }, schema: { status: 'up', detail: 'version 1' } },
              checked_at: '2026-11-06T00:00:00Z',
            },
    })),
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('diagnostic views', () => {
  it('does not display overall ready when metadata fails but readiness succeeds', async () => {
    serveChecks()
    const realChecks = globalThis.fetch
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) =>
        String(input).includes('/meta')
          ? Promise.reject(new TypeError('synthetic metadata failure'))
          : realChecks(input),
      ),
    )
    const wrapper = mount(OverviewView, { global: { plugins: [createPinia()] } })
    try {
      await flushPromises()
      const overall = wrapper.findAll('article')[2]!
      expect(overall.text()).toContain('检查失败')
      expect(overall.text()).toContain('error')
      expect(overall.text()).not.toContain('ready')
    } finally {
      wrapper.unmount()
    }
  })

  it('shows unknown database state after a successful overview refresh later fails', async () => {
    serveChecks()
    const pinia = createPinia()
    const wrapper = mount(OverviewView, { global: { plugins: [pinia] } })
    try {
      await flushPromises()
      expect(wrapper.text()).toContain('version 1')
      vi.stubGlobal(
        'fetch',
        vi.fn(() => Promise.reject(new TypeError('synthetic network failure'))),
      )
      await wrapper.find('button').trigger('click')
      await flushPromises()
      expect(useSystemStore(pinia).status).toBe('error')
      expect(wrapper.text()).toContain('检查失败')
      expect(wrapper.text()).toContain('未知')
      expect(wrapper.text()).not.toContain('version 1')
    } finally {
      wrapper.unmount()
    }
  })

  it('applies the same rules to manual and timed refresh and clears the timer on unmount', async () => {
    vi.useFakeTimers()
    serveChecks()
    const pinia = createPinia()
    const wrapper = mount(StatusView, { global: { plugins: [pinia] } })
    try {
      await flushPromises()
      expect(useSystemStore(pinia).status).toBe('ready')
      vi.stubGlobal(
        'fetch',
        vi.fn(() => Promise.reject(new TypeError('synthetic network failure'))),
      )
      await wrapper.find('button').trigger('click')
      await flushPromises()
      expect(useSystemStore(pinia).status).toBe('error')
      expect(wrapper.text()).not.toContain('version 1')
      serveChecks()
      await wrapper.find('input[type="checkbox"]').setValue(true)
      await vi.advanceTimersByTimeAsync(30_000)
      await flushPromises()
      expect(useSystemStore(pinia).status).toBe('ready')
      expect(wrapper.text()).toContain('version 1')
    } finally {
      wrapper.unmount()
    }
    expect(vi.getTimerCount()).toBe(0)
  })
})
