<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import * as bonusAPI from '@/api/bonus'
import { ApiError } from '@/api/client'
import { getProject, type Project } from '@/api/projects'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import {
  bonusRoundStatusLabel,
  bonusRoundStatusTone,
  bonusVersionStatusLabel,
} from '@/components/bonus/bonusPresentation'
import { useTeamsStore } from '@/stores/teams'
import { formatDateTime } from '@/utils/format'

const route = useRoute(),
  teams = useTeamsStore()
const teamId = computed(() => String(route.params.teamId ?? ''))
const projectId = computed(() => String(route.params.projectId ?? ''))
const roundId = computed(() => String(route.params.roundId ?? ''))
const project = ref<Project | null>(null),
  round = ref<bonusAPI.BonusRoundDetail | null>(null)
const loading = ref(false),
  error = ref(''),
  notFound = ref(false),
  confirmed = ref('')
let generation = 0
const contextKey = computed(() => `${teamId.value}/${projectId.value}/${roundId.value}`)
const ready = computed(
  () => confirmed.value === contextKey.value && !!round.value && !loading.value,
)
const versions = computed(() =>
  [...(round.value?.versions ?? [])].sort((a, b) => b.version - a.version),
)
const effectiveVersion = computed(() => round.value?.effective_confirmed_version ?? null)

async function load() {
  const team = teamId.value,
    projectKey = projectId.value,
    roundKey = roundId.value,
    key = `${team}/${projectKey}/${roundKey}`,
    gen = ++generation
  confirmed.value = ''
  loading.value = true
  error.value = ''
  notFound.value = false
  round.value = null
  project.value = null
  try {
    const [p, detail] = await Promise.all([
      getProject(team, projectKey),
      bonusAPI.getBonusRound(team, projectKey, roundKey),
    ])
    if (generation !== gen || key !== contextKey.value) return
    project.value = p
    round.value = detail
    teams.selectTeam(team)
    confirmed.value = key
  } catch (e) {
    if (generation !== gen || key !== contextKey.value) return
    if (e instanceof ApiError && [403, 404].includes(e.status ?? 0)) notFound.value = true
    else error.value = e instanceof ApiError ? e.message : '奖金批次加载失败'
  } finally {
    if (generation === gen && key === contextKey.value) loading.value = false
  }
}

watch(
  [teamId, projectId, roundId],
  () => {
    void load()
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  generation++
})
</script>

<template>
  <div class="space-y-6">
    <div class="page-header">
      <div>
        <RouterLink
          :to="{ name: 'bonus-rounds', params: { teamId, projectId } }"
          class="text-xs text-ink-subtle hover:text-ink"
          >← 奖金批次列表</RouterLink
        >
        <div class="mt-2 flex flex-wrap items-center gap-3">
          <h2 class="page-title">{{ round?.funding_label || '奖金批次' }}</h2>
          <StatusBadge
            v-if="round"
            :label="bonusRoundStatusLabel(round.status)"
            :tone="bonusRoundStatusTone(round.status)"
            dot
          />
        </div>
        <p class="page-subtitle">
          {{ project?.name || '项目' }}
          的独立资金批次。版本历史不可修改，正式核定版本是当前唯一有效结果。
        </p>
      </div>
      <button class="btn" :disabled="loading" @click="load">刷新</button>
    </div>

    <nav
      class="flex gap-1 overflow-x-auto rounded-lg border border-line bg-surface p-1"
      aria-label="项目导航"
    >
      <RouterLink
        :to="{ name: 'project-detail', params: { teamId, projectId } }"
        class="rounded-md px-3 py-1.5 text-xs font-medium text-ink-muted transition-colors hover:text-ink"
        >项目工作区</RouterLink
      >
      <RouterLink
        :to="{ name: 'bonus-rounds', params: { teamId, projectId } }"
        class="rounded-md px-3 py-1.5 text-xs font-medium text-ink-muted transition-colors hover:text-ink"
        >奖金批次</RouterLink
      >
      <span class="rounded-md bg-accent-soft px-3 py-1.5 text-xs font-medium text-accent-strong"
        >批次详情</span
      >
      <RouterLink
        :to="{ name: 'my-bonus-results', params: { teamId } }"
        class="rounded-md px-3 py-1.5 text-xs font-medium text-ink-muted transition-colors hover:text-ink"
        >我的奖金</RouterLink
      >
    </nav>

    <p v-if="notFound" role="alert" class="card empty-state">
      <strong class="text-sm text-ink">无法查看该奖金批次</strong>
      <span class="mt-2 max-w-md text-sm text-ink-muted"
        >该批次可能不存在，或它不属于当前 Team 与
        Project，或你不是项目成员。奖金数据不会因平台管理员身份自动开放。</span
      >
      <RouterLink
        :to="{ name: 'bonus-rounds', params: { teamId, projectId } }"
        class="btn btn-sm mt-4"
        >返回批次列表</RouterLink
      >
    </p>
    <p v-else-if="error" role="alert" class="rounded-lg bg-danger-soft p-4 text-sm text-danger">
      {{ error }}
    </p>
    <p v-else-if="loading" role="status" class="card p-8 text-sm text-ink-muted">
      正在加载批次详情…
    </p>

    <template v-else-if="ready && round">
      <section class="grid gap-5 lg:grid-cols-3">
        <div class="card lg:col-span-2">
          <div class="card-header">
            <h3 class="section-title">批次信息</h3>
            <span class="badge">{{ round.funding_key }}</span>
          </div>
          <dl class="divide-y divide-line">
            <div class="flex flex-wrap items-center justify-between gap-2 px-5 py-3">
              <dt class="text-xs text-ink-subtle">资金来源</dt>
              <dd class="max-w-xl text-right text-sm text-ink">{{ round.funding_source }}</dd>
            </div>
            <div class="flex items-center justify-between px-5 py-3">
              <dt class="text-xs text-ink-subtle">批次状态</dt>
              <dd>
                <StatusBadge
                  :label="bonusRoundStatusLabel(round.status)"
                  :tone="bonusRoundStatusTone(round.status)"
                  dot
                />
              </dd>
            </div>
            <div class="flex items-center justify-between px-5 py-3">
              <dt class="text-xs text-ink-subtle">修订次数</dt>
              <dd class="text-sm text-ink">{{ round.revision }}</dd>
            </div>
            <div class="flex items-center justify-between px-5 py-3">
              <dt class="text-xs text-ink-subtle">创建时间</dt>
              <dd class="text-sm text-ink">{{ formatDateTime(round.created_at) }}</dd>
            </div>
          </dl>
        </div>

        <div class="card">
          <div class="card-header"><h3 class="section-title">核定状态</h3></div>
          <div class="space-y-4 px-5 py-4">
            <div>
              <p class="field-label">当前方案版本</p>
              <p class="field-value">
                {{ round.latest_version ? `v${round.latest_version.version}` : '尚无版本' }}
              </p>
              <p v-if="round.latest_version" class="mt-1 text-[11px] text-ink-subtle">
                {{ bonusVersionStatusLabel(round.latest_version.status) }} ·
                {{ formatDateTime(round.latest_version.created_at) }}
              </p>
            </div>
            <div class="divider pt-3">
              <p class="field-label">正式核定版本</p>
              <p v-if="effectiveVersion" class="field-value">
                v{{ effectiveVersion.version }}
                <span class="ml-1 text-[11px] font-normal text-accent-strong">当前有效</span>
              </p>
              <p v-else class="field-value text-ink-muted">尚未核定</p>
              <p v-if="effectiveVersion" class="mt-1 text-[11px] text-ink-subtle">
                核定于 {{ formatDateTime(effectiveVersion.confirmed_at) }}
              </p>
            </div>
            <p class="rounded-lg bg-surface-muted px-3 py-2.5 text-[11px] leading-5 text-ink-muted">
              本阶段只提供只读元数据。预算、成员金额与付款状态不在项目读取权限内展示。
            </p>
          </div>
        </div>
      </section>

      <section class="card">
        <div class="card-header">
          <div>
            <h3 class="section-title">版本历史</h3>
            <p class="mt-1 text-xs text-ink-muted">
              每个版本不可修改；修订会创建新版本，旧正式结果在被重新核定前保持有效。
            </p>
          </div>
          <span class="badge">{{ versions.length }} 个版本</span>
        </div>
        <ol v-if="versions.length" class="divide-y divide-line">
          <li
            v-for="version in versions"
            :key="version.version"
            class="flex flex-wrap items-start justify-between gap-3 px-5 py-4"
          >
            <div class="flex min-w-0 items-start gap-3">
              <span
                class="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full border text-xs font-semibold"
                :class="
                  version.effective
                    ? 'border-accent/40 bg-accent-soft text-accent-strong'
                    : 'border-line bg-surface-muted text-ink-muted'
                "
                >v{{ version.version }}</span
              >
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="text-sm font-medium text-ink">版本 {{ version.version }}</span>
                  <StatusBadge
                    :label="bonusVersionStatusLabel(version.status)"
                    :tone="
                      version.effective
                        ? 'success'
                        : version.status === 'abandoned' || version.status === 'overturned'
                          ? 'danger'
                          : version.status === 'pending_review'
                            ? 'warning'
                            : 'neutral'
                    "
                  />
                  <span v-if="version.effective" class="text-[11px] font-medium text-accent-strong"
                    >当前有效</span
                  >
                </div>
                <p class="mt-1 text-xs text-ink-subtle">
                  创建 {{ formatDateTime(version.created_at) }}
                  <template v-if="version.submitted_at">
                    · 提交 {{ formatDateTime(version.submitted_at) }}
                  </template>
                  <template v-if="version.confirmed_at">
                    · 核定 {{ formatDateTime(version.confirmed_at) }}
                  </template>
                </p>
              </div>
            </div>
            <span v-if="version.effective" class="badge">正式结果来源</span>
          </li>
        </ol>
        <div v-else class="empty-state">
          <strong class="text-sm text-ink">尚无方案版本</strong>
          <p class="mt-2 text-sm text-ink-muted">批次已建立，但还没有提交任何版本方案。</p>
        </div>
      </section>
    </template>
  </div>
</template>
