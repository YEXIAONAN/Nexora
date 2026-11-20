<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import * as bonusAPI from '@/api/bonus'
import { ApiError } from '@/api/client'
import { getProject, type Project } from '@/api/projects'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import { bonusRoundStatusLabel, bonusRoundStatusTone } from '@/components/bonus/bonusPresentation'
import { useTeamsStore } from '@/stores/teams'
import { formatDateTime } from '@/utils/format'

const route = useRoute(),
  teams = useTeamsStore()
const teamId = computed(() => String(route.params.teamId ?? ''))
const projectId = computed(() => String(route.params.projectId ?? ''))
const project = ref<Project | null>(null),
  rounds = ref<bonusAPI.BonusRoundSummary[]>([])
const loading = ref(false),
  error = ref(''),
  notFound = ref(false),
  confirmed = ref('')
let generation = 0
const ready = computed(
  () => confirmed.value === `${teamId.value}/${projectId.value}` && !loading.value,
)

async function load() {
  const team = teamId.value,
    projectKey = projectId.value,
    key = `${team}/${projectKey}`,
    gen = ++generation
  confirmed.value = ''
  loading.value = true
  error.value = ''
  notFound.value = false
  rounds.value = []
  project.value = null
  try {
    const [p, page] = await Promise.all([
      getProject(team, projectKey),
      bonusAPI.listBonusRounds(team, projectKey),
    ])
    if (generation !== gen || key !== `${teamId.value}/${projectId.value}`) return
    project.value = p
    rounds.value = page.items
    teams.selectTeam(team)
    confirmed.value = key
  } catch (e) {
    if (generation !== gen || key !== `${teamId.value}/${projectId.value}`) return
    if (e instanceof ApiError && [403, 404].includes(e.status ?? 0)) notFound.value = true
    else error.value = e instanceof ApiError ? e.message : '奖金批次加载失败'
  } finally {
    if (generation === gen && key === `${teamId.value}/${projectId.value}`) loading.value = false
  }
}

watch(
  [teamId, projectId],
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
          :to="{ name: 'project-detail', params: { teamId, projectId } }"
          class="text-xs text-ink-subtle hover:text-ink"
          >← 返回项目</RouterLink
        >
        <h2 class="page-title mt-2">奖金批次</h2>
        <p class="page-subtitle">
          {{ project?.name || '项目' }}
          的独立资金批次。这里只展示批次元数据与版本历史，预算和成员金额不对项目成员公开。
        </p>
      </div>
      <div class="flex gap-2">
        <button class="btn" :disabled="loading" @click="load">刷新</button>
      </div>
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
      <span class="rounded-md bg-accent-soft px-3 py-1.5 text-xs font-medium text-accent-strong"
        >奖金批次</span
      >
      <RouterLink
        :to="{ name: 'my-bonus-results', params: { teamId } }"
        class="rounded-md px-3 py-1.5 text-xs font-medium text-ink-muted transition-colors hover:text-ink"
        >我的奖金</RouterLink
      >
    </nav>

    <p v-if="notFound" role="alert" class="card empty-state">
      <strong class="text-sm text-ink">无法查看该项目的奖金批次</strong>
      <span class="mt-2 max-w-md text-sm text-ink-muted"
        >你可能不是该 Team 或 Project 的成员。奖金数据严格按 Team 与 Project
        授权，平台管理员也不会自动获得访问权。</span
      >
    </p>
    <p v-else-if="error" role="alert" class="rounded-lg bg-danger-soft p-4 text-sm text-danger">
      {{ error }}
    </p>
    <p v-else-if="loading" role="status" class="card p-8 text-sm text-ink-muted">
      正在加载奖金批次…
    </p>

    <template v-else-if="ready">
      <section
        v-if="project?.status === 'archived'"
        class="rounded-lg border border-line bg-surface-muted px-4 py-3 text-xs text-ink-muted"
      >
        该项目已归档：普通项目内容只读，但奖金批次作为独立资金域仍可查看。
      </section>

      <div v-if="rounds.length" class="space-y-3">
        <article v-for="round in rounds" :key="round.id" class="card px-5 py-4">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="text-base font-semibold text-ink">{{ round.funding_label }}</h3>
                <StatusBadge
                  :label="bonusRoundStatusLabel(round.status)"
                  :tone="bonusRoundStatusTone(round.status)"
                  dot
                />
              </div>
              <p class="mt-1 font-mono text-[11px] text-ink-subtle">{{ round.funding_key }}</p>
              <p class="mt-2 line-clamp-2 text-sm leading-6 text-ink-muted">
                {{ round.funding_source }}
              </p>
            </div>
            <RouterLink
              :to="{
                name: 'bonus-round-detail',
                params: { teamId, projectId, roundId: round.id },
              }"
              class="btn btn-sm shrink-0"
              >查看批次 →</RouterLink
            >
          </div>
          <dl
            class="mt-4 grid grid-cols-2 gap-x-6 gap-y-2 border-t border-line pt-3 text-xs sm:grid-cols-4"
          >
            <div>
              <dt class="text-ink-subtle">当前方案版本</dt>
              <dd class="mt-0.5 text-ink">
                {{ round.latest_version ? `v${round.latest_version.version}` : '尚无版本' }}
              </dd>
            </div>
            <div>
              <dt class="text-ink-subtle">正式核定版本</dt>
              <dd class="mt-0.5 text-ink">
                {{
                  round.effective_confirmed_version
                    ? `v${round.effective_confirmed_version.version}`
                    : '未核定'
                }}
              </dd>
            </div>
            <div>
              <dt class="text-ink-subtle">修订次数</dt>
              <dd class="mt-0.5 text-ink">{{ round.revision }}</dd>
            </div>
            <div>
              <dt class="text-ink-subtle">创建时间</dt>
              <dd class="mt-0.5 text-ink">{{ formatDateTime(round.created_at) }}</dd>
            </div>
          </dl>
          <p class="mt-3 text-[11px] text-ink-subtle">
            金额与预算在本阶段不对项目成员展示；未正式核定的方案不会产生奖金结果。
          </p>
        </article>
      </div>

      <div v-else class="card empty-state">
        <strong class="text-sm text-ink">{{ project?.name || '该项目' }} 暂无奖金批次</strong>
        <p class="mt-2 max-w-md text-sm text-ink-muted">
          奖金批次是项目归档后仍可独立续办的资金域。当前没有已登记的批次，可能尚未启动奖金流程。
        </p>
        <RouterLink
          :to="{ name: 'project-detail', params: { teamId, projectId } }"
          class="btn btn-sm mt-4"
          >返回项目工作区</RouterLink
        >
      </div>
    </template>
  </div>
</template>
