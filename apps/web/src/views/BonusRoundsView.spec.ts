import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import BonusRoundsView from './BonusRoundsView.vue'

let wrapper: VueWrapper | undefined
let canCreate: boolean
let forbidden: boolean
const writes: { path: string; body: Record<string, unknown>; options: RequestInit }[] = []
function response(data: unknown) {
  return new Response(JSON.stringify({ data }), { status: 200 })
}
async function render() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      {
        path: '/teams/:teamId/projects/:projectId/bonus-rounds',
        name: 'bonus-rounds',
        component: BonusRoundsView,
      },
      {
        path: '/teams/:teamId/projects/:projectId',
        name: 'project-detail',
        component: { template: '<div />' },
      },
      {
        path: '/teams/:teamId/projects/:projectId/bonus-rounds/:roundId',
        name: 'bonus-round-detail',
        component: { template: '<div />' },
      },
      {
        path: '/teams/:teamId/bonus-results',
        name: 'my-bonus-results',
        component: { template: '<div />' },
      },
    ],
  })
  await router.push('/teams/A/projects/project/bonus-rounds')
  wrapper = mount(BonusRoundsView, { global: { plugins: [router] } })
  await flushPromises()
  return wrapper
}
async function field(w: VueWrapper, label: string, value: string) {
  const node = w.findAll('label').find((l) => l.text().startsWith(label))!
  await node.find('input,textarea,select').setValue(value)
}
beforeEach(() => {
  setActivePinia(createPinia())
  useAuthStore().setUser({
    id: 'self',
    display_name: '本人',
    username: 'self',
    system_root: true,
  } as Parameters<ReturnType<typeof useAuthStore>['setUser']>[0])
  canCreate = true
  forbidden = false
  writes.length = 0
  vi.stubGlobal(
    'fetch',
    vi.fn(async (path: string, options: RequestInit) => {
      if (options.method !== 'GET') {
        writes.push({ path, body: JSON.parse(String(options.body)), options })
        return response({ round_id: 'new-round', revision: 1 })
      }
      if (path.endsWith('/bonus-workspace')) {
        if (forbidden)
          return new Response(JSON.stringify({ error: { message: 'not allowed' } }), {
            status: 403,
          })
        return response({
          can_create: canCreate,
          members: [
            { user_id: 'self', display_name: '本人' },
            { user_id: 'manager', display_name: '负责人小林' },
            { user_id: 'reviewer', display_name: '核定人小周' },
          ],
          sources: [],
        })
      }
      if (path.endsWith('/bonus-rounds')) return response({ items: [], has_more: false })
      return response({ id: 'project', name: '真实项目', status: 'active' })
    }),
  )
})
afterEach(() => {
  wrapper?.unmount()
  vi.unstubAllGlobals()
})

describe('bonus round creation', () => {
  it('creates with real API wiring, filters self and explains initial permissions', async () => {
    const w = await render()
    await w
      .findAll('button')
      .find((b) => b.text() === '创建奖金批次')!
      .trigger('click')
    expect(w.findAll('option').map((o) => o.text())).not.toContain('本人')
    await field(w, '来源标识', 'school-award-2026')
    await field(w, '批次名称', '学院专项奖励')
    await field(w, '真实资金来源', '学院独立拨款')
    await field(w, '金额负责人', 'manager')
    await field(w, '独立核定人', 'reviewer')
    await field(w, '创建理由', '资金已到位')
    await w.find('form').trigger('submit')
    await flushPromises()
    expect(writes).toHaveLength(1)
    expect(writes[0]!.path).toBe('/api/v1/teams/A/projects/project/bonus-rounds')
    expect(writes[0]!.body).toMatchObject({
      funding_key: 'school-award-2026',
      manager_id: 'manager',
      reviewer_id: 'reviewer',
      reason: '资金已到位',
    })
    expect(writes[0]!.body).not.toHaveProperty('total_budget_minor')
    expect(writes[0]!.body.client_request_id).toMatch(/^[0-9a-f-]{36}$/)
    expect(writes[0]!.options.headers).toMatchObject({ 'X-Nexora-CSRF': '1' })
    expect(w.text()).toContain('当前创建者不自动获得金额权限')
    expect(w.find('a[href$="/new-round"]').exists()).toBe(true)
  })

  it.each(['forbidden', 'read-only'])(
    'keeps metadata usable without management rights: %s',
    async (mode) => {
      forbidden = mode === 'forbidden'
      canCreate = false
      const w = await render()
      expect(w.text()).toContain('真实项目 暂无奖金批次')
      expect(w.text()).not.toContain('创建奖金批次')
      expect(w.text()).not.toContain('无法查看该项目')
    },
  )

  it('rejects uppercase funding identifiers locally instead of posting invalid data', async () => {
    const w = await render()
    await w
      .findAll('button')
      .find((b) => b.text() === '创建奖金批次')!
      .trigger('click')
    await field(w, '来源标识', 'Invalid-Key')
    await w.find('form').trigger('submit')
    expect(w.text()).toContain('来源标识只能使用小写')
    expect(writes).toHaveLength(0)
  })
})
