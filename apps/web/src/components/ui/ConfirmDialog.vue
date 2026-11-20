<script setup lang="ts">
import { onBeforeUnmount, onMounted } from 'vue'

import { useUiStore } from '@/stores/ui'

const ui = useUiStore()

function onKey(event: KeyboardEvent): void {
  if (event.key === 'Escape' && ui.confirmState) ui.resolveConfirm(false)
}

onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition duration-150"
      enter-from-class="opacity-0"
      leave-active-class="transition duration-100"
      leave-to-class="opacity-0"
    >
      <div
        v-if="ui.confirmState"
        class="fixed inset-0 z-[70] flex items-center justify-center bg-black/35 px-4"
        role="dialog"
        aria-modal="true"
        @click.self="ui.resolveConfirm(false)"
      >
        <div class="w-full max-w-md rounded-xl border border-line bg-surface p-5 shadow-xl">
          <h2 class="text-base font-semibold text-ink">{{ ui.confirmState.title }}</h2>
          <p class="mt-2 text-sm leading-6 text-ink-muted">{{ ui.confirmState.message }}</p>
          <div class="mt-5 flex justify-end gap-2">
            <button type="button" class="btn" @click="ui.resolveConfirm(false)">取消</button>
            <button
              type="button"
              class="btn"
              :class="ui.confirmState.tone === 'danger' ? 'btn-danger' : 'btn-primary'"
              @click="ui.resolveConfirm(true)"
            >
              {{ ui.confirmState.confirmLabel }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
