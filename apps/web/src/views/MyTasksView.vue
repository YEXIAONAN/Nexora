<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { RouterLink, useRouter } from 'vue-router'

import { ApiError } from '@/api/client'
import { listProjects, type Project } from '@/api/projects'
import { getTaskTree, taskStatusLabel, type Task } from '@/api/tasks'
import { listTeamMembers, type TeamMember } from '@/api/teams'
import AppIcon from '@/components/ui/AppIcon.vue'
import { useAuthStore } from '@/stores/auth'
import { useTeamsStore } from '@/stores/teams'

interface Row {
  task: Task
  project: Project
}

const auth = useAuthStore()
const teams = useTeamsStore()
const router = useRouter()

const loading = ref(true)
const error = ref<string | null>(null)
const rows = ref<Row[]>([])
const projects = ref<Project[]>([])
const members = ref<TeamMember[]>([])
const statusFilter = ref<'open' | 'awaiting_review' | 'all'>('open')
const projectFilter = ref('')
const search = ref('')
let generation = 0
let controller: AbortController | null = null

const teamId = computed(() => teams.currentTeamId)

const filtered = computed(() => {
  const needle = search.value.trim().toLowerCase()
  return rows.value.filter(({ task, project }) => {
    if (projectFilter.value && project.id !== projectFilter.value) return false
    if (
      statusFilter.value === 'open' &&
      (task.status === 'completed' || task.status === 'cancelled')
    ) {
      return false
    }
    if (statusFilter.value === 'awaiting_review' && task.status !== 'awaiting_review') return false
    if (needle && !task.title.toLowerCase().includes(needle)) return false
    return true
  })
})

const memberNames = computed(() => {
  const map = new Map<string, string>()
  for (const member of members.value)
    map.set(member.user_id, member.display_name || member.username)
  return map
})

const priorityLabel: Record<Task['priority'], string> = {
  low: '低',
  normal: '普通',
  high: '高',
  urgent: '紧急',
}

function priorityClass(priority: Task['priority']): string {
  if (priority === 'urgent') return 'border-danger/30 bg-danger-soft text-danger'
  if (priority === 'high') return 'border-warning/30 bg-warning-soft text-warning'
  return 'border-line bg-surface-muted text-ink-muted'
}

function statusClass(status: Task['status']): string {
  if (status === 'awaiting_review' || status === 'completed') {
    return 'border-accent/25 bg-accent-soft text-accent-strong'
  }
  if (status === 'rejected') return 'border-danger/25 bg-danger-soft text-danger'
  if (status === 'in_progress') return 'border-info/25 bg-info-soft text-info'
  return 'border-line bg-surface-muted text-ink-muted'
}

function openTask(row: Row): void {
  void router.push(`/teams/${row.project.team_id}/projects/${row.project.id}?task=${row.task.id}`)
}

async function load(): Promise<void> {
  const current = ++generation
  controller?.abort()
  controller = null
  projects.value = []
  members.value = []
  rows.value = []
  projectFilter.value = ''
  error.value = null
  const currentTeamId = teamId.value
  loading.value = !!currentTeamId
  if (!currentTeamId) return
  controller = new AbortController()
  const { signal } = controller
  try {
    const [projectList, memberList] = await Promise.all([
      listProjects(currentTeamId, signal),
      listTeamMembers(currentTeamId, signal).catch(() => []),
    ])
    if (current !== generation) return
    projects.value = projectList
    members.value = memberList
    const trees = await Promise.all(
      projectList.map((project) =>
        getTaskTree(currentTeamId, project.id, signal)
          .then((tree) => ({ project, tree }))
          .catch((reason: unknown) => {
            if (reason instanceof ApiError && (reason.status === 403 || reason.status === 404)) {
              return null
            }
            throw reason
          }),
      ),
    )
    if (current !== generation) return
    const collected: Row[] = []
    for (const entry of trees) {
      if (!entry) continue
      for (const task of entry.tree.tasks) {
        const involved =
          task.owner_id === auth.user?.id || task.collaborator_ids.includes(auth.user?.id ?? '')
        if (involved) collected.push({ task, project: entry.project })
      }
    }
    collected.sort((a, b) => {
      const rank = { urgent: 0, high: 1, normal: 2, low: 3 }
      return (
        rank[a.task.priority] - rank[b.task.priority] ||
        (a.task.planned_end_at ?? '9999').localeCompare(b.task.planned_end_at ?? '9999')
      )
    })
    rows.value = collected
  } catch (reason) {
    if (current !== generation) return
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    error.value = reason instanceof ApiError ? reason.message : '无法加载任务'
  } finally {
    if (current === generation) loading.value = false
  }
}

onMounted(async () => {
  const current = generation
  if (auth.user) await teams.loadTeams(auth.user.id)
  if (current === generation) await load()
})

watch(teamId, load)
onBeforeUnmount(() => {
  generation++
  controller?.abort()
})
</script>

<template>
  <div class="space-y-5">
    <div class="page-header">
      <div>
        <p class="eyebrow">我的任务</p>
        <h2 class="page-title mt-1">分配给我的工作</h2>
        <p class="page-subtitle">
          {{
            teams.currentTeam ? `当前团队：${teams.currentTeam.name}` : '加入并选择团队后查看任务。'
          }}
        </p>
      </div>
      <div class="flex items-center gap-2">
        <select
          v-model="projectFilter"
          class="form-input h-9 w-auto min-w-[140px] text-xs"
          aria-label="按项目筛选"
        >
          <option value="">全部项目</option>
          <option v-for="project in projects" :key="project.id" :value="project.id">
            {{ project.name }}
          </option>
        </select>
      </div>
    </div>

    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div class="flex rounded-lg border border-line bg-surface p-0.5" role="tablist">
        <button
          type="button"
          class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
          :class="
            statusFilter === 'open'
              ? 'bg-accent-soft text-accent-strong'
              : 'text-ink-muted hover:text-ink'
          "
          @click="statusFilter = 'open'"
        >
          待完成
        </button>
        <button
          type="button"
          class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
          :class="
            statusFilter === 'awaiting_review'
              ? 'bg-accent-soft text-accent-strong'
              : 'text-ink-muted hover:text-ink'
          "
          @click="statusFilter = 'awaiting_review'"
        >
          待验收
        </button>
        <button
          type="button"
          class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
          :class="
            statusFilter === 'all'
              ? 'bg-accent-soft text-accent-strong'
              : 'text-ink-muted hover:text-ink'
          "
          @click="statusFilter = 'all'"
        >
          全部
        </button>
      </div>
      <input
        v-model="search"
        type="search"
        class="form-input h-9 max-w-xs text-xs"
        placeholder="搜索任务标题"
        aria-label="搜索任务"
      />
    </div>

    <div v-if="error" role="alert" class="card border-danger/30">
      <div class="card-body flex items-center justify-between">
        <p class="text-sm text-danger">{{ error }}</p>
        <button type="button" class="btn btn-sm" @click="load">重试</button>
      </div>
    </div>

    <div v-else-if="loading" class="card divide-y divide-line">
      <div v-for="i in 5" :key="i" class="h-16 animate-pulse bg-surface-muted/60" />
    </div>

    <div v-else-if="!teamId" class="card">
      <div class="empty-state">
        <AppIcon name="users" :size="22" class="text-ink-subtle" />
        <p class="mt-3 text-sm font-medium text-ink">还没有团队</p>
        <p class="mt-1 text-xs text-ink-muted">
          请联系团队管理员发送邀请，加入团队后，这里会显示分配给你的任务。
        </p>
        <RouterLink to="/teams" class="btn btn-primary btn-sm mt-4">前往团队</RouterLink>
      </div>
    </div>

    <div v-else-if="filtered.length === 0" class="card">
      <div class="empty-state">
        <AppIcon name="check" :size="22" class="text-ink-subtle" />
        <p class="mt-3 text-sm font-medium text-ink">
          {{ rows.length === 0 ? '暂时没有分配给你的任务' : '没有符合筛选条件的任务' }}
        </p>
        <p class="mt-1 text-xs text-ink-muted">
          {{
            rows.length === 0
              ? '项目负责人可以在项目任务中把你设为负责人或协作成员。'
              : '调整筛选条件后再试。'
          }}
        </p>
        <RouterLink v-if="teamId" :to="`/teams/${teamId}/projects`" class="btn btn-sm mt-4">
          浏览项目
        </RouterLink>
      </div>
    </div>

    <div v-else class="card overflow-hidden">
      <table class="w-full text-left text-sm">
        <thead class="border-b border-line bg-surface-muted/60 text-xs text-ink-subtle">
          <tr>
            <th class="px-4 py-2.5 font-medium">任务</th>
            <th class="hidden px-4 py-2.5 font-medium md:table-cell">项目</th>
            <th class="hidden px-4 py-2.5 font-medium sm:table-cell">优先级</th>
            <th class="hidden px-4 py-2.5 font-medium lg:table-cell">截止时间</th>
            <th class="px-4 py-2.5 font-medium">状态</th>
            <th class="px-4 py-2.5" />
          </tr>
        </thead>
        <tbody class="divide-y divide-line">
          <tr
            v-for="row in filtered"
            :key="row.task.id"
            class="cursor-pointer transition-colors hover:bg-surface-muted"
            @click="openTask(row)"
          >
            <td class="px-4 py-3">
              <p class="max-w-[320px] truncate font-medium text-ink">{{ row.task.title }}</p>
              <p class="mt-0.5 truncate text-xs text-ink-muted">
                负责人 {{ memberNames.get(row.task.owner_id) ?? '未知' }}
              </p>
            </td>
            <td class="hidden px-4 py-3 text-xs text-ink-muted md:table-cell">
              {{ row.project.name }}
            </td>
            <td class="hidden px-4 py-3 sm:table-cell">
              <span class="badge" :class="priorityClass(row.task.priority)">
                {{ priorityLabel[row.task.priority] }}
              </span>
            </td>
            <td class="hidden px-4 py-3 text-xs text-ink-muted lg:table-cell">
              {{ row.task.planned_end_at ? row.task.planned_end_at.slice(0, 10) : '未设置' }}
            </td>
            <td class="px-4 py-3">
              <span class="badge" :class="statusClass(row.task.status)">
                {{ taskStatusLabel[row.task.status] }}
              </span>
            </td>
            <td class="px-4 py-3 text-right">
              <AppIcon name="arrow-right" :size="15" class="text-ink-subtle" />
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
