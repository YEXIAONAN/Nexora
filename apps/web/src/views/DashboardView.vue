<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink, useRouter } from 'vue-router'

import { ApiError } from '@/api/client'
import { listProjects, type Project } from '@/api/projects'
import { pendingReviews, type PendingReview } from '@/api/submissions'
import { getTaskTree, taskStatusLabel, type Task } from '@/api/tasks'
import { listTeamMembers, type TeamMember } from '@/api/teams'
import AppIcon from '@/components/ui/AppIcon.vue'
import { useAuthStore } from '@/stores/auth'
import { useTeamsStore } from '@/stores/teams'
import { useUiStore } from '@/stores/ui'

interface MyTask {
  task: Task
  project: Project
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

function ownerName(task: Task): string {
  return memberNames.value.get(task.owner_id) ?? '未知成员'
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
    for (const entry of trees) {
      if (!entry) continue
      for (const task of entry.tree.tasks) {
        const involved =
          task.owner_id === auth.user?.id || task.collaborator_ids.includes(auth.user?.id ?? '')
        if (involved && task.status !== 'completed' && task.status !== 'cancelled') {
          mine.push({ task, project: entry.project })
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
  <div class="space-y-6">
    <div class="page-header">
      <div>
        <p class="eyebrow">工作台</p>
        <h2 class="mt-1 text-2xl font-semibold tracking-tight text-ink">
          {{ team ? `${team.name}` : '欢迎使用 Nexora' }}
        </h2>
        <p class="page-subtitle">
          {{
            team
              ? '这里汇总你当前团队的待办、审核与项目进展。'
              : '先创建或加入一个团队，开始管理项目与任务。'
          }}
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <button type="button" class="btn" :disabled="!team" @click="goCreateTask">
          <AppIcon name="plus" :size="15" />
          新建任务
        </button>
        <button type="button" class="btn btn-primary" :disabled="!team" @click="goCreateProject">
          <AppIcon name="folder" :size="15" />
          新建项目
        </button>
      </div>
    </div>

    <div v-if="failed" class="card border-danger/30 bg-danger-soft">
      <div class="card-body flex items-center justify-between gap-3">
        <p class="text-sm text-danger">{{ failed }}</p>
        <button type="button" class="btn btn-sm" @click="load">重试</button>
      </div>
    </div>

    <div v-if="!team && !loading" class="card">
      <div class="empty-state">
        <span
          class="flex h-11 w-11 items-center justify-center rounded-full bg-surface-muted text-ink-muted"
        >
          <AppIcon name="users" :size="20" />
        </span>
        <h3 class="mt-3 text-sm font-semibold text-ink">还没有可用的团队</h3>
        <p class="mt-1 max-w-sm text-sm text-ink-muted">
          创建一个团队，或先在「邀请收件箱」接受别人的邀请。团队之间的数据完全隔离。
        </p>
        <div class="mt-4 flex gap-2">
          <button type="button" class="btn btn-primary btn-sm" @click="goTeam">前往团队</button>
          <RouterLink to="/invitations" class="btn btn-sm">查看邀请</RouterLink>
        </div>
      </div>
    </div>

    <template v-else>
      <div class="grid gap-4 sm:grid-cols-3">
        <div class="card">
          <div class="card-body">
            <p class="text-xs font-medium text-ink-subtle">进行中的项目</p>
            <p class="stat mt-2">{{ activeProjects.length }}</p>
            <p class="mt-1 text-xs text-ink-muted">共 {{ projects.length }} 个项目</p>
          </div>
        </div>
        <div class="card">
          <div class="card-body">
            <p class="text-xs font-medium text-ink-subtle">分配给我的待办</p>
            <p class="stat mt-2">{{ openTasks.length }}</p>
            <p class="mt-1 text-xs text-ink-muted">进行中与待开始</p>
          </div>
        </div>
        <div class="card">
          <div class="card-body">
            <p class="text-xs font-medium text-ink-subtle">待我审核</p>
            <p class="stat mt-2">{{ reviews.length }}</p>
            <p class="mt-1 text-xs text-ink-muted">等待处理的成果提交</p>
          </div>
        </div>
      </div>

      <div class="grid gap-6 xl:grid-cols-[minmax(0,1.25fr)_minmax(0,1fr)]">
        <section class="card">
          <div class="card-header">
            <div>
              <h3 class="section-title">我的待办</h3>
              <p class="mt-0.5 text-xs text-ink-muted">按优先级与截止时间排序</p>
            </div>
            <RouterLink to="/my-tasks" class="btn btn-sm">全部任务</RouterLink>
          </div>
          <div v-if="loading" class="space-y-2 p-4">
            <div v-for="i in 4" :key="i" class="h-14 animate-pulse rounded-lg bg-surface-muted" />
          </div>
          <div v-else-if="myTasks.length === 0" class="empty-state">
            <span
              class="flex h-10 w-10 items-center justify-center rounded-full bg-surface-muted text-ink-muted"
            >
              <AppIcon name="check" :size="18" />
            </span>
            <p class="mt-3 text-sm font-medium text-ink">暂时没有分配给你的任务</p>
            <p class="mt-1 text-xs text-ink-muted">创建项目后，把任务分配给自己或团队成员。</p>
            <button type="button" class="btn btn-sm mt-4" @click="goCreateTask">新建任务</button>
          </div>
          <ul v-else class="divide-y divide-line">
            <li v-for="item in myTasks.slice(0, 6)" :key="item.task.id">
              <RouterLink
                :to="taskLink(item)"
                class="flex items-start gap-3 px-5 py-3 transition-colors hover:bg-surface-muted"
              >
                <span
                  class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-accent"
                  aria-hidden="true"
                />
                <div class="min-w-0 flex-1">
                  <p class="truncate text-sm font-medium text-ink">{{ item.task.title }}</p>
                  <p class="mt-0.5 truncate text-xs text-ink-muted">
                    {{ item.project.name }} · 负责人 {{ ownerName(item.task) }}
                  </p>
                </div>
                <div class="flex shrink-0 flex-col items-end gap-1.5">
                  <span class="badge">{{ taskStatusLabel[item.task.status] }}</span>
                  <span v-if="item.task.planned_end_at" class="text-[11px] text-ink-subtle">
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
              <h3 class="section-title">待我审核</h3>
              <p class="mt-0.5 text-xs text-ink-muted">只有被授权的审核人可以处理</p>
            </div>
          </div>
          <div v-if="loading" class="space-y-2 p-4">
            <div v-for="i in 3" :key="i" class="h-14 animate-pulse rounded-lg bg-surface-muted" />
          </div>
          <div v-else-if="reviews.length === 0" class="empty-state">
            <span
              class="flex h-10 w-10 items-center justify-center rounded-full bg-surface-muted text-ink-muted"
            >
              <AppIcon name="inbox" :size="18" />
            </span>
            <p class="mt-3 text-sm font-medium text-ink">没有待审核的成果</p>
            <p class="mt-1 text-xs text-ink-muted">成员提交成果后，被授权的审核人会在这里看到。</p>
          </div>
          <ul v-else class="divide-y divide-line">
            <li v-for="review in reviews.slice(0, 5)" :key="review.submission_version_id">
              <RouterLink
                :to="reviewLink(review)"
                class="flex items-start gap-3 px-5 py-3 transition-colors hover:bg-surface-muted"
              >
                <div class="min-w-0 flex-1">
                  <p class="truncate text-sm font-medium text-ink">{{ review.task_title }}</p>
                  <p class="mt-0.5 truncate text-xs text-ink-muted">
                    {{ review.project_name }} · {{ review.author_name }} · v{{ review.version }}
                  </p>
                </div>
                <AppIcon name="arrow-right" :size="15" class="mt-1 shrink-0 text-ink-subtle" />
              </RouterLink>
            </li>
          </ul>
        </section>
      </div>

      <section class="card">
        <div class="card-header">
          <div>
            <h3 class="section-title">进行中的项目</h3>
            <p class="mt-0.5 text-xs text-ink-muted">进入项目工作区管理任务、成员与贡献</p>
          </div>
          <RouterLink v-if="teamId" :to="`/teams/${teamId}/projects`" class="btn btn-sm"
            >全部项目</RouterLink
          >
        </div>
        <div v-if="loading" class="grid gap-3 p-5 sm:grid-cols-2">
          <div v-for="i in 2" :key="i" class="h-24 animate-pulse rounded-lg bg-surface-muted" />
        </div>
        <div v-else-if="activeProjects.length === 0" class="empty-state">
          <span
            class="flex h-10 w-10 items-center justify-center rounded-full bg-surface-muted text-ink-muted"
          >
            <AppIcon name="folder" :size="18" />
          </span>
          <p class="mt-3 text-sm font-medium text-ink">还没有进行中的项目</p>
          <p class="mt-1 text-xs text-ink-muted">创建第一个项目，然后拆分任务并分配给成员。</p>
          <button type="button" class="btn btn-primary btn-sm mt-4" @click="goCreateProject">
            创建项目
          </button>
        </div>
        <div v-else class="grid gap-3 p-5 sm:grid-cols-2">
          <RouterLink
            v-for="project in activeProjects.slice(0, 4)"
            :key="project.id"
            :to="`/teams/${project.team_id}/projects/${project.id}`"
            class="group rounded-lg border border-line bg-surface p-4 transition-colors hover:border-line-strong hover:bg-surface-muted"
          >
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <p class="truncate text-sm font-medium text-ink">{{ project.name }}</p>
                <p class="mt-1 line-clamp-2 text-xs leading-5 text-ink-muted">
                  {{ project.description || '暂无项目描述' }}
                </p>
              </div>
              <span class="badge shrink-0">
                {{ project.status === 'active' ? '进行中' : '已归档' }}
              </span>
            </div>
            <div class="mt-3 flex items-center justify-between text-[11px] text-ink-subtle">
              <span>开始于 {{ project.created_at.slice(0, 10) }}</span>
              <span
                class="inline-flex items-center gap-1 text-accent-strong opacity-0 transition-opacity group-hover:opacity-100"
              >
                打开 <AppIcon name="arrow-right" :size="12" />
              </span>
            </div>
          </RouterLink>
        </div>
      </section>
    </template>
  </div>
</template>
