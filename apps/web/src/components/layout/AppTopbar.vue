<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

import AppIcon from '@/components/ui/AppIcon.vue'
import type { IconName } from '@/components/ui/icons'
import { useSystemStore } from '@/stores/system'
import { useThemeStore } from '@/stores/theme'

defineEmits<{ 'open-nav': [] }>()

const route = useRoute()
const system = useSystemStore()
const theme = useThemeStore()

const title = computed(() => (typeof route.meta.title === 'string' ? route.meta.title : 'Nexora'))

const environmentLabel = computed(() => {
  if (!system.meta) return '未连接'
  return system.meta.environment === 'production' ? '生产环境' : '开发环境'
})

const environmentTone = computed(() => {
  if (system.error) return 'bg-danger'
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
    </div>
  </header>
</template>
