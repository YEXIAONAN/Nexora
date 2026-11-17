<script setup lang="ts">
import { computed, onMounted } from 'vue'

import AppIcon from '@/components/ui/AppIcon.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import { useSystemStore } from '@/stores/system'
import { formatDateTime, formatRelativeTime } from '@/utils/format'

const system = useSystemStore()

onMounted(() => {
  if (!system.meta && !system.loading) void system.refresh()
})

const apiTone = computed(() =>
  system.apiState === 'down' ? 'danger' : system.apiState === 'up' ? 'success' : 'neutral',
)
const apiLabel = computed(() => {
  const labels = { loading: '检查中', up: '正常', down: '异常', unknown: '未知' }
  return labels[system.apiState]
})

const databaseStatus = computed(() =>
  system.loading ? 'loading' : (system.readiness?.checks.database.status ?? 'unknown'),
)
const databaseLabel = computed(() =>
  databaseStatus.value === 'loading'
    ? '检查中'
    : databaseStatus.value === 'up'
      ? '正常'
      : databaseStatus.value === 'down'
        ? '异常'
        : '未知',
)

const schemaStatus = computed(() =>
  system.loading ? 'loading' : (system.readiness?.checks.schema.status ?? 'unknown'),
)

type CheckState = 'up' | 'down' | 'unknown' | 'loading'

const checks = computed<{ name: string; detail: string; state: CheckState }[]>(() => [
  {
    name: 'API 进程',
    detail: system.meta
      ? `已连接 · ${system.meta.name} ${system.meta.version}`
      : system.loading
        ? '检查中'
        : system.metaError
          ? '检查失败'
          : '尚未连接',
    state: system.apiState,
  },
  {
    name: 'PostgreSQL 连接',
    detail:
      system.readiness?.checks.database.status === 'up'
        ? '连接池可用'
        : system.readiness
          ? '连接不可用'
          : system.loading
            ? '检查中'
            : system.readinessError
              ? '无法获取当前状态'
              : '尚未检查',
    state: databaseStatus.value,
  },
  {
    name: 'Schema 迁移',
    detail:
      system.readiness?.checks.schema.detail ??
      (system.loading ? '检查中' : system.readinessError ? '无法获取当前状态' : '尚未检查'),
    state: schemaStatus.value,
  },
])

function checkTone(state: CheckState): 'success' | 'danger' | 'neutral' {
  if (state === 'up') return 'success'
  if (state === 'down') return 'danger'
  return 'neutral'
}

function checkLabel(state: CheckState): string {
  if (state === 'loading') return '检查中'
  if (state === 'up') return '通过'
  if (state === 'down') return '失败'
  return '未知'
}
</script>

<template>
  <div class="space-y-6">
    <section class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <p class="text-xs font-medium tracking-[0.14em] text-ink-subtle uppercase">Phase 0</p>
        <h2 class="mt-1 text-xl font-semibold tracking-tight text-ink">系统总览</h2>
        <p class="mt-1.5 max-w-2xl text-sm leading-6 text-ink-muted">
          以下状态来自真实 API 与 PostgreSQL 检查，不使用前端模拟数据。
        </p>
      </div>
      <button
        type="button"
        class="btn shrink-0"
        :disabled="system.loading"
        @click="system.refresh()"
      >
        <AppIcon name="refresh" :size="15" :class="system.loading ? 'animate-spin' : ''" />
        {{ system.loading ? '检查中' : '重新检查' }}
      </button>
    </section>

    <div
      v-if="system.error"
      class="flex flex-col gap-3 rounded-xl border border-danger/30 bg-danger-soft px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
      role="alert"
    >
      <p class="text-sm text-danger">
        {{ system.error }}。请确认 API 与数据库已启动，并检查
        <code class="font-mono text-xs">/readyz</code>。
      </p>
      <button type="button" class="btn shrink-0" @click="system.refresh()">重试</button>
    </div>

    <div
      v-else-if="system.meta && !system.isReady"
      class="rounded-xl border border-warning/30 bg-warning-soft px-4 py-3 text-sm text-warning"
      role="status"
    >
      API 可达，但数据库连接或 Schema 迁移检查未通过，当前实例不应接收业务流量。
    </div>

    <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
      <article class="card card-body">
        <div class="flex items-start justify-between gap-3">
          <div class="flex items-center gap-2 text-sm font-medium text-ink">
            <AppIcon name="server" :size="17" class="text-ink-subtle" />
            API 服务
          </div>
          <StatusBadge :label="apiLabel" :tone="apiTone" dot />
        </div>
        <p class="mt-4 font-mono text-2xl font-semibold tracking-tight text-ink">
          {{ system.meta?.version ?? '—' }}
        </p>
        <p class="mt-1 text-xs text-ink-muted">
          {{ system.meta ? `环境 ${system.meta.environment}` : '等待首次健康检查' }}
        </p>
      </article>

      <article class="card card-body">
        <div class="flex items-start justify-between gap-3">
          <div class="flex items-center gap-2 text-sm font-medium text-ink">
            <AppIcon name="database" :size="17" class="text-ink-subtle" />
            数据库
          </div>
          <StatusBadge :label="databaseLabel" :tone="checkTone(databaseStatus)" dot />
        </div>
        <p class="mt-4 font-mono text-2xl font-semibold tracking-tight text-ink">
          {{ system.readiness?.checks.schema.detail ?? '—' }}
        </p>
        <p class="mt-1 text-xs text-ink-muted">PostgreSQL · UTC 存储</p>
      </article>

      <article class="card card-body sm:col-span-2 xl:col-span-1">
        <div class="flex items-start justify-between gap-3">
          <div class="flex items-center gap-2 text-sm font-medium text-ink">
            <AppIcon name="shield" :size="17" class="text-ink-subtle" />
            整体就绪
          </div>
          <StatusBadge :label="system.statusLabel" :tone="system.statusTone" dot />
        </div>
        <p class="mt-4 font-mono text-2xl font-semibold tracking-tight text-ink">
          {{ system.status }}
        </p>
        <p class="mt-1 text-xs text-ink-muted">
          {{ formatRelativeTime(system.lastCheckedAt) }}检查
        </p>
      </article>
    </div>

    <div class="grid gap-4 lg:grid-cols-5">
      <section class="card lg:col-span-3">
        <div class="card-header">
          <div>
            <h3 class="text-sm font-semibold text-ink">运行检查</h3>
            <p class="mt-0.5 text-xs text-ink-muted">来自 /readyz 与 /api/v1/meta</p>
          </div>
          <span class="font-mono text-xs text-ink-subtle">
            {{ formatDateTime(system.lastCheckedAt) }}
          </span>
        </div>
        <ul class="divide-y divide-line">
          <li
            v-for="check in checks"
            :key="check.name"
            class="flex items-center justify-between gap-4 px-5 py-3.5"
          >
            <div class="min-w-0">
              <p class="text-sm font-medium text-ink">{{ check.name }}</p>
              <p class="mt-0.5 truncate text-xs text-ink-muted">{{ check.detail }}</p>
            </div>
            <StatusBadge :label="checkLabel(check.state)" :tone="checkTone(check.state)" dot />
          </li>
        </ul>
      </section>

      <section class="card lg:col-span-2">
        <div class="card-header">
          <h3 class="text-sm font-semibold text-ink">构建信息</h3>
          <StatusBadge label="只读" />
        </div>
        <dl class="space-y-3 px-5 py-4 text-sm">
          <div class="flex items-baseline justify-between gap-4">
            <dt class="text-xs text-ink-subtle">版本</dt>
            <dd class="font-mono text-xs text-ink">{{ system.meta?.version ?? '—' }}</dd>
          </div>
          <div class="flex items-baseline justify-between gap-4">
            <dt class="text-xs text-ink-subtle">提交</dt>
            <dd class="max-w-[65%] truncate font-mono text-xs text-ink">
              {{ system.meta?.commit ?? '—' }}
            </dd>
          </div>
          <div class="flex items-baseline justify-between gap-4">
            <dt class="text-xs text-ink-subtle">构建时间</dt>
            <dd class="text-xs text-ink">{{ formatDateTime(system.meta?.build_time) }}</dd>
          </div>
          <div class="flex items-baseline justify-between gap-4">
            <dt class="text-xs text-ink-subtle">实例 ID</dt>
            <dd class="max-w-[65%] truncate font-mono text-xs text-ink">
              {{ system.meta?.instance_id ?? '—' }}
            </dd>
          </div>
          <div class="flex items-baseline justify-between gap-4">
            <dt class="text-xs text-ink-subtle">服务端时间</dt>
            <dd class="text-xs text-ink">{{ formatDateTime(system.meta?.server_time) }}</dd>
          </div>
        </dl>
      </section>
    </div>

    <section class="card">
      <div class="card-header">
        <div>
          <h3 class="text-sm font-semibold text-ink">本阶段范围</h3>
          <p class="mt-0.5 text-xs text-ink-muted">Phase 0 只交付可验证的工程基线</p>
        </div>
        <StatusBadge label="未包含业务模块" tone="warning" />
      </div>
      <div class="grid gap-6 px-5 py-4 sm:grid-cols-2">
        <div>
          <p class="text-xs font-medium tracking-wider text-ink-subtle uppercase">已交付</p>
          <ul class="mt-2 space-y-2 text-sm text-ink-muted">
            <li class="flex gap-2"><span class="text-accent">·</span>Go API、配置校验与优雅退出</li>
            <li class="flex gap-2">
              <span class="text-accent">·</span>PostgreSQL 连接池与版本化迁移
            </li>
            <li class="flex gap-2">
              <span class="text-accent">·</span>请求 ID、结构化日志与统一错误响应
            </li>
            <li class="flex gap-2">
              <span class="text-accent">·</span>Vue 3 工作台外壳与深浅色主题
            </li>
            <li class="flex gap-2">
              <span class="text-accent">·</span>Docker Compose、CI 与备份脚本
            </li>
          </ul>
        </div>
        <div>
          <p class="text-xs font-medium tracking-wider text-ink-subtle uppercase">尚未实现</p>
          <p class="mt-2 text-sm leading-6 text-ink-muted">
            账号认证、团队与权限、项目与三级任务、成果验收、贡献核算、奖金分配、通知与导出。这些模块将在
            Phase 1 起按设计逐步实现，不会以静态页面或模拟数据代替。
          </p>
        </div>
      </div>
    </section>
  </div>
</template>
