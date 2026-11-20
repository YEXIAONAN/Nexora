<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink, useRoute } from 'vue-router'

import AppIcon from '@/components/ui/AppIcon.vue'
import type { IconName } from '@/components/ui/icons'
import { useAuthStore } from '@/stores/auth'
import { useTeamsStore } from '@/stores/teams'

defineProps<{ open: boolean }>()
defineEmits<{ close: [] }>()

interface NavItem {
  label: string
  to: string
  icon: IconName
  match?: string
  badge?: number
}

const auth = useAuthStore()
const teams = useTeamsStore()
const route = useRoute()
const appVersion = __APP_VERSION__

const teamId = computed(() => teams.currentTeamId)

const workspace = computed<NavItem[]>(() => [
  { label: '工作台', to: '/', icon: 'home' },
  { label: '我的任务', to: '/my-tasks', icon: 'check' },
])

const collaboration = computed<NavItem[]>(() => {
  const id = teamId.value
  return [
    { label: '团队', to: '/teams', icon: 'users' },
    {
      label: '项目',
      to: id ? `/teams/${id}/projects` : '/teams',
      icon: 'folder',
      match: id ? `/teams/${id}/projects` : '/__no-team__/projects',
    },
    {
      label: '团队成员',
      to: id ? `/teams/${id}/members` : '/teams',
      icon: 'user',
      match: id ? `/teams/${id}/members` : '/__no-team__/members',
    },
    {
      label: '我的贡献',
      to: id ? `/teams/${id}/contribution-results` : '/teams',
      icon: 'layers',
      match: id ? `/teams/${id}/contribution-results` : '/__no-team__/contributions',
    },
    {
      label: '我的奖金',
      to: id ? `/teams/${id}/bonus-results` : '/teams',
      icon: 'award',
      match: id ? `/teams/${id}/bonus-results` : '/__no-team__/bonus',
    },
    { label: '邀请收件箱', to: '/invitations', icon: 'inbox' },
  ]
})

const management = computed<NavItem[]>(() => [{ label: '设置', to: '/settings', icon: 'settings' }])

const administration: NavItem[] = [
  { label: '用户管理', to: '/admin/users', icon: 'user' },
  { label: '系统状态', to: '/admin/system', icon: 'activity' },
]

function isActive(item: NavItem): boolean {
  if (item.match) return route.path.startsWith(item.match)
  if (item.to === '/') return route.path === '/'
  if (item.to === '/teams') return route.path === '/teams'
  return route.path === item.to
}
</script>

<template>
  <div>
    <Transition
      enter-active-class="transition-opacity duration-150"
      enter-from-class="opacity-0"
      leave-active-class="transition-opacity duration-150"
      leave-to-class="opacity-0"
    >
      <div
        v-if="open"
        class="fixed inset-0 z-30 bg-black/40 lg:hidden"
        aria-hidden="true"
        @click="$emit('close')"
      />
    </Transition>

    <aside
      class="fixed inset-y-0 left-0 z-40 flex w-[268px] flex-col border-r border-line bg-surface transition-transform duration-200 lg:w-[240px] lg:translate-x-0"
      :class="open ? 'translate-x-0' : '-translate-x-full'"
      aria-label="主导航"
    >
      <div class="flex h-14 items-center justify-between px-4">
        <RouterLink to="/" class="flex items-center gap-2.5" @click="$emit('close')">
          <span
            class="flex h-7 w-7 items-center justify-center rounded-md bg-accent text-xs font-semibold text-white"
            aria-hidden="true"
          >
            N
          </span>
          <span class="text-[15px] font-semibold tracking-tight text-ink">Nexora</span>
        </RouterLink>
        <button
          type="button"
          class="btn btn-ghost h-8 w-8 px-0 lg:hidden"
          aria-label="关闭导航"
          @click="$emit('close')"
        >
          <AppIcon name="close" :size="17" />
        </button>
      </div>

      <div class="flex-1 overflow-y-auto px-2.5 pb-4">
        <p
          class="px-2.5 pt-3 pb-1 text-[10px] font-semibold tracking-[0.16em] text-ink-subtle uppercase"
        >
          工作区
        </p>
        <nav class="space-y-0.5">
          <RouterLink
            v-for="item in workspace"
            :key="item.to"
            :to="item.to"
            class="nav-item"
            :class="isActive(item) ? 'nav-item-active' : ''"
            @click="$emit('close')"
          >
            <AppIcon :name="item.icon" :size="16" />
            <span>{{ item.label }}</span>
          </RouterLink>
        </nav>

        <p
          class="px-2.5 pt-6 pb-1 text-[10px] font-semibold tracking-[0.16em] text-ink-subtle uppercase"
        >
          团队协作
        </p>
        <nav class="space-y-0.5">
          <RouterLink
            v-for="item in collaboration"
            :key="item.label"
            :to="item.to"
            class="nav-item"
            :class="isActive(item) ? 'nav-item-active' : ''"
            @click="$emit('close')"
          >
            <AppIcon :name="item.icon" :size="16" />
            <span class="truncate">{{ item.label }}</span>
          </RouterLink>
        </nav>

        <p
          class="px-2.5 pt-6 pb-1 text-[10px] font-semibold tracking-[0.16em] text-ink-subtle uppercase"
        >
          管理与设置
        </p>
        <nav class="space-y-0.5">
          <RouterLink
            v-for="item in management"
            :key="item.to"
            :to="item.to"
            class="nav-item"
            :class="isActive(item) ? 'nav-item-active' : ''"
            @click="$emit('close')"
          >
            <AppIcon :name="item.icon" :size="16" />
            <span>{{ item.label }}</span>
          </RouterLink>
          <template v-if="auth.isPlatformAdmin">
            <RouterLink
              v-for="item in administration"
              :key="item.to"
              :to="item.to"
              class="nav-item"
              :class="isActive(item) ? 'nav-item-active' : ''"
              @click="$emit('close')"
            >
              <AppIcon :name="item.icon" :size="16" />
              <span>{{ item.label }}</span>
            </RouterLink>
          </template>
        </nav>
      </div>

      <div class="border-t border-line px-3 py-3">
        <div
          v-if="teams.currentTeam"
          class="rounded-lg border border-line bg-surface-muted px-3 py-2.5"
        >
          <p class="text-[10px] font-medium tracking-wider text-ink-subtle uppercase">当前团队</p>
          <p class="mt-0.5 truncate text-xs font-medium text-ink">{{ teams.currentTeam.name }}</p>
        </div>
        <div
          v-else
          class="rounded-lg border border-dashed border-line px-3 py-2.5 text-xs text-ink-muted"
        >
          尚未加入团队
        </div>
        <p class="mt-2 px-1 font-mono text-[10px] text-ink-subtle">v{{ appVersion }}</p>
      </div>
    </aside>
  </div>
</template>
