<script setup lang="ts">
import { RouterLink } from 'vue-router'

export interface PageCrumb {
  label: string
  to?: string
}

withDefaults(
  defineProps<{
    eyebrow?: string
    title: string
    description?: string
    crumbs?: PageCrumb[]
  }>(),
  { eyebrow: '', description: '', crumbs: () => [] },
)
</script>

<template>
  <div class="page-header">
    <div class="page-heading">
      <nav v-if="crumbs.length" class="breadcrumbs" aria-label="面包屑">
        <template v-for="(crumb, index) in crumbs" :key="`${crumb.label}-${index}`">
          <RouterLink v-if="crumb.to" :to="crumb.to" class="breadcrumb-link">
            {{ crumb.label }}
          </RouterLink>
          <span v-else class="text-ink-muted">{{ crumb.label }}</span>
          <span v-if="index < crumbs.length - 1" class="breadcrumb-separator" aria-hidden="true"
            >/</span
          >
        </template>
      </nav>
      <p v-if="eyebrow" class="eyebrow" :class="crumbs.length ? 'mt-2' : ''">{{ eyebrow }}</p>
      <h2 class="page-title" :class="eyebrow || crumbs.length ? 'mt-1' : ''">{{ title }}</h2>
      <p v-if="description" class="page-subtitle">{{ description }}</p>
    </div>
    <div v-if="$slots.actions" class="page-actions">
      <slot name="actions" />
    </div>
  </div>
</template>
