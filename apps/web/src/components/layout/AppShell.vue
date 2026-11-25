<script setup lang="ts">
import { onMounted, ref } from 'vue'

import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'
import ToastHost from '@/components/ui/ToastHost.vue'
import { useAuthStore } from '@/stores/auth'
import { useTeamsStore } from '@/stores/teams'

import AppSidebar from './AppSidebar.vue'
import AppTopbar from './AppTopbar.vue'

const navOpen = ref(false)
const auth = useAuthStore()
const teams = useTeamsStore()

onMounted(async () => {
  await auth.initialize()
  if (auth.user) await teams.loadTeams(auth.user.id)
})
</script>

<template>
  <div class="min-h-screen bg-canvas">
    <a
      href="#main-content"
      class="sr-only z-[80] rounded-control bg-surface px-3 py-2 text-caption text-ink focus:not-sr-only focus:fixed focus:top-3 focus:left-3"
    >
      跳到主要内容
    </a>

    <AppSidebar :open="navOpen" @close="navOpen = false" />

    <div class="flex min-h-screen flex-col lg:pl-[var(--sidebar-width)]">
      <AppTopbar @open-nav="navOpen = true" />

      <main id="main-content" class="app-main flex-1">
        <slot />
      </main>

      <footer
        class="mx-auto w-full max-w-[var(--content-max)] px-[var(--content-padding-x)] pb-6 text-meta text-ink-subtle"
      >
        Nexora · 团队任务与贡献协作
      </footer>
    </div>

    <ToastHost />
    <ConfirmDialog />
  </div>
</template>
