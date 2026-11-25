<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import * as bonusAPI from '@/api/bonus'
import { ApiError } from '@/api/client'
import { getProject, type Project } from '@/api/projects'
import AlertBanner from '@/components/ui/AlertBanner.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import PageTabs from '@/components/ui/PageTabs.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import BonusRoundWorkspace from '@/components/bonus/BonusRoundWorkspace.vue'
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
let controller: AbortController | null = null
const metadataError = ref('')
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
  controller?.abort()
  controller = new AbortController()
  const signal = controller.signal
  metadataError.value = ''
  confirmed.value = ''
  loading.value = true
  error.value = ''
  notFound.value = false
  round.value = null
  project.value = null
  try {
    const [p, detail] = await Promise.all([
      getProject(team, projectKey, signal),
      bonusAPI.getBonusRound(team, projectKey, roundKey, signal),
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

async function refreshMetadata() {
  const gen = generation,
    key = contextKey.value
  try {
    const detail = await bonusAPI.getBonusRound(
      teamId.value,
      projectId.value,
      roundId.value,
      controller?.signal,
    )
    if (gen === generation && key === contextKey.value) {
      round.value = detail
      metadataError.value = ''
    }
  } catch (e) {
    if (gen === generation && key === contextKey.value)
      metadataError.value = e instanceof ApiError ? e.message : '版本历史刷新失败，请刷新页面'
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
  controller?.abort()
})
</script>

<template>
  <div class="page-stack">
    <PageHeader
      eyebrow="项目资金"
      :title="round?.funding_label || '奖金批次'"
      :description="`${project?.name || '项目'}的独立资金批次。人工方案、独立核定、发布与模拟发放分步办理，修订保留原正式结果。`"
      :crumbs="[
        { label: '项目', to: `/teams/${teamId}/projects` },
        { label: project?.name || '项目', to: `/teams/${teamId}/projects/${projectId}` },
        { label: '奖金管理', to: `/teams/${teamId}/projects/${projectId}/bonus-rounds` },
        { label: round?.funding_label || '批次详情' },
      ]"
    >
      <template #actions>
        <StatusBadge
          v-if="round"
          :label="bonusRoundStatusLabel(round.status)"
          :tone="bonusRoundStatusTone(round.status)"
          dot
        />
        <button class="btn" :disabled="loading" @click="load">刷新</button>
      </template>
    </PageHeader>

    <PageTabs
      aria-label="项目导航"
      :items="[
        {
          label: '项目工作区',
          to: { name: 'project-detail', params: { teamId, projectId } },
        },
        {
          label: '贡献核算',
          to: `/teams/${teamId}/projects/${projectId}/contributions`,
        },
        {
          label: '奖金管理',
          to: `/teams/${teamId}/projects/${projectId}/bonus-rounds`,
        },
        { label: '批次详情', active: true },
        { label: '我的奖金', to: `/teams/${teamId}/bonus-results` },
      ]"
    />

    <div v-if="notFound" class="card">
      <AlertBanner
        tone="warning"
        title="无法查看该奖金批次"
        message="该批次可能不存在，或它不属于当前 Team 与 Project，或你不是项目成员。奖金数据不会因平台管理员身份自动开放。"
      >
        <template #action>
          <RouterLink
            :to="{ name: 'bonus-rounds', params: { teamId, projectId } }"
            class="btn btn-sm"
            >返回奖金管理</RouterLink
          >
        </template>
      </AlertBanner>
    </div>
    <AlertBanner v-else-if="error" tone="danger" :message="error" />
    <div v-else-if="loading" class="card space-y-2 p-4">
      <div v-for="i in 3" :key="i" class="skeleton h-20" />
    </div>

    <template v-else-if="ready && round">
      <section class="grid grid-cols-1 gap-5 lg:grid-cols-3">
        <div class="card lg:col-span-2">
          <div class="card-header">
            <h3 class="card-title">批次信息</h3>
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
          <div class="card-header"><h3 class="card-title">核定状态</h3></div>
          <div class="space-y-4 px-5 py-4">
            <div>
              <p class="field-label">当前方案版本</p>
              <p class="field-value">
                {{ round.latest_version ? `v${round.latest_version.version}` : '尚无版本' }}
              </p>
              <p v-if="round.latest_version" class="mt-1 text-meta text-ink-subtle">
                {{ bonusVersionStatusLabel(round.latest_version.status) }} ·
                {{ formatDateTime(round.latest_version.created_at) }}
              </p>
            </div>
            <div class="divider pt-3">
              <p class="field-label">正式核定版本</p>
              <p v-if="effectiveVersion" class="field-value">
                v{{ effectiveVersion.version }}
                <span class="ml-1 text-meta font-normal text-accent-strong">当前有效</span>
              </p>
              <p v-else class="field-value text-ink-muted">尚未核定</p>
              <p v-if="effectiveVersion" class="mt-1 text-meta text-ink-subtle">
                核定于 {{ formatDateTime(effectiveVersion.confirmed_at) }}
              </p>
            </div>
            <p class="rounded-lg bg-surface-muted px-3 py-2.5 text-meta leading-5 text-ink-muted">
              项目读取权限仅包含元数据。预算与成员金额需显式授权；核定不等于发布或实际付款。
            </p>
          </div>
        </div>
      </section>

      <BonusRoundWorkspace
        :key="contextKey"
        :team-id="teamId"
        :project-id="projectId"
        :round-id="roundId"
        @changed="refreshMetadata"
      />
      <p v-if="metadataError" role="alert" class="text-sm text-danger">
        版本历史刷新失败：{{ metadataError }}
      </p>
      <section class="card">
        <div class="card-header">
          <div>
            <h3 class="card-title">版本历史</h3>
            <p class="card-description">
              修订会创建新版本，原历史记录不覆盖；新版本独立核定后替换正式版本，另行发布后成员才可查看新结果。
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
                  <span v-if="version.effective" class="text-meta font-medium text-accent-strong"
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
        <EmptyState
          v-else
          icon="layers"
          title="尚无方案版本"
          description="批次已建立，但还没有提交任何版本方案。"
        />
      </section>
    </template>
  </div>
</template>
