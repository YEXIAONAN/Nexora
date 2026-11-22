import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useAuthStore } from '@/stores/auth'

import TeamsView from './TeamsView.vue'

const { pushMock } = vi.hoisted(() => ({ pushMock: vi.fn() }))

vi.mock('vue-router', async () => {
  const { defineComponent, h } = await import('vue')
  return {
    useRouter: () => ({ push: pushMock }),
    RouterLink: defineComponent({
      props: { to: { type: String, required: true } },
      setup(props, { slots }) {
        return () => h('a', { href: props.to }, slots.default?.())
      },
    }),
  }
})

function jsonResponse(payload: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => payload,
  } as Response
}

const me = {
  id: 'u1',
  username: 'waiting',
  display_name: '',
  system_root: false,
  platform_admin: false,
}
const adminMe = { ...me, system_root: true }
const others = [
  { id: 'u2', username: 'alice', display_name: 'Alice', system_root: false, platform_admin: false },
]

const teamA = {
  id: '11111111-1111-4111-8111-111111111111',
  name: 'Team A',
  description: '第一个团队',
  created_by: 'u1',
  created_at: '2026-11-17T00:00:00Z',
  updated_at: '2026-11-17T00:00:00Z',
  my_role: 'owner' as const,
}

function stubFetch(handler: (input: string, init?: RequestInit) => Response | Promise<Response>) {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) =>
      Promise.resolve(handler(String(input), init)),
    ),
  )
}

async function mountAuthenticated() {
  const pinia = createPinia()
  setActivePinia(pinia)
  await useAuthStore().initialize()
  return mount(TeamsView, { global: { plugins: [pinia] } })
}

function usersPage() {
  return { data: { items: [adminMe, ...others], total: 2, limit: 100, offset: 0 } }
}

describe('TeamsView', () => {
  beforeEach(() => {
    window.localStorage.clear()
    pushMock.mockClear()
  })

  afterEach(() => vi.unstubAllGlobals())

  it('shows the empty state with a link to the invitation inbox', async () => {
    stubFetch((input) =>
      input.includes('/auth/me') ? jsonResponse({ data: me }) : jsonResponse({ data: [] }),
    )
    const wrapper = await mountAuthenticated()
    await flushPromises()
    expect(wrapper.text()).toContain('你还没有加入任何团队')
    expect(wrapper.find('a[href="/invitations"]').exists()).toBe(true)
  })

  it('renders real teams with per-team role badges', async () => {
    stubFetch((input) =>
      input.includes('/auth/me') ? jsonResponse({ data: me }) : jsonResponse({ data: [teamA] }),
    )
    const wrapper = await mountAuthenticated()
    await flushPromises()
    expect(wrapper.text()).toContain('Team A')
    expect(wrapper.text()).toContain('所有者')
    expect(wrapper.find(`[data-team-id="${teamA.id}"]`).exists()).toBe(true)
  })

  it('creates a team through the API and shows the success state', async () => {
    const calls: { input: string; init?: RequestInit }[] = []
    stubFetch((input, init) => {
      calls.push({ input, init })
      if (input.includes('/auth/me')) return jsonResponse({ data: adminMe })
      if (input.includes('/admin/users')) return jsonResponse(usersPage())
      if (init?.method === 'POST') {
        return jsonResponse(
          { data: { ...teamA, id: '33333333-3333-4333-8333-333333333333', name: '新团队' } },
          201,
        )
      }
      return jsonResponse({ data: [] })
    })
    const wrapper = await mountAuthenticated()
    await flushPromises()
    await wrapper.find('#team-name').setValue('新团队')
    await wrapper.find('#team-owner').setValue('u2')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    const createCall = calls.find((c) => c.init?.method === 'POST')
    expect(createCall?.input).toContain('/api/v1/teams')
    expect(JSON.parse(String(createCall?.init?.body))).toEqual({ name: '新团队', owner_id: 'u2' })
    expect(wrapper.text()).toContain('团队「新团队（负责人：Alice）」已创建')
    expect(wrapper.text()).toContain('新团队')
  })

  it('does not offer team creation to ordinary accounts', async () => {
    stubFetch((input) =>
      input.includes('/auth/me') ? jsonResponse({ data: me }) : jsonResponse({ data: [] }),
    )
    const wrapper = await mountAuthenticated()
    await flushPromises()
    expect(wrapper.find('#team-name').exists()).toBe(false)
    expect(wrapper.text()).toContain('团队由平台管理员创建')
    expect(wrapper.text()).toContain('普通账号不能自助创建团队')
  })
})
