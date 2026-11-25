<script setup lang="ts">
import { RouterLink, type RouteLocationRaw } from 'vue-router'

export interface PageTabItem {
  label: string
  to?: RouteLocationRaw
  active?: boolean
  count?: number | string
}

withDefaults(defineProps<{ items: PageTabItem[]; ariaLabel?: string }>(), {
  ariaLabel: '页面导航',
})
</script>

<template>
  <nav class="tabs" :aria-label="ariaLabel">
    <template v-for="item in items" :key="item.label">
      <RouterLink
        v-if="item.to && !item.active"
        :to="item.to"
        class="tab"
        :aria-current="item.active ? 'page' : undefined"
      >
        {{ item.label }}
        <span v-if="item.count !== undefined" class="tab-count">{{ item.count }}</span>
      </RouterLink>
      <span v-else class="tab tab-active" aria-current="page">
        {{ item.label }}
        <span v-if="item.count !== undefined" class="tab-count">{{ item.count }}</span>
      </span>
    </template>
  </nav>
</template>
