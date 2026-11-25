<script setup lang="ts">
import { computed } from 'vue'

type Tone = 'neutral' | 'success' | 'danger' | 'warning' | 'info'

const props = withDefaults(
  defineProps<{
    label: string
    tone?: Tone
    dot?: boolean
  }>(),
  { tone: 'neutral', dot: false },
)

const toneClasses: Record<Tone, string> = {
  neutral: 'border-line bg-surface-muted text-ink-muted',
  success: 'border-accent/30 bg-accent-soft text-accent-strong',
  danger: 'border-danger/30 bg-danger-soft text-danger',
  warning: 'border-warning/30 bg-warning-soft text-warning',
  info: 'border-info/25 bg-info-soft text-info',
}

const dotClasses: Record<Tone, string> = {
  neutral: 'bg-ink-subtle',
  success: 'bg-accent',
  danger: 'bg-danger',
  warning: 'bg-warning',
  info: 'bg-info',
}

const classes = computed(() => toneClasses[props.tone])
const dotClass = computed(() => dotClasses[props.tone])
</script>

<template>
  <span
    class="inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-medium"
    :class="classes"
  >
    <span v-if="dot" class="h-1.5 w-1.5 rounded-full" :class="dotClass" aria-hidden="true" />
    {{ label }}
  </span>
</template>
