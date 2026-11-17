import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import LoginView from './LoginView.vue'

const { pushMock, routeState } = vi.hoisted(() => ({
  pushMock: vi.fn(),
  routeState: { query: {} as Record<string, string> },
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({ push: pushMock }),
}))

function jsonResponse(payload: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => payload,
  } as Response
}

const meta = {
  name: 'Nexora',
  version: '0.1.0',
  commit: 'abc1234',
  build_time: '2026-11-17T00:00:00Z',
  environment: 'test',
  instance_id: null,
  root_initialized: true,
  server_time: '2026-11-17T00:00:00Z',
}

const user = {
  id: '00000000-0000-4000-8000-000000000001',
  username: 'waiting',
  display_name: '',
  system_root: true,
  platform_admin: false,
}

function stubFetch(handler: (input: string, init?: RequestInit) => Response | Promise<Response>) {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) =>
      Promise.resolve(handler(String(input), init)),
    ),
  )
}

describe('LoginView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    routeState.query = {}
    pushMock.mockClear()
  })

  afterEach(() => vi.unstubAllGlobals())

  it('shows the ROOT bootstrap hint without any default credentials', async () => {
    stubFetch(() => jsonResponse({ data: { ...meta, root_initialized: false } }))
    const wrapper = mount(LoginView)
    await flushPromises()
    expect(wrapper.text()).toContain('系统尚未初始化 ROOT 账号')
    expect(wrapper.text()).toContain('make bootstrap')
    expect(wrapper.text()).not.toContain('123456')
    expect(wrapper.find('input[type="password"]').attributes('value')).toBeUndefined()
  })

  it('hides the bootstrap hint once ROOT is initialized', async () => {
    stubFetch(() => jsonResponse({ data: meta }))
    const wrapper = mount(LoginView)
    await flushPromises()
    expect(wrapper.text()).not.toContain('系统尚未初始化 ROOT 账号')
  })

  it('shows the expired-session banner from the redirect query', async () => {
    routeState.query = { expired: '1' }
    stubFetch(() => jsonResponse({ data: meta }))
    const wrapper = mount(LoginView)
    await flushPromises()
    expect(wrapper.text()).toContain('会话已过期或已被撤销')
  })

  it('submits credentials with CSRF and navigates to the teams workspace', async () => {
    const calls: { input: string; init?: RequestInit }[] = []
    stubFetch((input, init) => {
      calls.push({ input, init })
      if (input.includes('/auth/login')) {
        return jsonResponse({ data: { user, expires_at: '2026-11-18T00:00:00Z' } })
      }
      return jsonResponse({ data: meta })
    })
    const wrapper = mount(LoginView)
    await flushPromises()
    await wrapper.find('#login-username').setValue('waiting')
    await wrapper.find('#login-password').setValue('correct-password-1')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    const loginCall = calls.find((c) => c.input.includes('/auth/login'))
    expect(loginCall?.init?.method).toBe('POST')
    expect((loginCall?.init?.headers as Record<string, string>)['X-Nexora-CSRF']).toBe('1')
    expect(JSON.parse(String(loginCall?.init?.body))).toEqual({
      username: 'waiting',
      password: 'correct-password-1',
    })
    expect(pushMock).toHaveBeenCalledWith('/teams')
  })

  it('renders the server error and stays on the page when login fails', async () => {
    stubFetch((input) =>
      input.includes('/auth/login')
        ? jsonResponse(
            { error: { code: 'unauthorized', message: '用户名或密码错误', request_id: 'r1' } },
            401,
          )
        : jsonResponse({ data: meta }),
    )
    const wrapper = mount(LoginView)
    await flushPromises()
    await wrapper.find('#login-username').setValue('waiting')
    await wrapper.find('#login-password').setValue('wrong-password-1')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.find('[role="alert"]').text()).toContain('用户名或密码错误')
    expect(pushMock).not.toHaveBeenCalled()
  })
})
