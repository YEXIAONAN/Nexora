<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { ApiError } from '@/api/client'
import * as projectsAPI from '@/api/projects'
import * as tasksAPI from '@/api/tasks'
import { listTeamMembers, type TeamMember } from '@/api/teams'
import ProjectForm from '@/components/projects/ProjectForm.vue'
import TaskEditor from '@/components/projects/TaskEditor.vue'
import TaskPanel from '@/components/projects/TaskPanel.vue'
import SubmissionPanel from '@/components/projects/SubmissionPanel.vue'
import ReviewersPanel from '@/components/projects/ReviewersPanel.vue'
import { useTeamsStore } from '@/stores/teams'
import { formatDateTime } from '@/utils/format'
const taskActions = { ...tasksAPI }
const projectActions = { ...projectsAPI }
const route = useRoute(),
  teams = useTeamsStore()
const teamId = computed(() => String(route.params.teamId ?? '')),
  projectId = computed(() => String(route.params.projectId ?? ''))
const contextKey = computed(() => `${teamId.value}/${projectId.value}`)
const project = ref<projectsAPI.Project | null>(null),
  members = ref<projectsAPI.ProjectMember[]>([]),
  teamMembers = ref<TeamMember[]>([]),
  milestones = ref<projectsAPI.Milestone[]>([])
const tree = ref<tasksAPI.TaskTree | null>(null),
  snapshots = ref<tasksAPI.Snapshot[]>([]),
  history = ref<tasksAPI.TaskHistory[]>([])
const loading = ref(false),
  busy = ref(false),
  error = ref(''),
  confirmed = ref(''),
  historyLoading = ref(false)
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
let generation = 0,
  historyGeneration = 0,
  mutationGeneration = 0
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
    if (typeof route.query.task === 'string' && tr.tasks.some((t) => t.id === route.query.task))
      selectedId.value = route.query.task
    else if (!tr.tasks.some((t) => t.id === selectedId.value))
      selectedId.value = tr.tasks[0]?.id ?? ''
    if (route.query.createTask === '1' && p.can_manage) {
      tab.value = 'tasks'
      creatingTask.value = true
    }
  } catch (e) {
    if (gen === generation && key === contextKey.value) {
      error.value = errorText(e)
      project.value = null
      tree.value = null
    }
  } finally {
    if (gen === generation && key === contextKey.value) loading.value = false
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
  try {
    await action(team, id)
    if (key !== contextKey.value || mutation !== mutationGeneration) return
    creatingTask.value = false
    editingTask.value = false
    await load()
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
  } else void run((t, id) => tasksAPI.createTask(t, id, input))
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
watch(selectedId, () => {
  history.value = []
  historyLoading.value = false
  historyGeneration++
  editingTask.value = false
})
watch(
  contextKey,
  () => {
    mutationGeneration++
    project.value = null
    tree.value = null
    selectedId.value = ''
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
})
</script>
<template>
  <div class="space-y-5">
    <RouterLink :to="{ name: 'projects', params: { teamId } }" class="text-sm text-accent-strong"
      >← 项目列表</RouterLink
    >
    <p
      v-if="error"
      role="alert"
      class="rounded-lg border border-danger/30 bg-danger-soft p-3 text-sm text-danger"
    >
      {{ error }}
    </p>
    <p v-if="loading" role="status" class="card p-8 text-sm text-ink-muted">正在加载项目工作区…</p>
    <button v-if="!loading && !ready" class="btn" @click="load">重新加载项目</button>
    <template v-if="ready && project && tree">
      <section class="card p-5 sm:p-6">
        <div class="flex flex-wrap justify-between gap-4">
          <div>
            <div class="flex items-center gap-2">
              <span class="badge">{{
                project.status === 'archived' ? '已归档 · 只读' : '进行中'
              }}</span
              ><span v-if="project.can_manage" class="text-xs text-accent-strong">项目负责人</span>
            </div>
            <h2 class="mt-3 text-2xl font-semibold">{{ project.name }}</h2>
            <p class="mt-2 max-w-3xl whitespace-pre-wrap text-sm leading-6 text-ink-muted">
              {{ project.description || '暂无项目描述' }}
            </p>
          </div>
          <button class="btn self-start" :disabled="busy" @click="load">刷新工作区</button>
        </div>
        <div class="mt-6 flex items-baseline justify-between">
          <p class="text-xs font-medium text-ink-muted">
            项目有效进度 · {{ tree.progress.leaf_count }} 个有效叶子任务
          </p>
          <strong data-testid="project-progress" class="text-xl text-accent-strong">{{
            taskActions.percent(tree.progress.progress_bps)
          }}</strong>
        </div>
        <div class="mt-2 h-2 overflow-hidden rounded-full bg-surface-muted">
          <div
            class="h-full rounded-full bg-accent"
            :style="{ width: `${tree.progress.progress_bps / 100}%` }"
          />
        </div>
        <p class="mt-2 text-xs text-ink-subtle">
          按叶子工作量汇总，取消任务不参与计算。正式完成需要验收。
        </p>
      </section>
      <div class="flex flex-wrap gap-2 border-b border-line pb-3">
        <RouterLink class="btn" :to="`/teams/${teamId}/projects/${projectId}/contributions`"
          >贡献核算</RouterLink
        >
        <button
          v-for="item in [
            { id: 'overview', label: '项目概览' },
            { id: 'tasks', label: '任务树' },
            { id: 'members', label: '项目成员' },
            { id: 'milestones', label: '里程碑' },
            { id: 'history', label: '进度历史' },
            { id: 'settings', label: '项目设置' },
          ]"
          :key="item.id"
          class="btn"
          :class="tab === item.id ? 'border-accent bg-accent-soft text-accent-strong' : 'btn-ghost'"
          :aria-pressed="tab === item.id"
          @click="tab = item.id"
        >
          {{ item.label }}
        </button>
      </div>
      <fieldset :disabled="busy" class="min-w-0 space-y-5">
        <p v-if="busy" role="status" class="text-sm text-ink-muted">正在保存…</p>
        <section v-if="tab === 'overview'" class="grid gap-5 lg:grid-cols-2">
          <div class="card">
            <div class="card-header">
              <h3 class="text-sm font-semibold">项目信息</h3>
              <span class="badge">{{
                project.status === 'archived' ? '已归档 · 只读' : '进行中'
              }}</span>
            </div>
            <dl class="divide-y divide-line">
              <div class="flex items-center justify-between px-5 py-3">
                <dt class="text-xs text-ink-subtle">项目负责人</dt>
                <dd class="text-sm text-ink">{{ leadName }}</dd>
              </div>
              <div class="flex items-center justify-between px-5 py-3">
                <dt class="text-xs text-ink-subtle">计划开始</dt>
                <dd class="text-sm text-ink">
                  {{
                    project.planned_start_at ? formatDateTime(project.planned_start_at) : '未设置'
                  }}
                </dd>
              </div>
              <div class="flex items-center justify-between px-5 py-3">
                <dt class="text-xs text-ink-subtle">计划结束</dt>
                <dd class="text-sm text-ink">
                  {{ project.planned_end_at ? formatDateTime(project.planned_end_at) : '未设置' }}
                </dd>
              </div>
              <div class="flex items-center justify-between px-5 py-3">
                <dt class="text-xs text-ink-subtle">创建时间</dt>
                <dd class="text-sm text-ink">{{ formatDateTime(project.created_at) }}</dd>
              </div>
            </dl>
          </div>
          <div class="card">
            <div class="card-header">
              <h3 class="text-sm font-semibold">当前进展</h3>
            </div>
            <div class="card-body space-y-4">
              <div class="flex items-end justify-between">
                <div>
                  <p class="text-xs text-ink-subtle">有效进度</p>
                  <p class="mt-1 text-2xl font-semibold text-ink">
                    {{ taskActions.percent(tree.progress.progress_bps) }}
                  </p>
                </div>
                <p class="text-xs text-ink-muted">
                  {{ completedLeaves }} / {{ tree.progress.leaf_count }} 个叶子任务完成
                </p>
              </div>
              <div class="h-2 overflow-hidden rounded-full bg-surface-muted">
                <div
                  class="h-full rounded-full bg-accent"
                  :style="{ width: `${tree.progress.progress_bps / 100}%` }"
                />
              </div>
              <div class="grid grid-cols-3 gap-2 pt-2">
                <div class="rounded-lg border border-line p-3 text-center">
                  <p class="text-lg font-semibold text-ink">{{ tree.tasks.length }}</p>
                  <p class="text-[11px] text-ink-subtle">任务</p>
                </div>
                <div class="rounded-lg border border-line p-3 text-center">
                  <p class="text-lg font-semibold text-ink">{{ members.length }}</p>
                  <p class="text-[11px] text-ink-subtle">项目成员</p>
                </div>
                <div class="rounded-lg border border-line p-3 text-center">
                  <p class="text-lg font-semibold text-ink">{{ milestones.length }}</p>
                  <p class="text-[11px] text-ink-subtle">里程碑</p>
                </div>
              </div>
              <RouterLink
                :to="`/teams/${teamId}/projects/${projectId}/contributions`"
                class="btn btn-sm w-full"
              >
                查看贡献核算
              </RouterLink>
            </div>
          </div>
        </section>
        <template v-if="tab === 'tasks'">
          <div class="flex items-center justify-between">
            <p class="text-xs text-ink-muted">最多三级；只显示你有权访问的任务。</p>
            <button v-if="canManage" class="btn btn-primary" @click="toggleTaskCreation">
              新建任务
            </button>
          </div>
          <TaskEditor
            v-if="creatingTask || editingTask"
            :key="editingTask ? selected?.id : 'new'"
            :task="editingTask ? selected : undefined"
            :tasks="tree.tasks"
            :members="members"
            :default-owner="project.lead_id"
            @save="saveTask"
            @close="closeTaskEditor"
          />
          <div class="grid items-start gap-5 xl:grid-cols-[minmax(260px,0.8fr)_minmax(0,1.2fr)]">
            <section class="card overflow-hidden">
              <div class="card-header">
                <h3 class="text-sm font-semibold">任务树</h3>
                <span class="badge">{{ tree.tasks.length }}</span>
              </div>
              <div v-if="taskRows.length" class="divide-y divide-line">
                <button
                  v-for="t in taskRows"
                  :key="t.id"
                  type="button"
                  class="flex w-full items-start gap-3 py-4 pr-4 text-left transition hover:bg-surface-muted"
                  :class="selectedId === t.id ? 'bg-accent-soft' : ''"
                  :style="{ paddingLeft: `${16 + (t.depth - 1) * 20}px` }"
                  :aria-pressed="selectedId === t.id"
                  @click="selectedId = t.id"
                >
                  <span class="mt-0.5 text-xs text-ink-subtle">{{ t.is_leaf ? '○' : '▾' }}</span
                  ><span class="min-w-0 grow"
                    ><span class="block truncate text-sm font-medium">{{ t.title }}</span
                    ><span class="mt-1 block text-xs text-ink-muted"
                      >{{ taskActions.taskStatusLabel[t.status] }} ·
                      {{ taskActions.percent(t.effective_progress_bps) }}
                      <span v-if="t.blocked" class="text-warning">· 阻塞</span></span
                    ></span
                  ><span class="badge shrink-0">{{ t.depth }} 级</span>
                </button>
              </div>
              <p v-else class="p-8 text-sm text-ink-muted">暂无有权访问的任务。</p>
            </section>
            <div v-if="selected" class="min-w-0 space-y-5">
              <TaskPanel
                v-if="selected"
                :key="`${selected.id}:${selected.version}`"
                :task="selected"
                :tasks="tree.tasks"
                :dependencies="tree.dependencies"
                :members="members"
                :can-manage="canManage"
                :history="history"
                :history-loading="historyLoading"
                @edit="beginTaskEdit"
                @start="
                  (task) => run((t, p) => taskActions.transitionTask(t, p, task, 'in_progress'))
                "
                @cancel="
                  (task, reason) =>
                    run((t, p) => taskActions.transitionTask(t, p, task, 'cancelled', reason))
                "
                @progress="
                  (task, actual) =>
                    run((t, p) => taskActions.recordTaskProgress(t, p, task, actual))
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
              />
              <SubmissionPanel
                v-if="selected.is_leaf"
                :key="`submission:${teamId}:${projectId}:${selected.id}:${selected.version}`"
                :team="teamId"
                :project="projectId"
                :task="selected"
                @changed="load"
              />
            </div>
          </div>
        </template>
        <section v-if="tab === 'members'" class="card">
          <div class="card-header"><h3 class="font-semibold">项目成员</h3></div>
          <div class="card-body space-y-4">
            <p class="text-xs text-ink-muted">
              任务负责人和协作者必须同时具有有效的 Team 与项目成员资格。
            </p>
            <ReviewersPanel
              :team="teamId"
              :project="projectId"
              :can-manage="canManage"
              :members="members"
            />
            <div
              v-for="m in members"
              :key="m.user_id"
              class="flex items-center gap-3 border-b border-line pb-3"
            >
              <span class="grow text-sm"
                >{{ m.display_name || m.username }}
                <span class="text-ink-subtle">@{{ m.username }}</span></span
              ><span class="badge"
                >{{ m.role === 'lead' ? '项目负责人' : '参与者'
                }}{{ m.active ? '' : ' · 已失效' }}</span
              ><button
                v-if="canManage && m.role !== 'lead' && m.active"
                class="btn text-danger"
                @click="run((t, p) => projectActions.removeProjectMember(t, p, m.user_id))"
              >
                移除
              </button>
            </div>
            <form
              v-if="canManage"
              class="flex items-end gap-2"
              @submit.prevent="run((t, p) => projectActions.addProjectMember(t, p, memberToAdd))"
            >
              <label class="form-label grow"
                >添加 Team 成员<select
                  aria-label="添加 Team 成员"
                  v-model="memberToAdd"
                  required
                  class="form-input"
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
          <div class="card-header"><h3 class="font-semibold">里程碑计划</h3></div>
          <div class="card-body space-y-5">
            <p v-if="!milestones.length" class="text-sm text-ink-muted">暂无里程碑。</p>
            <div
              v-for="m in milestones"
              :key="m.id"
              class="flex items-start gap-4 border-b border-line pb-4"
            >
              <div class="grow">
                <h4 class="font-medium">{{ m.name }}</h4>
                <p class="mt-1 text-sm text-ink-muted">{{ m.description }}</p>
                <p class="mt-2 text-xs text-ink-subtle">
                  {{ m.planned_at ? formatDateTime(m.planned_at) : '未设置日期' }}
                </p>
              </div>
              <button v-if="canManage" class="btn" @click="editMilestone(m)">编辑里程碑</button>
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
                  class="form-input"
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
            <h3 class="font-semibold">进度历史</h3>
            <span class="text-xs text-ink-subtle">最近 200 条</span>
          </div>
          <p v-if="!snapshots.length" class="card-body text-sm text-ink-muted">
            暂无任务变更快照。
          </p>
          <ol class="divide-y divide-line">
            <li
              v-for="s in snapshots"
              :key="s.id"
              class="flex items-center justify-between gap-4 px-5 py-4"
            >
              <div>
                <p class="text-sm">{{ s.event_type }}</p>
                <p class="mt-1 text-xs text-ink-subtle">
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
            <div class="card-header"><h3 class="font-semibold">编辑项目</h3></div>
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
            <div class="card-header"><h3 class="font-semibold">负责人交接</h3></div>
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
                  class="form-input"
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
          <section v-if="canManage" class="card border-warning/40 p-5">
            <h3 class="font-semibold">归档项目</h3>
            <p class="mt-2 text-sm text-ink-muted">
              归档后项目、任务、成员及计划均为只读。请先完成负责人和人员安排。
            </p>
            <button
              class="btn mt-4 text-warning"
              @click="run((t, p) => projectActions.archiveProject(t, p, project!.version))"
            >
              归档并设为只读
            </button>
          </section>
          <p v-if="!canManage && !project.can_handoff" class="card p-6 text-sm text-ink-muted">
            {{
              project.status === 'archived'
                ? '此项目已归档，所有内容只读。'
                : '仅项目负责人可以编辑项目设置。'
            }}
          </p>
        </div>
      </fieldset>
    </template>
  </div>
</template>
