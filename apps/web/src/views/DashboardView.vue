<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink, useRouter } from 'vue-router'

import { ApiError } from '@/api/client'
import { listProjects, type Project } from '@/api/projects'
import { pendingReviews, type PendingReview } from '@/api/submissions'
import { getTaskTree, taskStatusLabel, type Task } from '@/api/tasks'
import { listTeamMembers, type TeamMember } from '@/api/teams'
import AlertBanner from '@/components/ui/AlertBanner.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import MetricCard from '@/components/ui/MetricCard.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import { useAuthStore } from '@/stores/auth'
import { useTeamsStore } from '@/stores/teams'
import { useUiStore } from '@/stores/ui'
import { teamRoleLabel } from '@/utils/format'

interface MyTask {
  task: Task
  project: Project
}

interface ProjectStat {
  progress_bps: number
  leaf_count: number
  completed_leaves: number
  task_count: number
}

const auth = useAuthStore()
const teams = useTeamsStore()
const ui = useUiStore()
const router = useRouter()

const loading = ref(true)
const failed = ref<string | null>(null)
const projects = ref<Project[]>([])
const myTasks = ref<MyTask[]>([])
const reviews = ref<PendingReview[]>([])
const members = ref<TeamMember[]>([])
const stats = ref<Record<string, ProjectStat>>({})
let generation = 0
let controller: AbortController | null = null

const teamId = computed(() => teams.currentTeamId)
const team = computed(() => teams.currentTeam)
const activeProjects = computed(() =>
  projects.value.filter((project) => project.status === 'active'),
)
const openTasks = computed(() => myTasks.value.filter(({ task }) => task.status !== 'completed'))
const memberNames = computed(() => {
  const map = new Map<string, string>()
  for (const member of members.value)
    map.set(member.user_id, member.display_name || member.username)
  return map
})

const teamSummary = computed(() => {
  if (!team.value) return '先创建或加入一个团队，开始管理项目与任务。'
  return `${teamRoleLabel(team.value.my_role)} · ${members.value.length} 名成员 · ${projects.value.length} 个项目`
})

const priorityTone: Record<Task['priority'], 'neutral' | 'warning' | 'danger'> = {
  low: 'neutral',
  normal: 'neutral',
  high: 'warning',
  urgent: 'danger',
}

const priorityLabel: Record<Task['priority'], string> = {
  low: '低',
  normal: '普通',
  high: '高',
  urgent: '紧急',
}

function statusTone(status: Task['status']): 'neutral' | 'success' | 'danger' | 'warning' | 'info' {
  if (status === 'completed') return 'success'
  if (status === 'awaiting_review') return 'warning'
  if (status === 'rejected') return 'danger'
  if (status === 'in_progress') return 'info'
  return 'neutral'
}

function ownerName(task: Task): string {
  return memberNames.value.get(task.owner_id) ?? '未分配'
}

function leadName(project: Project): string {
  return memberNames.value.get(project.lead_id) ?? '—'
}

function projectStat(project: Project): ProjectStat | null {
  return stats.value[project.id] ?? null
}

function progressText(project: Project): string {
  const stat = projectStat(project)
  return stat ? `${(stat.progress_bps / 100).toFixed(2)}%` : '—'
}

function isOverdue(task: Task): boolean {
  return (
    !!task.planned_end_at &&
    task.status !== 'completed' &&
    task.status !== 'cancelled' &&
    new Date(task.planned_end_at).getTime() < Date.now()
  )
}

function taskLink(item: MyTask): string {
  return `/teams/${item.project.team_id}/projects/${item.project.id}?task=${item.task.id}`
}

function reviewLink(review: PendingReview): string {
  return `/teams/${teamId.value}/projects/${review.project_id}?task=${review.task_id}`
}

async function load(): Promise<void> {
  const currentTeamId = teamId.value
  if (!currentTeamId) {
    loading.value = false
    projects.value = []
    myTasks.value = []
    reviews.value = []
    members.value = []
    stats.value = {}
    return
  }
  const current = ++generation
  controller?.abort()
  controller = new AbortController()
  const signal = controller.signal
  loading.value = true
  failed.value = null
  try {
    const [projectList, memberList, pending] = await Promise.all([
      listProjects(currentTeamId, signal),
      listTeamMembers(currentTeamId, signal).catch(() => []),
      pendingReviews(currentTeamId).catch(() => []),
    ])
    if (current !== generation) return
    projects.value = projectList
    members.value = memberList
    reviews.value = pending

    const active = projectList.filter((project) => project.status === 'active').slice(0, 12)
    const trees = await Promise.all(
      active.map((project) =>
        getTaskTree(currentTeamId, project.id, signal)
          .then((tree) => ({ project, tree }))
          .catch(() => null),
      ),
    )
    if (current !== generation) return
    const mine: MyTask[] = []
    const nextStats: Record<string, ProjectStat> = {}
    for (const entry of trees) {
      if (!entry) continue
      const { project, tree } = entry
      nextStats[project.id] = {
        progress_bps: tree.progress.progress_bps,
        leaf_count: tree.progress.leaf_count,
        completed_leaves: tree.tasks.filter((task) => task.is_leaf && task.status === 'completed')
          .length,
        task_count: tree.tasks.length,
      }
      for (const task of tree.tasks) {
        const involved =
          task.owner_id === auth.user?.id || task.collaborator_ids.includes(auth.user?.id ?? '')
        if (involved && task.status !== 'completed' && task.status !== 'cancelled') {
          mine.push({ task, project })
        }
      }
    }
    mine.sort((a, b) => {
      const rank = { urgent: 0, high: 1, normal: 2, low: 3 }
      const byPriority = rank[a.task.priority] - rank[b.task.priority]
      if (byPriority !== 0) return byPriority
      return (a.task.planned_end_at ?? '9999').localeCompare(b.task.planned_end_at ?? '9999')
    })
    myTasks.value = mine
    stats.value = nextStats
  } catch (reason) {
    if (current !== generation) return
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    failed.value = reason instanceof ApiError ? reason.message : '工作台数据加载失败'
  } finally {
    if (current === generation) loading.value = false
  }
}

onMounted(async () => {
  if (auth.user) await teams.loadTeams(auth.user.id)
  await load()
})

watch(teamId, load)

async function goCreateProject(): Promise<void> {
  if (!teamId.value) return
  await router.push(`/teams/${teamId.value}/projects?create=1`)
}

function goCreateTask(): void {
  if (!teamId.value) return
  if (activeProjects.value.length === 0) {
    ui.notify('请先创建一个项目，再添加任务', 'info')
    void goCreateProject()
    return
  }
  void router.push(`/teams/${teamId.value}/projects/${activeProjects.value[0].id}?createTask=1`)
}

function goTeam(): void {
  void router.push('/teams')
}
</script>

<template>
  <div class="page-stack">
    <PageHeader
      eyebrow="工作台"
      :title="team ? team.name : '欢迎使用 Nexora'"
      :description="
        team
          ? `${teamSummary}。这里汇总你当前团队的待办、审核与项目进展。`
          : '先创建或加入一个团队，开始管理项目与任务。'
      "
    >
      <template #actions>
        <button type="button" class="btn" :disabled="!team" @click="goCreateTask">
          <AppIcon name="plus" :size="15" />
          新建任务
        </button>
        <button type="button" class="btn btn-primary" :disabled="!team" @click="goCreateProject">
          <AppIcon name="folder" :size="15" />
          新建项目
        </button>
      </template>
    </PageHeader>

    <AlertBanner v-if="failed" tone="danger" :message="failed">
      <template #action>
        <button type="button" class="btn btn-sm" @click="load">重试</button>
      </template>
    </AlertBanner>

    <div v-if="!team && !loading" class="card">
      <EmptyState
        icon="users"
        title="还没有可用的团队"
        description="创建一个团队，或先在「邀请收件箱」接受别人的邀请。团队之间的数据完全隔离。"
      >
        <template #action>
          <button type="button" class="btn btn-primary btn-sm" @click="goTeam">前往团队</button>
          <RouterLink to="/invitations" class="btn btn-sm">查看邀请</RouterLink>
        </template>
      </EmptyState>
    </div>

    <template v-else>
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <template v-if="loading">
          <div v-for="i in 4" :key="i" class="skeleton h-[116px]" />
        </template>
        <template v-else>
          <MetricCard
            label="进行中的项目"
            :value="activeProjects.length"
            :hint="`共 ${projects.length} 个项目`"
            icon="folder"
            :to="teamId ? `/teams/${teamId}/projects` : ''"
          />
          <MetricCard
            label="分配给我的待办"
            :value="openTasks.length"
            hint="按优先级与截止时间排序"
            icon="check"
            to="/my-tasks"
          />
          <MetricCard
            label="待我审核"
            :value="reviews.length"
            hint="等待处理的成果提交"
            icon="inbox"
            :tone="reviews.length ? 'warning' : 'default'"
          />
          <MetricCard
            label="团队成员"
            :value="members.length"
            hint="当前团队的活跃成员"
            icon="users"
            :to="teamId ? `/teams/${teamId}/members` : ''"
          />
        </template>
      </div>

      <div class="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,1.35fr)_minmax(0,1fr)]">
        <section class="card">
          <div class="card-header">
            <div>
              <h3 class="card-title">我的待办</h3>
              <p class="card-description">按优先级与截止时间排序，点击进入任务详情。</p>
            </div>
            <RouterLink to="/my-tasks" class="btn btn-sm">全部任务</RouterLink>
          </div>
          <div v-if="loading" class="space-y-2 p-4">
            <div v-for="i in 4" :key="i" class="skeleton h-14" />
          </div>
          <EmptyState
            v-else-if="myTasks.length === 0"
            icon="check"
            title="暂时没有分配给你的任务"
            description="创建项目后，把任务分配给自己或团队成员。"
          >
            <template #action>
              <button type="button" class="btn btn-sm" @click="goCreateTask">新建任务</button>
            </template>
          </EmptyState>
          <ul v-else class="divide-y divide-line">
            <li v-for="item in myTasks.slice(0, 6)" :key="item.task.id">
              <RouterLink
                :to="taskLink(item)"
                class="flex items-start gap-3 px-5 py-3 transition-colors hover:bg-surface-hover"
              >
                <span
                  class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full"
                  :class="item.task.priority === 'urgent' ? 'bg-danger' : 'bg-accent'"
                  aria-hidden="true"
                />
                <div class="min-w-0 flex-1">
                  <p class="truncate text-body font-medium text-ink">{{ item.task.title }}</p>
                  <p class="mt-0.5 truncate text-caption text-ink-muted">
                    {{ item.project.name }} · 负责人 {{ ownerName(item.task) }}
                  </p>
                </div>
                <div class="flex shrink-0 flex-col items-end gap-1.5">
                  <div class="flex items-center gap-1.5">
                    <StatusBadge
                      :label="priorityLabel[item.task.priority]"
                      :tone="priorityTone[item.task.priority]"
                    />
                    <StatusBadge
                      :label="taskStatusLabel[item.task.status]"
                      :tone="statusTone(item.task.status)"
                    />
                  </div>
                  <span
                    v-if="item.task.planned_end_at"
                    class="text-meta"
                    :class="isOverdue(item.task) ? 'text-danger' : 'text-ink-subtle'"
                  >
                    截止 {{ item.task.planned_end_at.slice(0, 10) }}
                  </span>
                </div>
              </RouterLink>
            </li>
          </ul>
        </section>

        <section class="card">
          <div class="card-header">
            <div>
              <h3 class="card-title">待我审核</h3>
              <p class="card-description">只有被授权的审核人可以处理。</p>
            </div>
            <span class="badge">{{ reviews.length }}</span>
          </div>
          <div v-if="loading" class="space-y-2 p-4">
            <div v-for="i in 3" :key="i" class="skeleton h-14" />
          </div>
          <EmptyState
            v-else-if="reviews.length === 0"
            icon="inbox"
            title="没有待审核的成果"
            description="成员提交成果后，被授权的审核人会在这里看到。"
          />
          <ul v-else class="divide-y divide-line">
            <li v-for="review in reviews.slice(0, 5)" :key="review.submission_version_id">
              <RouterLink
                :to="reviewLink(review)"
                class="flex items-start gap-3 px-5 py-3 transition-colors hover:bg-surface-hover"
              >
                <div class="min-w-0 flex-1">
                  <p class="truncate text-body font-medium text-ink">{{ review.task_title }}</p>
                  <p class="mt-0.5 truncate text-caption text-ink-muted">
                    {{ review.project_name }} · {{ review.author_name }} · v{{ review.version }}
                  </p>
                </div>
                <AppIcon name="arrow-right" :size="15" class="mt-1 shrink-0 text-ink-subtle" />
              </RouterLink>
            </li>
          </ul>
        </section>
      </div>

      <section class="card overflow-hidden">
        <div class="card-header">
          <div>
            <h3 class="card-title">进行中的项目</h3>
            <p class="card-description">有效进度按叶子任务工作量汇总，取消任务不参与计算。</p>
          </div>
          <RouterLink v-if="teamId" :to="`/teams/${teamId}/projects`" class="btn btn-sm">
            全部项目
          </RouterLink>
        </div>
        <div v-if="loading" class="grid gap-3 p-5 sm:grid-cols-2">
          <div v-for="i in 2" :key="i" class="skeleton h-24" />
        </div>
        <EmptyState
          v-else-if="activeProjects.length === 0"
          icon="folder"
          title="还没有进行中的项目"
          description="创建第一个项目，然后拆分任务并分配给成员。"
        >
          <template #action>
            <button type="button" class="btn btn-primary btn-sm" @click="goCreateProject">
              创建项目
            </button>
          </template>
        </EmptyState>
        <div v-else class="table-wrap">
          <table class="data-table">
            <thead>
              <tr>
                <th>项目</th>
                <th>状态</th>
                <th class="w-[220px]">有效进度</th>
                <th>任务</th>
                <th class="hidden lg:table-cell">负责人</th>
                <th class="hidden xl:table-cell">创建时间</th>
                <th />
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="project in activeProjects.slice(0, 6)"
                :key="project.id"
                class="row-clickable"
                @click="router.push(`/teams/${project.team_id}/projects/${project.id}`)"
              >
                <td>
                  <p class="font-medium text-ink">{{ project.name }}</p>
                  <p class="mt-0.5 line-clamp-1 max-w-[420px] text-caption text-ink-muted">
                    {{ project.description || '暂无项目描述' }}
                  </p>
                </td>
                <td>
                  <StatusBadge label="进行中" tone="success" dot />
                </td>
                <td>
                  <div class="flex items-center gap-3">
                    <div class="progress-track w-full max-w-[140px]">
                      <div
                        class="progress-bar"
                        :style="{ width: `${(projectStat(project)?.progress_bps ?? 0) / 100}%` }"
                      />
                    </div>
                    <span class="w-14 text-right text-caption font-medium text-ink">
                      {{ progressText(project) }}
                    </span>
                  </div>
                </td>
                <td class="text-caption text-ink-muted">
                  {{ projectStat(project)?.task_count ?? 0 }} 个
                </td>
                <td class="hidden text-caption text-ink-muted lg:table-cell">
                  {{ leadName(project) }}
                </td>
                <td class="hidden text-caption text-ink-subtle xl:table-cell">
                  {{ project.created_at.slice(0, 10) }}
                </td>
                <td class="text-right">
                  <AppIcon name="arrow-right" :size="15" class="text-ink-subtle" />
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </template>
  </div>
</template>
