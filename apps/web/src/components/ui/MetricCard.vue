<script setup lang="ts">
import { RouterLink } from 'vue-router'
import AppIcon from './AppIcon.vue'
import type { IconName } from './icons'

withDefaults(
  defineProps<{
    label: string
    value: string | number
    hint?: string
    icon?: IconName
    to?: string
    tone?: 'default' | 'accent' | 'warning' | 'danger'
  }>(),
  { hint: '', icon: 'activity', to: '', tone: 'default' },
)

const valueTone: Record<string, string> = {
  default: 'text-ink',
  accent: 'text-ink',
  warning: 'text-warning',
  danger: 'text-danger',
}
</script>

<template>
  <component
    :is="to ? RouterLink : 'div'"
    :to="to || undefined"
    class="metric-card"
    :class="to ? 'cursor-pointer' : ''"
  >
    <div class="metric">
      <p class="metric-label">
        <span>{{ label }}</span>
        <AppIcon v-if="icon" :name="icon" :size="15" class="text-ink-subtle" />
      </p>
      <p class="metric-value" :class="valueTone[tone]">{{ value }}</p>
      <p v-if="hint" class="metric-meta">{{ hint }}</p>
    </div>
  </component>
</template>
