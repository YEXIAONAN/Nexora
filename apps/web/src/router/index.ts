import { createRouter, createWebHistory } from 'vue-router'

import { useAuthStore } from '@/stores/auth'

declare module 'vue-router' {
  interface RouteMeta {
    title?: string
    requiresAuth?: boolean
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
      path: '/',
      name: 'overview',
      component: () => import('@/views/OverviewView.vue'),
      meta: { title: '系统总览' },
    },
    {
      path: '/status',
      name: 'status',
      component: () => import('@/views/StatusView.vue'),
      meta: { title: '运行状态' },
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
  if (to.meta.requiresAuth || to.name === 'login') await auth.initialize()
  if (to.meta.requiresAuth && !auth.isAuthenticated) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.name === 'login' && auth.isAuthenticated) {
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
