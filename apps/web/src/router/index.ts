import { createRouter, createWebHistory } from 'vue-router'

import { useAuthStore } from '@/stores/auth'

declare module 'vue-router' {
  interface RouteMeta {
    title?: string
    requiresAuth?: boolean
    requiresAdmin?: boolean
    bare?: boolean
  }
}

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
      meta: { title: '登录', bare: true },
    },
    {
      path: '/register',
      name: 'register',
      component: () => import('@/views/RegisterView.vue'),
      meta: { title: '创建账号', bare: true },
    },
    {
      path: '/',
      name: 'overview',
      component: () => import('@/views/DashboardView.vue'),
      meta: { title: '工作台', requiresAuth: true },
    },
    {
      path: '/my-tasks',
      name: 'my-tasks',
      component: () => import('@/views/MyTasksView.vue'),
      meta: { title: '我的任务', requiresAuth: true },
    },
    {
      path: '/settings',
      name: 'settings',
      component: () => import('@/views/SettingsView.vue'),
      meta: { title: '设置', requiresAuth: true },
    },
    {
      path: '/admin/users',
      name: 'admin-users',
      component: () => import('@/views/AdminUsersView.vue'),
      meta: { title: '用户管理', requiresAuth: true, requiresAdmin: true },
    },
    {
      path: '/admin/system',
      name: 'admin-system',
      component: () => import('@/views/StatusView.vue'),
      meta: { title: '系统状态', requiresAuth: true, requiresAdmin: true },
    },
    {
      path: '/status',
      name: 'status',
      component: () => import('@/views/StatusView.vue'),
      meta: { title: '运行状态', requiresAuth: true, requiresAdmin: true },
    },
    {
      path: '/teams',
      name: 'teams',
      component: () => import('@/views/TeamsView.vue'),
      meta: { title: '我的团队', requiresAuth: true },
    },
    {
      path: '/teams/:teamId/members',
      name: 'team-members',
      component: () => import('@/views/TeamMembersView.vue'),
      meta: { title: '团队成员', requiresAuth: true },
    },
    {
      path: '/teams/:teamId/projects',
      name: 'projects',
      component: () => import('@/views/ProjectsView.vue'),
      meta: { title: '项目', requiresAuth: true },
    },
    {
      path: '/teams/:teamId/projects/:projectId',
      name: 'project-detail',
      component: () => import('@/views/ProjectDetailView.vue'),
      meta: { title: '项目工作区', requiresAuth: true },
    },
    {
      path: '/teams/:teamId/contribution-results',
      name: 'my-contribution-results',
      component: () => import('@/views/MyContributionResultsView.vue'),
      meta: { title: '我的贡献结果', requiresAuth: true },
    },
    {
      path: '/teams/:teamId/projects/:projectId/contributions',
      name: 'project-contributions',
      component: () => import('@/views/ProjectContributionsView.vue'),
      meta: { title: '贡献核算', requiresAuth: true },
    },
    {
      path: '/teams/:teamId/projects/:projectId/contributions/:caseId',
      name: 'contribution-case',
      component: () => import('@/views/ContributionCaseView.vue'),
      meta: { title: '贡献案例', requiresAuth: true },
    },
    {
      path: '/teams/:teamId/projects/:projectId/contribution-appraisal',
      name: 'contribution-appraisal',
      component: () => import('@/views/ContributionAppraisalView.vue'),
      meta: { title: '独立核定工作台', requiresAuth: true },
    },
    {
      path: '/contribution-dispute-requests',
      name: 'contribution-dispute-intake',
      component: () => import('@/views/ContributionDisputeIntakeView.vue'),
      meta: { title: '本人异议请求受理', requiresAuth: true },
    },
    {
      path: '/invitations',
      name: 'invitations',
      component: () => import('@/views/InvitationsView.vue'),
      meta: { title: '邀请收件箱', requiresAuth: true },
    },
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: () => import('@/views/NotFoundView.vue'),
      meta: { title: '页面不存在' },
    },
  ],
  scrollBehavior: () => ({ top: 0 }),
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (to.meta.requiresAuth || to.name === 'login' || to.name === 'register') {
    await auth.initialize()
  }
  if (to.meta.requiresAuth && !auth.isAuthenticated) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.meta.requiresAdmin && !auth.isPlatformAdmin) {
    return { name: 'overview' }
  }
  if ((to.name === 'login' || to.name === 'register') && auth.isAuthenticated) {
    const redirect = to.query.redirect
    const target =
      typeof redirect === 'string' && redirect.startsWith('/') && !redirect.startsWith('//')
        ? redirect
        : '/teams'
    return { path: target }
  }
  return true
})

router.afterEach((to) => {
  document.title = to.meta.title ? `${to.meta.title} · Nexora` : 'Nexora'
})

export default router
