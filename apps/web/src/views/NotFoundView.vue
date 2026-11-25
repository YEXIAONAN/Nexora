<script setup lang="ts">
import { RouterLink } from 'vue-router'

import AppIcon from '@/components/ui/AppIcon.vue'
</script>

<template>
  <div class="flex min-h-[60vh] items-center justify-center">
    <div class="card w-full max-w-md px-8 py-10 text-center">
      <p class="font-mono text-meta tracking-[0.2em] text-ink-subtle">404</p>
      <h2 class="mt-2 text-heading font-semibold text-ink">页面不存在</h2>
      <p class="mt-2 text-body leading-6 text-ink-muted">
        你访问的地址没有对应的页面。返回工作台继续处理团队任务与项目。
      </p>
      <RouterLink to="/" class="btn btn-primary mt-5">
        返回工作台
        <AppIcon name="arrow-up-right" :size="15" />
      </RouterLink>
    </div>
  </div>
</template>
