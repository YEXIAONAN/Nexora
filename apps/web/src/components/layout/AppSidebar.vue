<script setup lang="ts">
import { RouterLink } from 'vue-router'

import AppIcon from '@/components/ui/AppIcon.vue'
import type { IconName } from '@/components/ui/icons'

defineProps<{ open: boolean }>()
defineEmits<{ close: [] }>()

const appVersion = __APP_VERSION__

const navItems: { label: string; to: string; icon: IconName }[] = [
  { label: '系统总览', to: '/', icon: 'overview' },
  { label: '运行状态', to: '/status', icon: 'activity' },
]
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
      class="fixed inset-y-0 left-0 z-40 flex w-[272px] flex-col border-r border-line bg-surface transition-transform duration-200 lg:w-[248px] lg:translate-x-0"
      :class="open ? 'translate-x-0' : '-translate-x-full'"
      aria-label="主导航"
    >
      <div class="flex h-16 items-center justify-between px-5">
        <RouterLink to="/" class="flex items-center gap-2.5" @click="$emit('close')">
          <span
            class="flex h-8 w-8 items-center justify-center rounded-lg bg-accent text-sm font-semibold text-white"
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
          <AppIcon name="close" :size="18" />
        </button>
      </div>

      <div class="px-3 py-2">
        <p class="px-3 pb-2 text-[11px] font-medium tracking-[0.14em] text-ink-subtle uppercase">
          工作区
        </p>
        <nav class="space-y-1">
          <RouterLink
            v-for="item in navItems"
            :key="item.to"
            :to="item.to"
            class="nav-item"
            active-class="nav-item-active"
            @click="$emit('close')"
          >
            <AppIcon :name="item.icon" :size="17" />
            <span>{{ item.label }}</span>
          </RouterLink>
        </nav>
      </div>

      <div class="mt-auto px-4 pb-5">
        <div class="rounded-lg border border-line bg-surface-muted px-3 py-3">
          <p class="flex items-center gap-2 text-xs font-medium text-ink">
            <AppIcon name="shield" :size="14" />
            Phase 0 · 工程基线
          </p>
          <p class="mt-1.5 text-xs leading-5 text-ink-subtle">
            当前仅提供工程基线与真实运行状态，业务模块按阶段交付。
          </p>
          <p class="mt-2 font-mono text-[11px] text-ink-subtle">v{{ appVersion }}</p>
        </div>
      </div>
    </aside>
  </div>
</template>
