<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import AppIcon from '@/components/ui/AppIcon.vue'
import type { IconName } from '@/components/ui/icons'
import { useAuthStore } from '@/stores/auth'
import { useSystemStore } from '@/stores/system'
import { useTeamsStore } from '@/stores/teams'
import { useThemeStore } from '@/stores/theme'

defineEmits<{ 'open-nav': [] }>()

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const system = useSystemStore()
const teams = useTeamsStore()
const theme = useThemeStore()

const title = computed(() => (typeof route.meta.title === 'string' ? route.meta.title : 'Nexora'))

const environmentLabel = computed(() => {
  if (system.loading) return '检查中'
  if (!system.meta) return '未连接'
  const labels = { production: '生产环境', test: '测试环境', development: '开发环境' }
  return labels[system.meta.environment]
})

const environmentTone = computed(() => {
  if (system.apiState === 'down') return 'bg-danger'
  if (system.meta) return 'bg-accent'
  return 'bg-ink-subtle'
})

const themeLabel = computed(() => {
  if (theme.preference === 'light') return '浅色'
  if (theme.preference === 'dark') return '深色'
  return '跟随系统'
})

const themeIcon = computed<IconName>(() => {
  if (theme.preference === 'light') return 'sun'
  if (theme.preference === 'dark') return 'moon'
  return 'monitor'
})

function cycleTheme(): void {
  const next =
    theme.preference === 'system' ? 'light' : theme.preference === 'light' ? 'dark' : 'system'
  theme.setPreference(next)
}

async function onTeamChange(event: Event): Promise<void> {
  const teamId = (event.target as HTMLSelectElement).value
  teams.selectTeam(teamId)
  if (
    (route.name === 'projects' || route.name === 'project-detail') &&
    teamId !== route.params.teamId
  ) {
    await router.push({ name: 'projects', params: { teamId } })
  }
  if (route.name === 'team-members' && teamId !== route.params.teamId) {
    await router.push({ name: 'team-members', params: { teamId } })
  }
}

async function logout(): Promise<void> {
  await auth.logout()
  teams.reset()
  await router.push({ name: 'login' })
}
</script>

<template>
  <header
    class="sticky top-0 z-20 flex h-16 items-center gap-3 border-b border-line bg-canvas/90 px-4 backdrop-blur sm:px-6 lg:px-8"
  >
    <button
      type="button"
      class="btn btn-ghost h-9 w-9 px-0 lg:hidden"
      aria-label="打开导航"
      @click="$emit('open-nav')"
    >
      <AppIcon name="menu" :size="18" />
    </button>

    <h1 class="truncate text-sm font-semibold text-ink">{{ title }}</h1>

    <div class="ml-auto flex items-center gap-2">
      <select
        v-if="auth.isAuthenticated && teams.teams.length > 0"
        class="hidden max-w-[180px] rounded-lg border border-line bg-surface px-2 py-1.5 text-xs text-ink focus:border-accent focus:outline-none sm:block"
        :value="teams.currentTeamId ?? ''"
        aria-label="当前团队"
        @change="onTeamChange"
      >
        <option v-for="team in teams.teams" :key="team.id" :value="team.id">
          {{ team.name }}
        </option>
      </select>

      <span v-if="auth.isAuthenticated" class="badge hidden max-w-[200px] md:inline-flex">
        <span class="truncate">{{ auth.displayName }}</span>
        <span v-if="auth.user?.system_root" class="font-semibold text-accent-strong">ROOT</span>
      </span>

      <span class="badge hidden sm:inline-flex">
        <span class="h-1.5 w-1.5 rounded-full" :class="environmentTone" aria-hidden="true" />
        {{ environmentLabel }}
      </span>
      <button
        type="button"
        class="btn btn-ghost h-9 gap-2 px-2.5"
        :aria-label="`主题：${themeLabel}`"
        :title="`主题：${themeLabel}`"
        @click="cycleTheme"
      >
        <AppIcon :name="themeIcon" :size="16" />
        <span class="hidden text-xs sm:inline">{{ themeLabel }}</span>
      </button>
      <button
        v-if="auth.isAuthenticated"
        type="button"
        class="btn btn-ghost h-9 gap-2 px-2.5"
        aria-label="退出登录"
        title="退出登录"
        @click="logout"
      >
        <AppIcon name="logout" :size="16" />
        <span class="hidden text-xs sm:inline">退出</span>
      </button>
    </div>
  </header>
</template>
