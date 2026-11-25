<script setup lang="ts">
export interface SegmentOption {
  value: string
  label: string
  count?: number
}

defineProps<{ modelValue: string; options: SegmentOption[]; ariaLabel?: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
</script>

<template>
  <div class="tabs w-fit" role="tablist" :aria-label="ariaLabel ?? '筛选'">
    <button
      v-for="option in options"
      :key="option.value"
      type="button"
      role="tab"
      class="tab"
      :class="modelValue === option.value ? 'tab-active' : ''"
      :aria-selected="modelValue === option.value"
      @click="emit('update:modelValue', option.value)"
    >
      {{ option.label }}
      <span v-if="option.count !== undefined" class="tab-count">{{ option.count }}</span>
    </button>
  </div>
</template>
