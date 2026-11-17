<script setup lang="ts">
import { onMounted, ref } from 'vue'

import { useAuthStore } from '@/stores/auth'
import { useTeamsStore } from '@/stores/teams'

import AppSidebar from './AppSidebar.vue'
import AppTopbar from './AppTopbar.vue'

const appVersion = __APP_VERSION__
const navOpen = ref(false)
const auth = useAuthStore()
const teams = useTeamsStore()

onMounted(async () => {
  await auth.initialize()
  if (auth.user) await teams.loadTeams(auth.user.id)
})
</script>

<template>
  <div class="min-h-full">
    <AppSidebar :open="navOpen" @close="navOpen = false" />

    <div class="flex min-h-screen flex-col lg:pl-[248px]">
      <AppTopbar @open-nav="navOpen = true" />

      <main class="mx-auto w-full max-w-[1180px] flex-1 px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
        <slot />
      </main>

      <footer
        class="mx-auto w-full max-w-[1180px] px-4 pb-6 text-xs text-ink-subtle sm:px-6 lg:px-8"
      >
        Nexora V1.0 · Phase 03 · <span class="font-mono">v{{ appVersion }}</span>
      </footer>
    </div>
  </div>
</template>
