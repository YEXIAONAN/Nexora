<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'

import AppIcon from '@/components/ui/AppIcon.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import { useSystemStore } from '@/stores/system'
import { formatDateTime, formatRelativeTime } from '@/utils/format'

const system = useSystemStore()
const autoRefresh = ref(false)
let timer: number | undefined

function startTimer(): void {
  stopTimer()
  timer = window.setInterval(() => void system.refresh(), 30_000)
}

function stopTimer(): void {
  if (timer !== undefined) {
    window.clearInterval(timer)
    timer = undefined
  }
}

watch(autoRefresh, (enabled) => {
  if (enabled) startTimer()
  else stopTimer()
})

onMounted(() => {
  void system.refresh()
})

onUnmounted(() => {
  stopTimer()
})

const checkRows = [
  { key: 'database' as const, label: 'PostgreSQL 连接', endpoint: '/readyz' },
  { key: 'schema' as const, label: 'Schema 迁移版本', endpoint: '/readyz' },
]
</script>

<template>
  <div class="space-y-6">
    <section class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <p class="text-xs font-medium tracking-[0.14em] text-ink-subtle uppercase">Diagnostics</p>
        <h2 class="mt-1 text-xl font-semibold tracking-tight text-ink">运行状态</h2>
        <p class="mt-1.5 max-w-2xl text-sm leading-6 text-ink-muted">
          用于本地开发与部署验收的诊断视图，展示服务端返回的原始检查结果。
        </p>
      </div>
      <div class="flex items-center gap-2">
        <label class="flex cursor-pointer items-center gap-2 text-xs text-ink-muted">
          <input v-model="autoRefresh" type="checkbox" class="h-3.5 w-3.5 accent-[var(--accent)]" />
          每 30 秒自动刷新
        </label>
        <button type="button" class="btn" :disabled="system.loading" @click="system.refresh()">
          <AppIcon name="refresh" :size="15" :class="system.loading ? 'animate-spin' : ''" />
          刷新
        </button>
      </div>
    </section>

    <div
      v-if="system.error"
      class="rounded-xl border border-danger/30 bg-danger-soft px-4 py-3 text-sm text-danger"
      role="alert"
    >
      {{ system.error }}
    </div>

    <section class="card">
      <div class="card-header">
        <div>
          <h3 class="text-sm font-semibold text-ink">就绪检查</h3>
          <p class="mt-0.5 text-xs text-ink-muted">
            最近检查：{{ formatRelativeTime(system.lastCheckedAt) }}
          </p>
        </div>
        <StatusBadge :label="system.statusLabel" :tone="system.statusTone" dot />
      </div>
      <ul class="divide-y divide-line">
        <li
          v-for="row in checkRows"
          :key="row.key"
          class="flex flex-col gap-2 px-5 py-4 sm:flex-row sm:items-center sm:justify-between"
        >
          <div>
            <p class="text-sm font-medium text-ink">{{ row.label }}</p>
            <p class="mt-0.5 font-mono text-xs text-ink-subtle">GET {{ row.endpoint }}</p>
          </div>
          <div class="flex items-center gap-3">
            <span class="text-xs text-ink-muted">
              {{
                system.loading
                  ? '检查中'
                  : (system.readiness?.checks[row.key]?.detail ??
                    (system.readiness?.checks[row.key]?.status === 'up'
                      ? '检查通过'
                      : '无返回数据'))
              }}
            </span>
            <StatusBadge
              :label="
                system.loading
                  ? '检查中'
                  : system.readiness?.checks[row.key]?.status === 'up'
                    ? '正常'
                    : system.readiness
                      ? '异常'
                      : '未知'
              "
              :tone="
                system.readiness?.checks[row.key]?.status === 'up'
                  ? 'success'
                  : system.readiness
                    ? 'danger'
                    : 'neutral'
              "
              dot
            />
          </div>
        </li>
      </ul>
      <div
        v-if="system.readiness"
        class="border-t border-line px-5 py-3 font-mono text-[11px] text-ink-subtle"
      >
        checked_at {{ formatDateTime(system.readiness.checked_at) }}
      </div>
    </section>

    <div class="grid gap-4 lg:grid-cols-2">
      <section class="card">
        <div class="card-header">
          <h3 class="text-sm font-semibold text-ink">实例信息</h3>
          <StatusBadge label="GET /api/v1/meta" />
        </div>
        <dl class="divide-y divide-line">
          <div class="flex items-baseline justify-between gap-4 px-5 py-3">
            <dt class="text-xs text-ink-subtle">产品</dt>
            <dd class="text-sm text-ink">{{ system.meta?.name ?? '—' }}</dd>
          </div>
          <div class="flex items-baseline justify-between gap-4 px-5 py-3">
            <dt class="text-xs text-ink-subtle">版本</dt>
            <dd class="font-mono text-xs text-ink">{{ system.meta?.version ?? '—' }}</dd>
          </div>
          <div class="flex items-baseline justify-between gap-4 px-5 py-3">
            <dt class="text-xs text-ink-subtle">环境</dt>
            <dd class="text-sm text-ink">{{ system.meta?.environment ?? '—' }}</dd>
          </div>
          <div class="flex items-baseline justify-between gap-4 px-5 py-3">
            <dt class="text-xs text-ink-subtle">提交</dt>
            <dd class="max-w-[70%] truncate font-mono text-xs text-ink">
              {{ system.meta?.commit ?? '—' }}
            </dd>
          </div>
          <div class="flex items-baseline justify-between gap-4 px-5 py-3">
            <dt class="text-xs text-ink-subtle">构建时间</dt>
            <dd class="text-xs text-ink">{{ formatDateTime(system.meta?.build_time) }}</dd>
          </div>
          <div class="flex items-start justify-between gap-4 px-5 py-3">
            <dt class="text-xs text-ink-subtle">安装实例</dt>
            <dd class="max-w-[70%] break-all font-mono text-xs text-ink">
              {{ system.meta?.instance_id ?? '—' }}
            </dd>
          </div>
        </dl>
      </section>

      <section class="card">
        <div class="card-header">
          <h3 class="text-sm font-semibold text-ink">接口约定</h3>
        </div>
        <ul class="divide-y divide-line">
          <li class="px-5 py-3.5">
            <div class="flex items-center justify-between gap-3">
              <code class="font-mono text-xs text-ink">GET /healthz</code>
              <span class="text-xs text-ink-subtle">进程存活</span>
            </div>
            <p class="mt-1 text-xs leading-5 text-ink-muted">
              不访问数据库；容器编排用它判断是否需要重启进程。
            </p>
          </li>
          <li class="px-5 py-3.5">
            <div class="flex items-center justify-between gap-3">
              <code class="font-mono text-xs text-ink">GET /readyz</code>
              <span class="text-xs text-ink-subtle">依赖就绪</span>
            </div>
            <p class="mt-1 text-xs leading-5 text-ink-muted">
              检查数据库连接与迁移版本；未就绪时返回 HTTP 503。
            </p>
          </li>
          <li class="px-5 py-3.5">
            <div class="flex items-center justify-between gap-3">
              <code class="font-mono text-xs text-ink">GET /api/v1/meta</code>
              <span class="text-xs text-ink-subtle">实例元数据</span>
            </div>
            <p class="mt-1 text-xs leading-5 text-ink-muted">
              返回构建信息与安装实例 ID；安装 ID 来自 PostgreSQL。
            </p>
          </li>
        </ul>
        <div class="border-t border-line px-5 py-3">
          <p class="text-xs leading-5 text-ink-subtle">
            完整契约见仓库 <code class="font-mono">docs/api/openapi.yaml</code>，前端类型由它生成。
          </p>
        </div>
      </section>
    </div>
  </div>
</template>
