<script setup lang="ts">
import { onBeforeUnmount, watch } from 'vue'
import AppIcon from './AppIcon.vue'

const props = withDefaults(
  defineProps<{
    open: boolean
    title: string
    eyebrow?: string
    width?: string
  }>(),
  { eyebrow: '', width: 'max-w-[620px]' },
)
const emit = defineEmits<{ close: [] }>()

function onKey(event: KeyboardEvent): void {
  if (event.key === 'Escape' && props.open) emit('close')
}

watch(
  () => props.open,
  (open) => {
    document.body.style.overflow = open ? 'hidden' : ''
    if (open) window.addEventListener('keydown', onKey)
    else window.removeEventListener('keydown', onKey)
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  document.body.style.overflow = ''
  window.removeEventListener('keydown', onKey)
})
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition-opacity duration-150"
      enter-from-class="opacity-0"
      leave-active-class="transition-opacity duration-100"
      leave-to-class="opacity-0"
    >
      <div
        v-if="open"
        class="overlay-backdrop justify-end"
        role="dialog"
        aria-modal="true"
        :aria-label="title"
      >
        <div class="absolute inset-0 bg-black/30" aria-hidden="true" @click="emit('close')" />
        <aside class="drawer-panel" :class="width">
          <header class="flex items-start justify-between gap-3 border-b border-line px-5 py-4">
            <div class="min-w-0">
              <p v-if="eyebrow" class="eyebrow">{{ eyebrow }}</p>
              <h2 class="mt-1 text-title font-semibold text-ink">{{ title }}</h2>
            </div>
            <button
              type="button"
              class="btn btn-ghost btn-icon-sm"
              aria-label="关闭"
              @click="emit('close')"
            >
              <AppIcon name="close" :size="15" />
            </button>
          </header>
          <div class="min-h-0 flex-1 overflow-y-auto">
            <slot />
          </div>
        </aside>
      </div>
    </Transition>
  </Teleport>
</template>
