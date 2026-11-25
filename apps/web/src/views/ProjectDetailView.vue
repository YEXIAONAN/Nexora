<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { ApiError } from '@/api/client'
import * as projectsAPI from '@/api/projects'
import * as tasksAPI from '@/api/tasks'
import { listTeamMembers, type TeamMember } from '@/api/teams'
import ProjectForm from '@/components/projects/ProjectForm.vue'
import TaskEditor from '@/components/projects/TaskEditor.vue'
import TaskDetail from '@/components/projects/TaskDetail.vue'
import ReviewersPanel from '@/components/projects/ReviewersPanel.vue'
import AlertBanner from '@/components/ui/AlertBanner.vue'
import AppDrawer from '@/components/ui/AppDrawer.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import { useTeamsStore } from '@/stores/teams'
import { useUiStore } from '@/stores/ui'
import { formatDateTime } from '@/utils/format'
const taskActions = { ...tasksAPI }
const projectActions = { ...projectsAPI }
const route = useRoute(),
  router = useRouter(),
  teams = useTeamsStore(),
  ui = useUiStore()
const teamId = computed(() => String(route.params.teamId ?? '')),
  projectId = computed(() => String(route.params.projectId ?? ''))
const contextKey = computed(() => `${teamId.value}/${projectId.value}`)
const project = ref<projectsAPI.Project | null>(null),
  members = ref<projectsAPI.ProjectMember[]>([]),
  teamMembers = ref<TeamMember[]>([]),
  milestones = ref<projectsAPI.Milestone[]>([])
const tree = ref<tasksAPI.TaskTree | null>(null),
  snapshots = ref<tasksAPI.Snapshot[]>([]),
  history = ref<tasksAPI.TaskHistory[]>([]),
  reports = ref<tasksAPI.ProgressReport[]>([])
const loading = ref(false),
  busy = ref(false),
  error = ref(''),
  success = ref(''),
  confirmed = ref(''),
  historyLoading = ref(false),
  reportsLoading = ref(false)
const tab = ref('tasks'),
  selectedId = ref(''),
  creatingTask = ref(false),
  editingTask = ref(false),
  memberToAdd = ref(''),
  newLead = ref(''),
  handoffReason = ref('')
const milestoneName = ref(''),
  milestoneDescription = ref(''),
  milestoneDate = ref(''),
  editingMilestone = ref<projectsAPI.Milestone | undefined>()
const projectTabs = [
  { id: 'overview', label: '项目概览' },
  { id: 'tasks', label: '任务树' },
  { id: 'members', label: '项目成员' },
  { id: 'milestones', label: '里程碑' },
  { id: 'history', label: '进度历史' },
  { id: 'settings', label: '项目设置' },
] as const
let generation = 0,
  historyGeneration = 0,
  mutationGeneration = 0,
  reportsGeneration = 0
const ready = computed(
  () => confirmed.value === contextKey.value && !!project.value && !!tree.value && !loading.value,
)
const canManage = computed(() => ready.value && project.value?.can_manage === true)
const selected = computed(() => tree.value?.tasks.find((t) => t.id === selectedId.value))
const completedLeaves = computed(
  () => tree.value?.tasks.filter((t) => t.is_leaf && t.status === 'completed').length ?? 0,
)
const leadName = computed(() => {
  const leadId = project.value?.lead_id
  const lead = members.value.find((m) => m.user_id === leadId)
  return lead?.display_name || lead?.username || '—'
})
const taskRows = computed(() => {
  const list = tree.value?.tasks ?? [],
    ids = new Set(list.map((t) => t.id)),
    out: tasksAPI.Task[] = []
  const walk = (t: tasksAPI.Task) => {
    out.push(t)
    list.filter((c) => c.parent_id === t.id).forEach(walk)
  }
  list.filter((t) => !t.parent_id || !ids.has(t.parent_id)).forEach(walk)
  return out
})
const errorText = (e: unknown) =>
  e instanceof ApiError
    ? `${e.message}${
        e.fields
          ? ' · ' +
            Object.entries(e.fields)
              .map(([k, v]) => `${k}: ${v}`)
              .join('；')
          : ''
      }`
    : '操作失败，请重试'
async function load() {
  const team = teamId.value,
    id = projectId.value,
    key = contextKey.value,
    gen = ++generation
  confirmed.value = ''
  loading.value = true
  error.value = ''
  history.value = []
  historyGeneration++
  historyLoading.value = false
  try {
    const [p, m, tm, ms, tr, ss] = await Promise.all([
      projectsAPI.getProject(team, id),
      projectsAPI.listProjectMembers(team, id),
      listTeamMembers(team),
      projectsAPI.listMilestones(team, id),
      tasksAPI.getTaskTree(team, id),
      tasksAPI.getSnapshots(team, id),
    ])
    if (gen !== generation || key !== contextKey.value) return
    project.value = p
    members.value = m
    teamMembers.value = tm
    milestones.value = ms
    tree.value = tr
    snapshots.value = ss
    confirmed.value = key
    teams.selectTeam(team)
    if (!tr.tasks.some((t) => t.id === selectedId.value))
      selectedId.value =
        typeof route.query.task === 'string' && tr.tasks.some((t) => t.id === route.query.task)
          ? route.query.task
          : (tr.tasks[0]?.id ?? '')
    if (route.query.createTask === '1' && p.can_manage) {
      tab.value = 'tasks'
      creatingTask.value = true
      const query = { ...route.query }
      delete query.createTask
      await router.replace({ query, hash: route.hash })
    } else if (typeof route.query.tab === 'string') {
      const requested = route.query.tab
      if (projectTabs.some((item) => item.id === requested)) {
        tab.value = requested
        const query = { ...route.query }
        delete query.tab
        await router.replace({ query, hash: route.hash })
      }
    }
  } catch (e) {
    if (gen === generation && key === contextKey.value) {
      error.value = errorText(e)
      project.value = null
      tree.value = null
    }
  } finally {
    if (gen === generation && key === contextKey.value) {
      loading.value = false
      void loadTaskReports()
    }
  }
}
async function run(action: (team: string, project: string) => Promise<unknown>) {
  if (!ready.value || busy.value) return
  const team = teamId.value,
    id = projectId.value,
    key = confirmed.value,
    mutation = ++mutationGeneration
  busy.value = true
  error.value = ''
  success.value = ''
  try {
    await action(team, id)
    if (key !== contextKey.value || mutation !== mutationGeneration) return
    creatingTask.value = false
    editingTask.value = false
    await load()
    if (key === contextKey.value && mutation === mutationGeneration && ready.value)
      success.value = '操作已保存，工作区已更新。'
  } catch (e) {
    if (key === contextKey.value && mutation === mutationGeneration) error.value = errorText(e)
  } finally {
    if (key === contextKey.value && mutation === mutationGeneration) busy.value = false
  }
}
function toggleTaskCreation() {
  creatingTask.value = !creatingTask.value
  editingTask.value = false
}
function closeTaskEditor() {
  creatingTask.value = false
  editingTask.value = false
}
function beginTaskEdit() {
  editingTask.value = true
  creatingTask.value = false
}
function saveProject(input: projectsAPI.CreateProject) {
  const p = project.value
  if (!p) return
  const fields = {
    name: input.name,
    description: input.description,
    planned_start_at: input.planned_start_at,
    planned_end_at: input.planned_end_at,
  }
  void run((t, id) => projectsAPI.editProject(t, id, { ...fields, expected_version: p.version }))
}
function saveTask(input: tasksAPI.CreateTask) {
  const task = editingTask.value ? selected.value : undefined
  if (task) {
    const fields = {
      title: input.title,
      description: input.description,
      owner_id: input.owner_id,
      collaborator_ids: input.collaborator_ids,
      priority: input.priority,
      planned_start_at: input.planned_start_at,
      planned_end_at: input.planned_end_at,
    }
    void run((t, id) =>
      tasksAPI.editTask(t, id, task.id, { ...fields, expected_version: task.version }),
    )
  } else
    void run(async (t, id) => {
      const created = await tasksAPI.createTask(t, id, input)
      if (`${t}/${id}` === contextKey.value) selectedId.value = created.id
    })
}
function saveMilestone() {
  const existing = editingMilestone.value,
    input = {
      name: milestoneName.value,
      description: milestoneDescription.value,
      planned_at: milestoneDate.value ? new Date(milestoneDate.value).toISOString() : null,
    }
  void run(async (t, id) => {
    await projectsAPI.saveMilestone(t, id, input, existing)
    if (`${t}/${id}` === contextKey.value) resetMilestone()
  })
}
function resetMilestone() {
  editingMilestone.value = undefined
  milestoneName.value = ''
  milestoneDescription.value = ''
  milestoneDate.value = ''
}
function editMilestone(m: projectsAPI.Milestone) {
  editingMilestone.value = m
  milestoneName.value = m.name
  milestoneDescription.value = m.description
  milestoneDate.value = m.planned_at
    ? new Date(
        new Date(m.planned_at).getTime() - new Date(m.planned_at).getTimezoneOffset() * 60000,
      )
        .toISOString()
        .slice(0, 16)
    : ''
}
async function loadHistory(task: tasksAPI.Task) {
  if (!ready.value) return
  const key = confirmed.value,
    team = teamId.value,
    id = projectId.value,
    gen = ++historyGeneration
  historyLoading.value = true
  history.value = []
  try {
    const h = await tasksAPI.getTaskHistory(team, id, task.id)
    if (gen === historyGeneration && key === contextKey.value && selectedId.value === task.id)
      history.value = h
  } catch (e) {
    if (gen === historyGeneration && key === contextKey.value) error.value = errorText(e)
  } finally {
    if (gen === historyGeneration && key === contextKey.value) historyLoading.value = false
  }
}
async function loadTaskReports(task = selected.value) {
  if (!ready.value || !task?.is_leaf) {
    reports.value = []
    return
  }
  const key = confirmed.value,
    team = teamId.value,
    id = projectId.value,
    taskID = task.id,
    gen = ++reportsGeneration
  reportsLoading.value = true
  try {
    const rows = await tasksAPI.listTaskProgressReports(team, id, taskID)
    if (gen === reportsGeneration && key === contextKey.value && selectedId.value === taskID)
      reports.value = rows
  } catch (e) {
    if (gen === reportsGeneration && key === contextKey.value) error.value = errorText(e)
  } finally {
    if (gen === reportsGeneration && key === contextKey.value) reportsLoading.value = false
  }
}
function submitReport(input: tasksAPI.ProgressReportInput) {
  const task = selected.value
  if (!task) return
  void run((t, p) => tasksAPI.reportTaskProgress(t, p, task, input))
}
function manageMembersFromDrawer() {
  closeTaskEditor()
  tab.value = 'members'
}
async function archiveProject() {
  if (!project.value) return
  const ok = await ui.confirm({
    title: '归档项目',
    message:
      '归档后项目、任务、成员与计划均为只读，历史贡献与验收记录冻结。奖金批次仍可独立继续办理。此操作不可通过普通接口撤销。',
    confirmLabel: '确认归档',
    tone: 'danger',
  })
  if (!ok) return
  const version = project.value.version
  await run((t, p) => projectActions.archiveProject(t, p, version))
}
async function removeProjectMember(member: projectsAPI.ProjectMember) {
  const ok = await ui.confirm({
    title: '移除项目成员',
    message: `确认将 ${member.display_name || member.username} 移出该项目？其项目访问权限立即失效，历史记录保留。`,
    confirmLabel: '确认移除成员',
    tone: 'danger',
  })
  if (!ok) return
  await run((t, p) => projectActions.removeProjectMember(t, p, member.user_id))
}
function priorityLabel(priority: string): string {
  return { low: '低', normal: '普通', high: '高', urgent: '紧急' }[priority] ?? priority
}
function ownerLabel(task: tasksAPI.Task): string {
  const member = members.value.find((m) => m.user_id === task.owner_id)
  return member?.display_name || member?.username || '未分配'
}
function dueLabel(task: tasksAPI.Task): string {
  if (!task.planned_end_at) return ''
  const end = new Date(task.planned_end_at)
  return `${end.getMonth() + 1}/${end.getDate()}`
}
function isOverdue(task: tasksAPI.Task): boolean {
  return (
    !!task.planned_end_at &&
    task.status !== 'completed' &&
    task.status !== 'cancelled' &&
    new Date(task.planned_end_at).getTime() < Date.now()
  )
}
function progressPercent(task: tasksAPI.Task): string {
  if (task.status === 'completed') return '100%'
  return taskActions.percent(task.effective_progress_bps)
}
function listProgressLabel(task: tasksAPI.Task): string {
  switch (task.status) {
    case 'completed':
      return '已验收'
    case 'awaiting_review':
      return '待验收'
    case 'rejected':
      return '已驳回'
    case 'cancelled':
      return '已取消'
    default:
      return progressPercent(task)
  }
}
function statusDot(status: string): string {
  switch (status) {
    case 'in_progress':
      return 'bg-accent'
    case 'awaiting_review':
      return 'bg-warning'
    case 'completed':
      return 'bg-accent'
    case 'rejected':
      return 'bg-danger'
    case 'cancelled':
      return 'bg-ink-subtle'
    default:
      return 'bg-line-strong'
  }
}
watch(selectedId, () => {
  history.value = []
  historyLoading.value = false
  historyGeneration++
  reports.value = []
  reportsLoading.value = false
  reportsGeneration++
  editingTask.value = false
  if (ready.value) void loadTaskReports()
})
watch(
  () => route.query.task,
  (taskId) => {
    if (
      ready.value &&
      typeof taskId === 'string' &&
      tree.value?.tasks.some((task) => task.id === taskId)
    ) {
      selectedId.value = taskId
      tab.value = 'tasks'
    }
  },
)
watch(
  contextKey,
  () => {
    mutationGeneration++
    project.value = null
    tree.value = null
    selectedId.value = ''
    success.value = ''
    busy.value = false
    tab.value = 'tasks'
    creatingTask.value = false
    editingTask.value = false
    memberToAdd.value = ''
    newLead.value = ''
    handoffReason.value = ''
    resetMilestone()
    void load()
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  generation++
  historyGeneration++
  mutationGeneration++
  reportsGeneration++
})
</script>
<template>
  <div class="page-stack">
    <PageHeader
      eyebrow="项目工作区"
      :title="project?.name || '项目'"
      :description="project?.description || '把计划拆成任务，让每一步进展有据可查。'"
      :crumbs="[
        { label: '项目', to: `/teams/${teamId}/projects` },
        { label: project?.name || '项目工作区' },
      ]"
    >
      <template #actions>
        <StatusBadge
          v-if="project"
          :label="project.status === 'archived' ? '已归档 · 只读' : '进行中'"
          :tone="project.status === 'archived' ? 'neutral' : 'success'"
          dot
        />
        <RouterLink class="btn" :to="`/teams/${teamId}/projects/${projectId}/contributions`"
          >贡献核算</RouterLink
        >
        <RouterLink class="btn" :to="`/teams/${teamId}/projects/${projectId}/bonus-rounds`"
          >奖金管理</RouterLink
        >
        <button class="btn" :disabled="busy" @click="load">刷新工作区</button>
      </template>
    </PageHeader>

    <AlertBanner v-if="error" tone="danger" :message="error" />
    <AlertBanner v-if="success && !error" tone="success" :message="success" />
    <div v-if="loading" class="card space-y-2 p-4">
      <div v-for="i in 3" :key="i" class="skeleton h-20" />
    </div>
    <button v-if="!loading && !ready" class="btn" @click="load">重新加载项目</button>
    <template v-if="ready && project && tree">
      <section class="card">
        <div class="card-body">
          <div class="flex flex-wrap items-end justify-between gap-4">
            <div>
              <p class="field-label">项目有效进度</p>
              <strong
                data-testid="project-progress"
                class="mt-1 block text-display font-semibold text-accent-strong"
              >
                {{ taskActions.percent(tree.progress.progress_bps) }}
              </strong>
            </div>
            <dl class="flex flex-wrap gap-x-8 gap-y-2 text-caption">
              <div>
                <dt class="text-ink-subtle">有效叶子任务</dt>
                <dd class="mt-0.5 font-medium text-ink">{{ tree.progress.leaf_count }}</dd>
              </div>
              <div>
                <dt class="text-ink-subtle">已完成</dt>
                <dd class="mt-0.5 font-medium text-ink">{{ completedLeaves }}</dd>
              </div>
              <div>
                <dt class="text-ink-subtle">项目成员</dt>
                <dd class="mt-0.5 font-medium text-ink">{{ members.length }}</dd>
              </div>
              <div>
                <dt class="text-ink-subtle">项目负责人</dt>
                <dd class="mt-0.5 font-medium text-ink">{{ leadName }}</dd>
              </div>
            </dl>
          </div>
          <div class="progress-track mt-4">
            <div class="progress-bar" :style="{ width: `${tree.progress.progress_bps / 100}%` }" />
          </div>
          <p class="mt-2 text-meta text-ink-subtle">
            按叶子工作量汇总，取消任务不参与计算。正式完成需要验收。
          </p>
        </div>
      </section>

      <nav class="tabs" aria-label="项目页面">
        <button
          v-for="item in projectTabs"
          :key="item.id"
          type="button"
          class="tab"
          :class="tab === item.id ? 'tab-active' : ''"
          :aria-pressed="tab === item.id"
          @click="tab = item.id"
        >
          {{ item.label }}
        </button>
      </nav>
      <fieldset :disabled="busy" class="min-w-0 space-y-5">
        <p v-if="busy" role="status" class="text-caption text-ink-muted">正在保存…</p>
        <section v-if="tab === 'overview'" class="space-y-5">
          <div class="grid grid-cols-1 gap-5 lg:grid-cols-2">
            <div class="card">
              <div class="card-header">
                <h3 class="card-title">项目信息</h3>
                <span class="badge">{{
                  project.status === 'archived' ? '已归档 · 只读' : '进行中'
                }}</span>
              </div>
              <dl class="divide-y divide-line">
                <div class="flex items-center justify-between px-5 py-3">
                  <dt class="text-caption text-ink-subtle">项目负责人</dt>
                  <dd class="text-body text-ink">{{ leadName }}</dd>
                </div>
                <div class="flex items-center justify-between px-5 py-3">
                  <dt class="text-caption text-ink-subtle">计划开始</dt>
                  <dd class="text-body text-ink">
                    {{
                      project.planned_start_at ? formatDateTime(project.planned_start_at) : '未设置'
                    }}
                  </dd>
                </div>
                <div class="flex items-center justify-between px-5 py-3">
                  <dt class="text-caption text-ink-subtle">计划结束</dt>
                  <dd class="text-body text-ink">
                    {{ project.planned_end_at ? formatDateTime(project.planned_end_at) : '未设置' }}
                  </dd>
                </div>
                <div class="flex items-center justify-between px-5 py-3">
                  <dt class="text-caption text-ink-subtle">创建时间</dt>
                  <dd class="text-body text-ink">{{ formatDateTime(project.created_at) }}</dd>
                </div>
              </dl>
            </div>
            <div class="card">
              <div class="card-header">
                <h3 class="card-title">当前进展</h3>
              </div>
              <div class="card-body space-y-4">
                <div class="flex items-end justify-between">
                  <div>
                    <p class="text-caption text-ink-subtle">有效进度</p>
                    <p class="mt-1 text-display font-semibold text-ink">
                      {{ taskActions.percent(tree.progress.progress_bps) }}
                    </p>
                  </div>
                  <p class="text-caption text-ink-muted">
                    {{ completedLeaves }} / {{ tree.progress.leaf_count }} 个叶子任务完成
                  </p>
                </div>
                <div class="progress-track">
                  <div
                    class="progress-bar"
                    :style="{ width: `${tree.progress.progress_bps / 100}%` }"
                  />
                </div>
                <div class="grid grid-cols-3 gap-2 pt-2">
                  <div class="rounded-control border border-line p-3 text-center">
                    <p class="text-title font-semibold text-ink">{{ tree.tasks.length }}</p>
                    <p class="text-meta text-ink-subtle">任务</p>
                  </div>
                  <div class="rounded-control border border-line p-3 text-center">
                    <p class="text-title font-semibold text-ink">{{ members.length }}</p>
                    <p class="text-meta text-ink-subtle">项目成员</p>
                  </div>
                  <div class="rounded-control border border-line p-3 text-center">
                    <p class="text-title font-semibold text-ink">{{ milestones.length }}</p>
                    <p class="text-meta text-ink-subtle">里程碑</p>
                  </div>
                </div>
              </div>
            </div>
          </div>
          <div class="card">
            <div class="card-header">
              <div>
                <h3 class="card-title">贡献与奖金</h3>
                <p class="card-description">
                  贡献核算与奖金批次是独立域。项目归档后，奖金仍可继续办理。
                </p>
              </div>
            </div>
            <div class="card-body grid grid-cols-1 gap-3 sm:grid-cols-2">
              <RouterLink
                :to="`/teams/${teamId}/projects/${projectId}/contributions`"
                class="rounded-control border border-line bg-surface p-4 transition-colors hover:border-line-strong hover:bg-surface-hover"
              >
                <p class="text-body font-medium text-ink">贡献核算</p>
                <p class="mt-1 text-caption text-ink-muted">
                  基于已验收成果建立贡献案例，独立核定后作为奖金来源。
                </p>
              </RouterLink>
              <RouterLink
                :to="`/teams/${teamId}/projects/${projectId}/bonus-rounds`"
                class="rounded-control border border-line bg-surface p-4 transition-colors hover:border-line-strong hover:bg-surface-hover"
              >
                <p class="text-body font-medium text-ink">奖金管理</p>
                <p class="mt-1 text-caption text-ink-muted">
                  创建独立资金批次、编制人工方案、独立核定、发布与记录模拟发放。
                </p>
              </RouterLink>
            </div>
          </div>
        </section>
        <template v-if="tab === 'tasks'">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h3 class="card-title">任务</h3>
              <p class="card-description">
                最多三级；只显示你有权访问的任务。点击任务查看工作进展、交付与验收记录。
              </p>
            </div>
            <button v-if="canManage" class="btn btn-primary btn-sm" @click="toggleTaskCreation">
              新建任务
            </button>
          </div>
          <div
            class="grid grid-cols-1 items-start gap-4 xl:grid-cols-[minmax(360px,0.9fr)_minmax(0,1.35fr)]"
          >
            <section class="card overflow-hidden">
              <div class="card-header">
                <h3 class="card-title">任务列表</h3>
                <span class="badge">{{ tree.tasks.length }}</span>
              </div>
              <div v-if="taskRows.length" class="divide-y divide-line">
                <button
                  v-for="t in taskRows"
                  :key="t.id"
                  type="button"
                  class="group flex w-full items-center gap-3 py-2.5 pr-3 text-left transition"
                  :class="selectedId === t.id ? 'bg-accent-soft' : 'hover:bg-surface-muted'"
                  :style="{ paddingLeft: `${12 + (t.depth - 1) * 16}px` }"
                  :aria-pressed="selectedId === t.id"
                  @click="selectedId = t.id"
                >
                  <span
                    class="h-1.5 w-1.5 shrink-0 rounded-full"
                    :class="statusDot(t.status)"
                    aria-hidden="true"
                  />
                  <span class="min-w-0 grow">
                    <span class="flex items-center gap-1.5">
                      <span
                        class="truncate text-sm font-medium"
                        :class="t.status === 'completed' ? 'text-ink-muted' : 'text-ink'"
                        >{{ t.title }}</span
                      >
                      <span
                        v-if="t.priority === 'high' || t.priority === 'urgent'"
                        class="shrink-0 rounded border border-warning/30 bg-warning-soft px-1 py-px text-micro font-medium text-warning"
                        >{{ priorityLabel(t.priority) }}</span
                      >
                      <span v-if="t.blocked" class="shrink-0 text-micro text-warning">阻塞</span>
                    </span>
                    <span
                      class="mt-0.5 flex flex-wrap items-center gap-x-2 text-meta text-ink-subtle"
                    >
                      <span>{{ ownerLabel(t) }}</span>
                      <span :class="isOverdue(t) ? 'text-danger' : ''" v-if="t.planned_end_at"
                        >截止 {{ dueLabel(t) }}</span
                      >
                      <span v-if="t.collaborator_ids.length"
                        >{{ t.collaborator_ids.length }} 协作者</span
                      >
                    </span>
                  </span>
                  <span class="flex shrink-0 items-center gap-2">
                    <span
                      class="hidden h-1 w-14 overflow-hidden rounded-full bg-surface-muted sm:block"
                    >
                      <span
                        class="block h-full rounded-full"
                        :class="t.status === 'completed' ? 'bg-accent' : 'bg-accent/60'"
                        :style="{ width: progressPercent(t) }"
                      />
                    </span>
                    <span
                      class="w-14 text-right text-meta font-medium"
                      :class="
                        selectedId === t.id
                          ? 'text-accent-strong'
                          : t.status === 'rejected'
                            ? 'text-danger'
                            : 'text-ink-muted'
                      "
                      >{{ listProgressLabel(t) }}</span
                    >
                  </span>
                </button>
              </div>
              <EmptyState
                v-else
                icon="check"
                title="还没有任务"
                description="项目负责人可以创建任务，并从项目成员中选择负责人和协作者。"
              />
            </section>
            <section v-if="selected" class="card min-w-0 overflow-hidden xl:sticky xl:top-4">
              <TaskDetail
                :key="`${selected.id}:${selected.version}`"
                :team="teamId"
                :project="projectId"
                :task="selected"
                :tasks="tree.tasks"
                :dependencies="tree.dependencies"
                :members="members"
                :team-members="teamMembers"
                :can-manage="canManage"
                :history="history"
                :history-loading="historyLoading"
                :reports="reports"
                :reports-loading="reportsLoading"
                :busy="busy"
                @edit="beginTaskEdit"
                @start="
                  (task) => run((t, p) => taskActions.transitionTask(t, p, task, 'in_progress'))
                "
                @cancel="
                  (task, reason) =>
                    run((t, p) => taskActions.transitionTask(t, p, task, 'cancelled', reason))
                "
                @weight="
                  (task, weight, reason) =>
                    run((t, p) => taskActions.changeTaskWeight(t, p, task, weight, reason))
                "
                @move="(task, parent) => run((t, p) => taskActions.moveTask(t, p, task, parent))"
                @dependency="
                  (task, predecessor) =>
                    run((t, p) => taskActions.addDependency(t, p, task, predecessor))
                "
                @ignore="
                  (task, dep, reason) =>
                    run((t, p) => taskActions.ignoreDependency(t, p, task, dep, reason))
                "
                @history="loadHistory"
                @report="submitReport"
                @refresh-reports="loadTaskReports()"
                @changed="load"
              />
            </section>
            <div v-else class="card">
              <EmptyState
                icon="layers"
                title="选择一个任务"
                description="任务详情会显示执行进度、工作进展记录、交付验收与活动历史。"
              />
            </div>
          </div>
        </template>
        <section v-if="tab === 'members'" class="card">
          <div class="card-header">
            <div>
              <h3 class="card-title">项目成员</h3>
              <p class="card-description">
                任务负责人和协作者必须同时具有有效的 Team 与项目成员资格。
              </p>
            </div>
            <span class="badge">{{ members.length }} 名</span>
          </div>
          <div class="card-body space-y-4">
            <ReviewersPanel
              :team="teamId"
              :project="projectId"
              :can-manage="canManage"
              :members="members"
            />
            <div
              v-for="m in members"
              :key="m.user_id"
              class="flex flex-wrap items-center gap-3 border-b border-line pb-3 last:border-0 last:pb-0"
            >
              <span class="grow text-body text-ink"
                >{{ m.display_name || m.username }}
                <span class="text-ink-subtle">@{{ m.username }}</span></span
              ><StatusBadge
                :label="
                  (m.role === 'lead' ? '项目负责人' : '参与者') + (m.active ? '' : ' · 已失效')
                "
                :tone="m.active ? 'neutral' : 'danger'"
              /><button
                v-if="canManage && m.role !== 'lead' && m.active"
                class="btn btn-sm btn-danger"
                @click="removeProjectMember(m)"
              >
                移除
              </button>
            </div>
            <form
              v-if="canManage"
              class="flex flex-wrap items-end gap-2"
              @submit.prevent="run((t, p) => projectActions.addProjectMember(t, p, memberToAdd))"
            >
              <label class="form-label min-w-[220px] flex-1"
                >添加 Team 成员<select
                  aria-label="添加 Team 成员"
                  v-model="memberToAdd"
                  required
                  class="form-select"
                >
                  <option value="" disabled>请选择</option>
                  <option
                    v-for="m in teamMembers.filter(
                      (m) => !members.some((pm) => pm.user_id === m.user_id && pm.active),
                    )"
                    :key="m.user_id"
                    :value="m.user_id"
                  >
                    {{ m.display_name || m.username }}
                  </option>
                </select></label
              ><button class="btn btn-primary" type="submit">添加成员</button>
            </form>
          </div>
        </section>
        <section v-if="tab === 'milestones'" class="card">
          <div class="card-header"><h3 class="card-title">里程碑计划</h3></div>
          <div class="card-body space-y-5">
            <p v-if="!milestones.length" class="text-caption text-ink-muted">暂无里程碑。</p>
            <div
              v-for="m in milestones"
              :key="m.id"
              class="flex items-start gap-4 border-b border-line pb-4 last:border-0 last:pb-0"
            >
              <div class="grow">
                <h4 class="text-body font-medium">{{ m.name }}</h4>
                <p class="mt-1 text-caption text-ink-muted">{{ m.description }}</p>
                <p class="mt-2 text-caption text-ink-subtle">
                  {{ m.planned_at ? formatDateTime(m.planned_at) : '未设置日期' }}
                </p>
              </div>
              <button v-if="canManage" class="btn btn-sm" @click="editMilestone(m)">
                编辑里程碑
              </button>
            </div>
            <form
              v-if="canManage"
              class="grid gap-3 sm:grid-cols-2"
              @submit.prevent="saveMilestone"
            >
              <label class="form-label"
                >里程碑名称<input
                  v-model="milestoneName"
                  required
                  maxlength="120"
                  class="form-input" /></label
              ><label class="form-label"
                >计划时间<input
                  v-model="milestoneDate"
                  type="datetime-local"
                  class="form-input" /></label
              ><label class="form-label sm:col-span-2"
                >里程碑说明<textarea
                  v-model="milestoneDescription"
                  maxlength="4000"
                  class="form-textarea"
                /></label
              ><button class="btn btn-primary" type="submit">
                {{ editingMilestone ? '保存里程碑' : '添加里程碑' }}</button
              ><button v-if="editingMilestone" class="btn" type="button" @click="resetMilestone">
                取消编辑
              </button>
            </form>
          </div>
        </section>
        <section v-if="tab === 'history'" class="card">
          <div class="card-header">
            <h3 class="card-title">进度历史</h3>
            <span class="text-caption text-ink-subtle">最近 200 条</span>
          </div>
          <p v-if="!snapshots.length" class="card-body text-caption text-ink-muted">
            暂无任务变更快照。
          </p>
          <ol class="divide-y divide-line">
            <li
              v-for="s in snapshots"
              :key="s.id"
              class="flex items-center justify-between gap-4 px-5 py-4"
            >
              <div>
                <p class="text-body">{{ s.event_type }}</p>
                <p class="mt-1 text-caption text-ink-subtle">
                  {{ formatDateTime(s.created_at) }} · 有效叶子 {{ s.progress.leaf_count }} · 权重
                  {{ s.progress.total_weight }}
                </p>
              </div>
              <strong class="text-accent-strong">{{
                taskActions.percent(s.progress.progress_bps)
              }}</strong>
            </li>
          </ol>
        </section>
        <div v-if="tab === 'settings'" class="space-y-5">
          <section v-if="canManage" class="card">
            <div class="card-header"><h3 class="card-title">编辑项目</h3></div>
            <div class="card-body">
              <ProjectForm
                :key="project.version"
                :project="project"
                :members="teamMembers"
                @save="saveProject"
              />
            </div>
          </section>
          <section v-if="project.can_handoff" class="card">
            <div class="card-header"><h3 class="card-title">负责人交接</h3></div>
            <form
              class="card-body grid gap-3 sm:grid-cols-2"
              @submit.prevent="
                run((t, p) =>
                  projectActions.handoffProject(t, p, {
                    lead_id: newLead,
                    reason: handoffReason,
                    expected_version: project!.version,
                  }),
                )
              "
            >
              <label class="form-label"
                >新负责人<select
                  aria-label="新负责人"
                  v-model="newLead"
                  required
                  class="form-select"
                >
                  <option value="" disabled>请选择</option>
                  <option
                    v-for="m in teamMembers.filter((m) => m.user_id !== project?.lead_id)"
                    :key="m.user_id"
                    :value="m.user_id"
                  >
                    {{ m.display_name || m.username }}
                  </option>
                </select></label
              ><label class="form-label"
                >交接原因<input
                  v-model="handoffReason"
                  required
                  maxlength="1000"
                  class="form-input" /></label
              ><button class="btn" type="submit">记录交接</button>
            </form>
          </section>
          <section v-if="canManage" class="card border-warning/40">
            <div class="card-header">
              <div>
                <h3 class="card-title">归档项目</h3>
                <p class="card-description">
                  归档后项目、任务、成员及计划均为只读。请先完成负责人和人员安排；奖金批次仍可继续办理。
                </p>
              </div>
            </div>
            <div class="card-body">
              <button class="btn btn-danger" @click="archiveProject">归档并设为只读</button>
            </div>
          </section>
          <p v-if="!canManage && !project.can_handoff" class="card">
            <EmptyState
              icon="shield"
              :title="
                project.status === 'archived'
                  ? '此项目已归档，所有内容只读'
                  : '仅项目负责人可以编辑项目设置'
              "
              :description="
                project.status === 'archived'
                  ? '奖金批次作为独立资金域仍可继续查看和办理。'
                  : '你可以在项目成员页查看自己的项目角色。'
              "
            />
          </p>
        </div>
      </fieldset>
    </template>

    <AppDrawer
      :open="ready && (creatingTask || editingTask)"
      :title="editingTask ? '编辑任务' : '新建任务'"
      eyebrow="任务"
      :width="'max-w-[620px]'"
      @close="closeTaskEditor"
    >
      <TaskEditor
        :key="editingTask ? (selected?.id ?? 'edit') : 'new'"
        :task="editingTask ? selected : undefined"
        :tasks="tree?.tasks ?? []"
        :members="members"
        :team-members="teamMembers"
        :default-owner="project?.lead_id ?? ''"
        :can-manage-members="canManage"
        @save="saveTask"
        @close="closeTaskEditor"
        @manage-members="manageMembersFromDrawer"
      />
    </AppDrawer>
  </div>
</template>
