<script setup lang="ts">
import AppIcon from './AppIcon.vue'
import type { IconName } from './icons'

withDefaults(
  defineProps<{
    tone?: 'danger' | 'warning' | 'success' | 'info'
    title?: string
    message?: string
  }>(),
  { tone: 'info', title: '', message: '' },
)

const toneClass: Record<string, string> = {
  danger: 'alert-danger',
  warning: 'alert-warning',
  success: 'alert-success',
  info: 'alert-info',
}

const icon: Record<string, IconName> = {
  danger: 'alert',
  warning: 'alert',
  success: 'check',
  info: 'bell',
}
</script>

<template>
  <div
    class="alert"
    :class="toneClass[tone]"
    :role="tone === 'success' || tone === 'info' ? 'status' : 'alert'"
  >
    <div class="flex min-w-0 items-start gap-2.5">
      <AppIcon :name="icon[tone]" :size="16" class="mt-0.5 shrink-0" />
      <div class="min-w-0">
        <p v-if="title" class="font-medium">{{ title }}</p>
        <p v-if="message" class="text-body leading-6" :class="title ? 'mt-0.5' : ''">
          {{ message }}
        </p>
        <slot />
      </div>
    </div>
    <div v-if="$slots.action" class="flex shrink-0 items-center gap-2 sm:ml-4">
      <slot name="action" />
    </div>
  </div>
</template>
