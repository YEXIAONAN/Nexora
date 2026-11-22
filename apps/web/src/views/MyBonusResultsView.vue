<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import * as bonusAPI from '@/api/bonus'
import { ApiError } from '@/api/client'
import { listProjects, type Project } from '@/api/projects'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import { formatBonusAmount } from '@/components/bonus/bonusPresentation'
import { useTeamsStore } from '@/stores/teams'
import { formatDateTime } from '@/utils/format'

const route = useRoute(),
  teams = useTeamsStore()
const teamId = computed(() => String(route.params.teamId ?? ''))
const results = ref<bonusAPI.BonusResult[]>([])
const projects = ref<Project[]>([])
const loading = ref(false),
  error = ref(''),
  notFound = ref(false),
  confirmed = ref('')
let generation = 0
let controller: AbortController | null = null
const ready = computed(() => confirmed.value === teamId.value && !loading.value)
const projectNames = computed(
  () => new Map(projects.value.map((project) => [project.id, project.name])),
)
function projectName(projectId: string): string {
  return projectNames.value.get(projectId) ?? '项目'
}

async function load() {
  const team = teamId.value,
    gen = ++generation
  controller?.abort()
  controller = new AbortController()
  const signal = controller.signal
  confirmed.value = ''
  loading.value = true
  error.value = ''
  notFound.value = false
  results.value = []
  projects.value = []
  try {
    const [page, projectList] = await Promise.all([
      bonusAPI.listMyBonusResults(team, signal),
      listProjects(team, signal),
    ])
    if (generation !== gen || team !== teamId.value) return
    results.value = page.items
    projects.value = projectList
    teams.selectTeam(team)
    confirmed.value = team
  } catch (e) {
    if (generation !== gen || team !== teamId.value) return
    if (e instanceof ApiError && [403, 404].includes(e.status ?? 0)) notFound.value = true
    else error.value = e instanceof ApiError ? e.message : '我的奖金结果加载失败'
  } finally {
    if (generation === gen && team === teamId.value) loading.value = false
  }
}

watch(
  teamId,
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
  <div class="space-y-6">
    <div class="page-header">
      <div>
        <p class="text-xs text-ink-subtle">{{ teams.currentTeam?.name || '团队工作区' }}</p>
        <h2 class="page-title mt-1">我的奖金</h2>
        <p class="page-subtitle">
          仅显示已经独立核定并发布、且属于你本人的正式奖金结果。草稿和待审修订不影响原结果；新版本独立核定后替换正式版本，另行发布后才可查看。
        </p>
      </div>
      <button class="btn" :disabled="loading" @click="load">刷新</button>
    </div>

    <nav
      class="flex gap-1 overflow-x-auto rounded-lg border border-line bg-surface p-1"
      aria-label="团队导航"
    >
      <RouterLink
        :to="`/teams/${teamId}/projects`"
        class="rounded-md px-3 py-1.5 text-xs font-medium text-ink-muted transition-colors hover:text-ink"
        >项目</RouterLink
      >
      <RouterLink
        :to="`/teams/${teamId}/members`"
        class="rounded-md px-3 py-1.5 text-xs font-medium text-ink-muted transition-colors hover:text-ink"
        >成员</RouterLink
      >
      <RouterLink
        :to="`/teams/${teamId}/contribution-results`"
        class="rounded-md px-3 py-1.5 text-xs font-medium text-ink-muted transition-colors hover:text-ink"
        >我的贡献</RouterLink
      >
      <span class="rounded-md bg-accent-soft px-3 py-1.5 text-xs font-medium text-accent-strong"
        >我的奖金</span
      >
    </nav>

    <p v-if="notFound" role="alert" class="card empty-state">
      <strong class="text-sm text-ink">无法读取奖金结果</strong>
      <span class="mt-2 max-w-md text-sm text-ink-muted"
        >你可能不是该 Team 的活跃成员，或尚未加入该 Team
        下的任何项目。奖金结果严格绑定本人身份与项目成员关系。</span
      >
    </p>
    <p v-else-if="error" role="alert" class="rounded-lg bg-danger-soft p-4 text-sm text-danger">
      {{ error }}
    </p>
    <p v-else-if="loading" role="status" class="card p-8 text-sm text-ink-muted">
      正在加载我的奖金…
    </p>

    <template v-else-if="ready">
      <div v-if="results.length" class="space-y-3">
        <article
          v-for="item in results"
          :key="`${item.round_id}-${item.confirmed_at}`"
          class="card px-5 py-4"
        >
          <div class="flex flex-wrap items-start justify-between gap-4">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="text-base font-semibold text-ink">{{ item.funding_label }}</h3>
                <StatusBadge
                  :label="
                    item.payment_status === 'recorded'
                      ? '已发布 · 已记录模拟发放'
                      : '已发布 · 未记录模拟发放'
                  "
                  tone="success"
                  dot
                />
              </div>
              <p class="mt-1 text-xs text-ink-muted">{{ projectName(item.project_id) }}</p>
              <p class="mt-2 text-xs text-ink-subtle">
                核定时间 {{ formatDateTime(item.confirmed_at) }}
              </p>
            </div>
            <div class="text-right">
              <p class="text-xs text-ink-subtle">本人金额（CNY）</p>
              <p class="mt-1 text-2xl font-semibold tracking-tight text-ink">
                {{ formatBonusAmount(item.amount_minor) }}
              </p>
            </div>
          </div>
          <div
            class="mt-4 flex flex-wrap items-center justify-between gap-2 border-t border-line pt-3"
          >
            <p class="text-[11px] text-ink-subtle">
              这是内部正式奖金结果，不代表已支付。模拟发放记录非实际转账，Nexora 不执行真实付款。
            </p>
            <RouterLink
              :to="{
                name: 'bonus-round-detail',
                params: { teamId, projectId: item.project_id, roundId: item.round_id },
              }"
              class="btn btn-sm"
              >查看批次详情 →</RouterLink
            >
          </div>
        </article>
      </div>

      <div v-else class="card empty-state">
        <strong class="text-sm text-ink">暂无已核定的奖金结果</strong>
        <p class="mt-2 max-w-md text-sm text-ink-muted">
          只有独立核定并发布后的奖金结果才会显示。若尚未发布，请等待负责人员完成核定与发布流程。
        </p>
        <RouterLink :to="`/teams/${teamId}/projects`" class="btn btn-sm mt-4">查看项目</RouterLink>
      </div>
    </template>
  </div>
</template>
