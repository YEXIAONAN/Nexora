<script setup lang="ts">
import AppIcon from './AppIcon.vue'
import type { IconName } from './icons'

withDefaults(defineProps<{ icon?: IconName; title: string; description?: string }>(), {
  icon: 'inbox',
  description: '',
})
</script>

<template>
  <div class="empty-state">
    <span class="empty-icon"><AppIcon :name="icon" :size="18" /></span>
    <p class="empty-title">{{ title }}</p>
    <p v-if="description" class="empty-description">{{ description }}</p>
    <div v-if="$slots.action" class="mt-4 flex flex-wrap justify-center gap-2">
      <slot name="action" />
    </div>
  </div>
</template>
