<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { RouterLink, useRouter } from 'vue-router'

import { ApiError } from '@/api/client'
import { listProjects, type Project } from '@/api/projects'
import { getTaskTree, taskStatusLabel, type Task } from '@/api/tasks'
import { listTeamMembers, type TeamMember } from '@/api/teams'
import AlertBanner from '@/components/ui/AlertBanner.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import SegmentedControl from '@/components/ui/SegmentedControl.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
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

const priorityTone: Record<Task['priority'], 'neutral' | 'warning' | 'danger'> = {
  low: 'neutral',
  normal: 'neutral',
  high: 'warning',
  urgent: 'danger',
}

const statusTone: Record<Task['status'], 'neutral' | 'success' | 'danger' | 'warning' | 'info'> = {
  pending: 'neutral',
  in_progress: 'info',
  awaiting_review: 'warning',
  completed: 'success',
  rejected: 'danger',
  cancelled: 'neutral',
}

const filterOptions = computed(() => [
  {
    value: 'open',
    label: '待完成',
    count: rows.value.filter(
      (row) => row.task.status !== 'completed' && row.task.status !== 'cancelled',
    ).length,
  },
  {
    value: 'awaiting_review',
    label: '待验收',
    count: rows.value.filter((row) => row.task.status === 'awaiting_review').length,
  },
  { value: 'all', label: '全部', count: rows.value.length },
])

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
  <div class="page-stack">
    <PageHeader
      eyebrow="我的任务"
      title="分配给我的工作"
      :description="
        teams.currentTeam ? `当前团队：${teams.currentTeam.name}` : '加入并选择团队后查看任务。'
      "
    >
      <template #actions>
        <select
          v-model="projectFilter"
          class="form-select h-9 w-auto min-w-[150px] py-0"
          aria-label="按项目筛选"
        >
          <option value="">全部项目</option>
          <option v-for="project in projects" :key="project.id" :value="project.id">
            {{ project.name }}
          </option>
        </select>
      </template>
    </PageHeader>

    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <SegmentedControl v-model="statusFilter" :options="filterOptions" aria-label="任务状态筛选" />
      <input
        v-model="search"
        type="search"
        class="form-input h-9 max-w-xs"
        placeholder="搜索任务标题"
        aria-label="搜索任务"
      />
    </div>

    <AlertBanner v-if="error" tone="danger" :message="error">
      <template #action>
        <button type="button" class="btn btn-sm" @click="load">重试</button>
      </template>
    </AlertBanner>

    <div v-else-if="loading" class="card divide-y divide-line">
      <div v-for="i in 5" :key="i" class="h-16 animate-pulse bg-surface-muted/60" />
    </div>

    <div v-else-if="!teamId" class="card">
      <EmptyState
        icon="users"
        title="还没有团队"
        description="请联系团队管理员发送邀请，加入团队后，这里会显示分配给你的任务。"
      >
        <template #action>
          <RouterLink to="/teams" class="btn btn-primary btn-sm">前往团队</RouterLink>
        </template>
      </EmptyState>
    </div>

    <div v-else-if="filtered.length === 0" class="card">
      <EmptyState
        icon="check"
        :title="rows.length === 0 ? '暂时没有分配给你的任务' : '没有符合筛选条件的任务'"
        :description="
          rows.length === 0
            ? '项目负责人可以在项目任务中把你设为负责人或协作成员。'
            : '调整筛选条件后再试。'
        "
      >
        <template #action>
          <RouterLink v-if="teamId" :to="`/teams/${teamId}/projects`" class="btn btn-sm">
            浏览项目
          </RouterLink>
        </template>
      </EmptyState>
    </div>

    <div v-else class="card overflow-hidden">
      <div class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>任务</th>
              <th class="hidden md:table-cell">项目</th>
              <th class="hidden sm:table-cell">优先级</th>
              <th class="hidden lg:table-cell">截止时间</th>
              <th>状态</th>
              <th />
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="row in filtered"
              :key="row.task.id"
              class="row-clickable"
              @click="openTask(row)"
            >
              <td>
                <p class="max-w-[320px] truncate font-medium text-ink">{{ row.task.title }}</p>
                <p class="mt-0.5 truncate text-caption text-ink-muted">
                  负责人 {{ memberNames.get(row.task.owner_id) ?? '未知' }}
                </p>
              </td>
              <td class="hidden text-caption text-ink-muted md:table-cell">
                {{ row.project.name }}
              </td>
              <td class="hidden sm:table-cell">
                <StatusBadge
                  :label="priorityLabel[row.task.priority]"
                  :tone="priorityTone[row.task.priority]"
                />
              </td>
              <td class="hidden text-caption text-ink-muted lg:table-cell">
                {{ row.task.planned_end_at ? row.task.planned_end_at.slice(0, 10) : '未设置' }}
              </td>
              <td>
                <StatusBadge
                  :label="taskStatusLabel[row.task.status]"
                  :tone="statusTone[row.task.status]"
                />
              </td>
              <td class="text-right">
                <AppIcon name="arrow-right" :size="15" class="text-ink-subtle" />
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
