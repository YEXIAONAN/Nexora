<script setup lang="ts">
import AppIcon from '@/components/ui/AppIcon.vue'
import { useUiStore } from '@/stores/ui'

const ui = useUiStore()

const toneClasses: Record<string, string> = {
  success: 'border-accent/30 bg-surface text-ink',
  error: 'border-danger/30 bg-surface text-ink',
  info: 'border-line bg-surface text-ink',
}

const iconClasses: Record<string, string> = {
  success: 'text-accent-strong',
  error: 'text-danger',
  info: 'text-ink-muted',
}
</script>

<template>
  <div
    class="pointer-events-none fixed inset-x-0 bottom-4 z-[60] flex flex-col items-center gap-2 px-4 sm:bottom-6"
    aria-live="polite"
  >
    <TransitionGroup
      enter-active-class="transition duration-150"
      enter-from-class="translate-y-2 opacity-0"
      leave-active-class="transition duration-150"
      leave-to-class="translate-y-1 opacity-0"
    >
      <div
        v-for="toast in ui.toasts"
        :key="toast.id"
        class="pointer-events-auto flex w-full max-w-md items-start gap-2.5 rounded-xl border px-4 py-3 text-sm shadow-lg"
        :class="toneClasses[toast.tone]"
        role="status"
      >
        <AppIcon
          :name="toast.tone === 'success' ? 'check' : toast.tone === 'error' ? 'alert' : 'bell'"
          :size="16"
          class="mt-0.5 shrink-0"
          :class="iconClasses[toast.tone]"
        />
        <span class="min-w-0 grow leading-5">{{ toast.text }}</span>
        <button
          type="button"
          class="shrink-0 rounded p-0.5 text-ink-subtle hover:text-ink"
          aria-label="关闭提示"
          @click="ui.dismiss(toast.id)"
        >
          <AppIcon name="close" :size="14" />
        </button>
      </div>
    </TransitionGroup>
  </div>
</template>
