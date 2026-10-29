<script setup lang="ts">
import { RouterLink } from 'vue-router'

import AppIcon from '@/components/ui/AppIcon.vue'
</script>

<template>
  <div class="flex min-h-[50vh] items-center justify-center">
    <div class="max-w-md text-center">
      <p class="font-mono text-xs tracking-[0.2em] text-ink-subtle">404</p>
      <h2 class="mt-2 text-xl font-semibold text-ink">页面不存在</h2>
      <p class="mt-2 text-sm leading-6 text-ink-muted">
        你访问的地址没有对应的页面。返回系统总览查看当前实例状态。
      </p>
      <RouterLink to="/" class="btn btn-primary mt-5">
        返回系统总览
        <AppIcon name="arrow-up-right" :size="15" />
      </RouterLink>
    </div>
  </div>
</template>
