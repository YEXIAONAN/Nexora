<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import * as bonusAPI from '@/api/bonus'
import { ApiError } from '@/api/client'
import { listProjects, type Project } from '@/api/projects'
import AlertBanner from '@/components/ui/AlertBanner.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import PageTabs from '@/components/ui/PageTabs.vue'
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
  <div class="page-stack">
    <PageHeader
      eyebrow="个人结果"
      title="我的奖金"
      description="仅显示已经独立核定并发布、且属于你本人的正式奖金结果。草稿和待审修订不影响原结果；新版本独立核定后替换正式版本，另行发布后才可查看。"
    >
      <template #actions>
        <button class="btn" :disabled="loading" @click="load">刷新</button>
      </template>
    </PageHeader>

    <PageTabs
      aria-label="团队导航"
      :items="[
        { label: '项目', to: `/teams/${teamId}/projects` },
        { label: '成员', to: `/teams/${teamId}/members` },
        { label: '我的贡献', to: `/teams/${teamId}/contribution-results` },
        { label: '我的奖金', active: true, count: results.length },
      ]"
    />

    <div v-if="notFound" class="card">
      <AlertBanner
        tone="warning"
        title="无法读取奖金结果"
        message="你可能不是该 Team 的活跃成员，或尚未加入该 Team 下的任何项目。奖金结果严格绑定本人身份与项目成员关系。"
      />
    </div>
    <AlertBanner v-else-if="error" tone="danger" :message="error" />
    <div v-else-if="loading" class="card space-y-2 p-4">
      <div v-for="i in 3" :key="i" class="skeleton h-24" />
    </div>

    <template v-else-if="ready">
      <div v-if="results.length" class="space-y-3">
        <article
          v-for="item in results"
          :key="`${item.round_id}-${item.confirmed_at}`"
          class="card"
        >
          <div class="card-header">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="card-title">{{ item.funding_label }}</h3>
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
              <p class="mt-1 text-caption text-ink-muted">{{ projectName(item.project_id) }}</p>
              <p class="mt-2 text-caption text-ink-subtle">
                核定时间 {{ formatDateTime(item.confirmed_at) }}
              </p>
            </div>
            <div class="text-right">
              <p class="text-caption text-ink-subtle">本人金额（CNY）</p>
              <p class="mt-1 text-display font-semibold tracking-tight text-ink">
                {{ formatBonusAmount(item.amount_minor) }}
              </p>
            </div>
          </div>
          <div class="card-footer">
            <p class="text-meta text-ink-subtle">
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

      <div v-else class="card">
        <EmptyState
          icon="award"
          title="暂无已核定的奖金结果"
          description="只有独立核定并发布后的奖金结果才会显示。若尚未发布，请等待负责人员完成核定与发布流程。"
        >
          <template #action>
            <RouterLink :to="`/teams/${teamId}/projects`" class="btn btn-sm">查看项目</RouterLink>
          </template>
        </EmptyState>
      </div>
    </template>
  </div>
</template>
