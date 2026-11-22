import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { ApiError } from '@/api/client'
import * as bonusAPI from '@/api/bonus'
import * as projectsAPI from '@/api/projects'
import MyBonusResultsView from './MyBonusResultsView.vue'

vi.mock('@/api/bonus', () => ({ listMyBonusResults: vi.fn() }))
vi.mock('@/api/projects', () => ({ listProjects: vi.fn() }))

const confirmedAt = '2026-11-20T02:00:00Z'

function mountView() {
  setActivePinia(createPinia())
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/teams/:teamId/bonus-results', component: MyBonusResultsView },
      {
        path: '/teams/:teamId/projects/:projectId/bonus-rounds/:roundId',
        name: 'bonus-round-detail',
        component: { template: '<div />' },
      },
      { path: '/teams/:teamId/projects', component: { template: '<div />' } },
      { path: '/teams/:teamId/members', component: { template: '<div />' } },
      { path: '/teams/:teamId/contribution-results', component: { template: '<div />' } },
    ],
  })
  return { router }
}

describe('MyBonusResultsView', () => {
  beforeEach(() => {
    vi.mocked(projectsAPI.listProjects).mockResolvedValue([])
    vi.mocked(bonusAPI.listMyBonusResults).mockReset()
  })

  it('renders the explicit empty state instead of fabricating amounts', async () => {
    vi.mocked(bonusAPI.listMyBonusResults).mockResolvedValue({ items: [], has_more: false })
    const { router } = mountView()
    await router.push('/teams/A/bonus-results')
    const wrapper = mount(MyBonusResultsView, { global: { plugins: [router] } })
    await flushPromises()
    expect(wrapper.text()).toContain('暂无已核定的奖金结果')
    expect(wrapper.text()).not.toContain('¥')
  })

  it('shows only confirmed amounts with a not-paid disclaimer', async () => {
    vi.mocked(bonusAPI.listMyBonusResults).mockResolvedValue({
      items: [
        {
          round_id: 'round',
          project_id: 'project',
          funding_label: '学院奖学金 2026',
          currency_code: 'CNY',
          currency_minor_unit: 2,
          amount_minor: 123456,
          status: 'confirmed',
          payment_status: 'not_recorded',
          confirmed_at: confirmedAt,
          published_at: confirmedAt,
        },
      ],
      has_more: false,
    })
    vi.mocked(projectsAPI.listProjects).mockResolvedValue([
      { id: 'project', name: '示例项目' },
    ] as unknown as projectsAPI.Project[])
    const { router } = mountView()
    await router.push('/teams/A/bonus-results')
    const wrapper = mount(MyBonusResultsView, { global: { plugins: [router] } })
    await flushPromises()
    expect(wrapper.text()).toContain('学院奖学金 2026')
    expect(wrapper.text()).toContain('¥1,234.56')
    expect(wrapper.text()).toContain('不代表已支付')
  })

  it('labels recorded payment as simulation rather than actual transfer', async () => {
    vi.mocked(bonusAPI.listMyBonusResults).mockResolvedValue({
      items: [
        {
          round_id: 'round',
          project_id: 'project',
          funding_label: '专项奖励',
          currency_code: 'CNY',
          currency_minor_unit: 2,
          amount_minor: 29,
          status: 'confirmed',
          payment_status: 'recorded',
          confirmed_at: confirmedAt,
          published_at: confirmedAt,
        },
      ],
      has_more: false,
    })
    const { router } = mountView()
    await router.push('/teams/A/bonus-results')
    const wrapper = mount(MyBonusResultsView, { global: { plugins: [router] } })
    await flushPromises()
    expect(wrapper.text()).toContain('已发布 · 已记录模拟发放')
    expect(wrapper.text()).toContain('模拟发放记录非实际转账')
    expect(wrapper.text()).toContain('¥0.29')
    wrapper.unmount()
  })

  it('explains a permission failure without leaking whether data exists', async () => {
    vi.mocked(bonusAPI.listMyBonusResults).mockRejectedValue(new ApiError('not found', 404))
    const { router } = mountView()
    await router.push('/teams/A/bonus-results')
    const wrapper = mount(MyBonusResultsView, { global: { plugins: [router] } })
    await flushPromises()
    expect(wrapper.text()).toContain('无法读取奖金结果')
    expect(wrapper.text()).toContain('严格绑定本人身份')
  })
})
