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
import BonusRoundCreateForm from '@/components/bonus/BonusRoundCreateForm.vue'
import { useAuthStore } from '@/stores/auth'
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
const auth = useAuthStore()
const workspace = ref<bonusAPI.BonusProjectWorkspace | null>(null)
const workspaceError = ref('')
const creating = ref(false)
const createError = ref('')
const created = ref<bonusAPI.BonusCommandResult | null>(null)
const showCreate = ref(false)
let controller: AbortController | null = null
let generation = 0
const ready = computed(
  () => confirmed.value === `${teamId.value}/${projectId.value}` && !loading.value,
)
const memberCandidates = computed(
  () => workspace.value?.members.filter((member) => member.user_id !== auth.user?.id) ?? [],
)
const officerShortage = computed(
  () => workspace.value?.can_create === true && memberCandidates.value.length < 2,
)
const sourceCount = computed(() => workspace.value?.sources.length ?? 0)

async function load() {
  const team = teamId.value,
    projectKey = projectId.value,
    key = `${team}/${projectKey}`,
    gen = ++generation
  controller?.abort()
  controller = new AbortController()
  const signal = controller.signal
  workspace.value = null
  workspaceError.value = ''
  showCreate.value = false
  creating.value = false
  createError.value = ''
  created.value = null
  confirmed.value = ''
  loading.value = true
  error.value = ''
  notFound.value = false
  rounds.value = []
  project.value = null
  try {
    const [p, page, management] = await Promise.all([
      getProject(team, projectKey, signal),
      bonusAPI.listBonusRounds(team, projectKey, signal),
      bonusAPI.getBonusProjectWorkspace(team, projectKey, signal).catch((e: unknown) => {
        if (generation === gen && !(e instanceof ApiError && [403, 404].includes(e.status ?? 0)))
          workspaceError.value = e instanceof ApiError ? e.message : '管理入口加载失败，请刷新重试'
        return null
      }),
    ])
    if (generation !== gen || key !== `${teamId.value}/${projectId.value}`) return
    project.value = p
    workspace.value = management
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

async function createRound(input: Omit<bonusAPI.CreateBonusRound, 'client_request_id'>) {
  if (creating.value || !ready.value || !workspace.value?.can_create) return
  const gen = generation,
    team = teamId.value,
    projectKey = projectId.value
  const signal = controller?.signal
  creating.value = true
  createError.value = ''
  created.value = null
  try {
    const result = await bonusAPI.createBonusRound(
      team,
      projectKey,
      { ...input, client_request_id: crypto.randomUUID() },
      signal,
    )
    if (gen !== generation) return
    created.value = result
    showCreate.value = false
    try {
      const page = await bonusAPI.listBonusRounds(team, projectKey, signal)
      if (gen === generation) rounds.value = page.items
    } catch {
      if (gen === generation) createError.value = '批次已创建，但列表刷新失败，请刷新查看。'
    }
  } catch (e) {
    if (gen !== generation) return
    createError.value =
      e instanceof ApiError
        ? `${e.message}${e.status === 409 ? ' 请刷新检查已有批次后重试。' : ''}`
        : '创建失败，请重试'
  } finally {
    if (gen === generation) creating.value = false
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
  controller?.abort()
})
</script>

<template>
  <div class="page-stack">
    <PageHeader
      eyebrow="项目资金"
      title="奖金管理"
      :description="`${project?.name || '项目'}的独立资金批次。奖金域独立于项目进度与贡献计算：项目归档后仍可继续办理，无需等待归档。`"
      :crumbs="[
        { label: '项目', to: `/teams/${teamId}/projects` },
        { label: project?.name || '项目', to: `/teams/${teamId}/projects/${projectId}` },
        { label: '奖金管理' },
      ]"
    >
      <template #actions>
        <button class="btn" :disabled="loading || creating" @click="load">刷新</button>
        <button
          v-if="ready && workspace?.can_create"
          class="btn btn-primary"
          :disabled="creating || officerShortage"
          @click="showCreate = !showCreate"
        >
          {{ showCreate ? '收起创建' : '创建奖金批次' }}
        </button>
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
        { label: '奖金管理', active: true, count: rounds.length },
        { label: '我的奖金', to: `/teams/${teamId}/bonus-results` },
      ]"
    />

    <div v-if="notFound" class="card">
      <AlertBanner
        tone="warning"
        title="无法查看该项目的奖金批次"
        message="你可能不是该 Team 或 Project 的成员。奖金数据严格按 Team 与 Project 授权，平台管理员也不会自动获得访问权。"
      />
    </div>
    <AlertBanner v-else-if="error" tone="danger" :message="error" />
    <div v-else-if="loading" class="card space-y-2 p-4">
      <div v-for="i in 3" :key="i" class="skeleton h-20" />
    </div>

    <template v-else-if="ready">
      <AlertBanner v-if="workspaceError" tone="danger" :message="`管理入口：${workspaceError}`" />
      <AlertBanner v-if="createError" tone="danger" :message="createError" />
      <AlertBanner
        v-if="created"
        tone="success"
        title="批次创建成功，初始预算为 0"
        message="当前创建者不自动获得金额权限，请由选定负责人维护方案。"
      >
        <template #action>
          <RouterLink
            :to="{
              name: 'bonus-round-detail',
              params: { teamId, projectId, roundId: created.round_id },
            }"
            class="btn btn-sm"
            >进入新批次与授权管理 →</RouterLink
          >
        </template>
      </AlertBanner>

      <section v-if="workspace?.can_create" class="card">
        <div class="card-header">
          <div>
            <h3 class="card-title">创建批次的前置条件</h3>
            <p class="card-description">
              批次、方案、核定与发放是分开的步骤；创建批次本身不需要项目归档。
            </p>
          </div>
          <span class="badge">独立资金域</span>
        </div>
        <ul class="divide-y divide-line">
          <li class="flex flex-wrap items-center justify-between gap-3 px-5 py-3">
            <div>
              <p class="text-body font-medium text-ink">创建权限</p>
              <p class="text-caption text-ink-muted">团队 Owner/Admin 或当前项目负责人</p>
            </div>
            <StatusBadge label="已满足" tone="success" dot />
          </li>
          <li class="flex flex-wrap items-center justify-between gap-3 px-5 py-3">
            <div>
              <p class="text-body font-medium text-ink">至少两位其他活跃项目成员</p>
              <p class="text-caption text-ink-muted">
                金额负责人与独立核定人必须不同，且均不能是创建者。当前可选
                {{ memberCandidates.length }} 人。
              </p>
            </div>
            <StatusBadge
              :label="officerShortage ? '还缺成员' : '已满足'"
              :tone="officerShortage ? 'warning' : 'success'"
              dot
            />
          </li>
          <li class="flex flex-wrap items-center justify-between gap-3 px-5 py-3">
            <div>
              <p class="text-body font-medium text-ink">已正式核定的贡献</p>
              <p class="text-caption text-ink-muted">
                用于编制方案与成员分配；创建批次时不强制，但提交独立核定前必须存在。 当前
                {{ sourceCount }} 项可用来源。
              </p>
            </div>
            <StatusBadge
              :label="sourceCount ? '可用于方案' : '尚无来源'"
              :tone="sourceCount ? 'success' : 'neutral'"
            />
          </li>
        </ul>
        <div class="card-footer">
          <p class="text-caption text-ink-muted">
            项目归档后仍可继续创建批次、编制方案、核定与记录模拟发放；归档不会修改已冻结的贡献历史。
          </p>
          <RouterLink
            v-if="officerShortage"
            :to="{
              name: 'project-detail',
              params: { teamId, projectId },
              query: { tab: 'members' },
            }"
            class="btn btn-sm"
            >补充项目成员</RouterLink
          >
        </div>
      </section>

      <AlertBanner
        v-else-if="workspace"
        tone="info"
        title="你当前没有创建批次的权限"
        message="只有团队 Owner/Admin 或当前项目负责人可以创建批次并指定金额负责人。你仍可查看批次元数据；如已获金额授权，可进入批次处理方案与核定。"
      />

      <AlertBanner
        v-else-if="!workspaceError"
        tone="info"
        title="管理入口未对当前账号开放"
        message="你可能是普通项目成员，或尚未获得该批次的显式金额授权。批次元数据仍可查看；如需参与方案、核定或发放，请联系项目负责人授予相应角色。"
      />

      <BonusRoundCreateForm
        v-if="showCreate && workspace?.can_create && !officerShortage"
        :key="confirmed"
        :members="workspace.members"
        :user-id="auth.user?.id ?? ''"
        :busy="creating"
        @create="createRound"
      />

      <section
        v-if="project?.status === 'archived'"
        class="rounded-control border border-line bg-surface-muted px-4 py-3 text-caption leading-5 text-ink-muted"
      >
        该项目已归档：普通项目内容只读，但奖金批次作为独立资金域仍可继续查看和办理。
      </section>

      <div v-if="rounds.length" class="space-y-3">
        <article v-for="round in rounds" :key="round.id" class="card">
          <div class="card-header">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="card-title">{{ round.funding_label }}</h3>
                <StatusBadge
                  :label="bonusRoundStatusLabel(round.status)"
                  :tone="bonusRoundStatusTone(round.status)"
                  dot
                />
              </div>
              <p class="mt-1 font-mono text-meta text-ink-subtle">{{ round.funding_key }}</p>
              <p class="mt-2 line-clamp-2 max-w-3xl text-caption leading-5 text-ink-muted">
                {{ round.funding_source }}
              </p>
            </div>
            <RouterLink
              :to="{
                name: 'bonus-round-detail',
                params: { teamId, projectId, roundId: round.id },
              }"
              class="btn btn-sm"
              >进入批次 →</RouterLink
            >
          </div>
          <div class="card-body">
            <dl class="grid grid-cols-2 gap-x-6 gap-y-2 text-caption sm:grid-cols-4">
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
          </div>
          <div class="card-footer text-meta text-ink-subtle">
            <span>金额与预算仅向显式授权成员开放。</span>
            <span>进入批次可按授权编辑、独立核定及发布。</span>
          </div>
        </article>
      </div>

      <div v-else class="card">
        <EmptyState
          icon="award"
          :title="`${project?.name || '该项目'} 暂无奖金批次`"
          description="奖金批次是独立资金域，项目归档后仍可继续办理。当前没有已登记的批次；如你具备权限，可使用页面上方的创建入口开始。"
        />
      </div>
    </template>
  </div>
</template>
