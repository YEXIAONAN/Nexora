<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'

import AppIcon from '@/components/ui/AppIcon.vue'
import type { IconName } from '@/components/ui/icons'
import { useAuthStore } from '@/stores/auth'
import { useTeamsStore } from '@/stores/teams'
import { useThemeStore } from '@/stores/theme'

defineEmits<{ 'open-nav': [] }>()

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const teams = useTeamsStore()
const theme = useThemeStore()

const title = computed(() => (typeof route.meta.title === 'string' ? route.meta.title : 'Nexora'))

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

const initials = computed(() => {
  const name = auth.displayName.trim()
  return name ? name.slice(0, 2) : 'N'
})

const userMenuOpen = ref(false)
const userMenuRef = ref<HTMLElement | null>(null)

function onDocumentClick(event: MouseEvent): void {
  if (!userMenuOpen.value) return
  if (userMenuRef.value && !userMenuRef.value.contains(event.target as Node)) {
    userMenuOpen.value = false
  }
}

onMounted(() => document.addEventListener('click', onDocumentClick))
onBeforeUnmount(() => document.removeEventListener('click', onDocumentClick))

function cycleTheme(): void {
  const next =
    theme.preference === 'system' ? 'light' : theme.preference === 'light' ? 'dark' : 'system'
  theme.setPreference(next)
}

async function onTeamChange(event: Event): Promise<void> {
  const teamId = (event.target as HTMLSelectElement).value
  teams.selectTeam(teamId)
  if (route.params.teamId && teamId !== route.params.teamId) {
    const name = String(route.name ?? '')
    if (['projects', 'team-members', 'my-contribution-results'].includes(name)) {
      await router.push({ name, params: { teamId } })
    } else if (name.startsWith('project-') || name === 'contribution-case') {
      await router.push({ name: 'projects', params: { teamId } })
    }
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
    class="sticky top-0 z-20 flex h-14 items-center gap-2 border-b border-line bg-canvas/90 px-3 backdrop-blur sm:px-5"
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

    <div class="ml-auto flex items-center gap-1.5">
      <select
        v-if="auth.isAuthenticated && teams.teams.length > 0"
        class="hidden h-8 max-w-[168px] rounded-lg border border-line bg-surface px-2 text-xs text-ink focus:border-accent focus:outline-none sm:block"
        :value="teams.currentTeamId ?? ''"
        aria-label="当前团队"
        @change="onTeamChange"
      >
        <option v-for="team in teams.teams" :key="team.id" :value="team.id">
          {{ team.name }}
        </option>
      </select>

      <RouterLink
        v-if="auth.isAuthenticated"
        to="/invitations"
        class="btn btn-ghost h-9 w-9 px-0"
        aria-label="邀请收件箱"
        title="邀请收件箱"
      >
        <AppIcon name="bell" :size="17" />
      </RouterLink>

      <button
        type="button"
        class="btn btn-ghost h-9 gap-2 px-2.5"
        :aria-label="`主题：${themeLabel}`"
        :title="`主题：${themeLabel}`"
        @click="cycleTheme"
      >
        <AppIcon :name="themeIcon" :size="16" />
        <span class="hidden text-xs xl:inline">{{ themeLabel }}</span>
      </button>

      <div v-if="auth.isAuthenticated" ref="userMenuRef" class="relative hidden sm:block">
        <button
          type="button"
          class="flex items-center gap-2 rounded-lg border border-transparent py-1 pr-2 pl-1.5 transition-colors hover:border-line hover:bg-surface-muted"
          aria-label="用户菜单"
          :aria-expanded="userMenuOpen"
          @click.stop="userMenuOpen = !userMenuOpen"
        >
          <span class="avatar h-7 w-7 text-[11px]" aria-hidden="true">{{ initials }}</span>
          <div class="leading-tight">
            <p class="max-w-[120px] truncate text-xs font-medium text-ink">
              {{ auth.displayName }}
            </p>
            <p class="text-[10px] text-ink-subtle">
              {{
                auth.user?.system_root ? 'ROOT' : auth.user?.platform_admin ? '平台管理员' : '成员'
              }}
            </p>
          </div>
          <AppIcon name="chevron-down" :size="13" class="text-ink-subtle" />
        </button>

        <div
          v-if="userMenuOpen"
          class="absolute right-0 mt-1.5 w-52 overflow-hidden rounded-lg border border-line bg-surface shadow-lg"
          role="menu"
        >
          <div class="border-b border-line px-3 py-2.5">
            <p class="truncate text-xs font-medium text-ink">{{ auth.displayName }}</p>
            <p class="truncate font-mono text-[11px] text-ink-subtle">@{{ auth.user?.username }}</p>
          </div>
          <div class="p-1">
            <RouterLink
              to="/settings"
              class="flex items-center gap-2 rounded-md px-2.5 py-1.5 text-xs text-ink-muted hover:bg-surface-muted hover:text-ink"
              role="menuitem"
              @click="userMenuOpen = false"
            >
              <AppIcon name="settings" :size="14" />
              个人设置
            </RouterLink>
            <RouterLink
              to="/invitations"
              class="flex items-center gap-2 rounded-md px-2.5 py-1.5 text-xs text-ink-muted hover:bg-surface-muted hover:text-ink"
              role="menuitem"
              @click="userMenuOpen = false"
            >
              <AppIcon name="inbox" :size="14" />
              邀请收件箱
            </RouterLink>
            <button
              type="button"
              class="flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-left text-xs text-danger hover:bg-danger-soft"
              role="menuitem"
              @click="logout"
            >
              <AppIcon name="logout" :size="14" />
              退出登录
            </button>
          </div>
        </div>
      </div>

      <button
        v-if="auth.isAuthenticated"
        type="button"
        class="btn btn-ghost h-9 gap-1.5 px-2.5"
        aria-label="退出登录"
        title="退出登录"
        @click="logout"
      >
        <AppIcon name="logout" :size="16" />
        <span class="hidden text-xs lg:inline">退出</span>
      </button>
    </div>
  </header>
</template>
