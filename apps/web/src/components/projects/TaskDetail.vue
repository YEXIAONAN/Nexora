<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { ProjectMember } from '@/api/projects'
import type { TeamMember } from '@/api/teams'
import {
  canSelectParent,
  percent,
  taskStatusLabel,
  type Dependency,
  type ProgressReport,
  type ProgressReportInput,
  type Task,
  type TaskHistory,
} from '@/api/tasks'
import ProgressReportPanel from '@/components/projects/ProgressReportPanel.vue'
import SubmissionPanel from '@/components/projects/SubmissionPanel.vue'
import { formatDateTime } from '@/utils/format'

const props = withDefaults(
  defineProps<{
    team: string
    project: string
    task: Task
    tasks: Task[]
    dependencies: Dependency[]
    members: ProjectMember[]
    teamMembers: TeamMember[]
    canManage: boolean
    history: TaskHistory[]
    historyLoading: boolean
    reports: ProgressReport[]
    reportsLoading: boolean
    busy: boolean
  }>(),
  {
    tasks: () => [],
    dependencies: () => [],
    members: () => [],
    teamMembers: () => [],
    history: () => [],
    reports: () => [],
  },
)
const emit = defineEmits<{
  edit: []
  start: [Task]
  cancel: [Task, string]
  weight: [Task, number, string]
  move: [Task, string | null]
  dependency: [Task, string]
  ignore: [Task, string, string]
  history: [Task]
  report: [ProgressReportInput]
  refreshReports: []
  changed: []
}>()

type TabId = 'progress' | 'delivery' | 'relations' | 'activity' | 'settings'
const tab = ref<TabId>('progress')
const cancelReason = ref('')
const weight = ref(props.task.weight)
const weightReason = ref('')
const parent = ref(props.task.parent_id ?? '')
const predecessor = ref('')
const ignoreReasons = ref<Record<string, string>>({})

watch(
  () => props.task.id,
  () => {
    tab.value = 'progress'
    cancelReason.value = ''
    weight.value = props.task.weight
    weightReason.value = ''
    parent.value = props.task.parent_id ?? ''
    predecessor.value = ''
    ignoreReasons.value = {}
  },
)
watch(tab, (value) => {
  if (value === 'activity') emit('history', props.task)
})

const deps = computed(() => props.dependencies.filter((d) => d.task_id === props.task.id))
const children = computed(() => props.tasks.filter((t) => t.parent_id === props.task.id))
const parentTask = computed(() =>
  props.task.parent_id ? props.tasks.find((t) => t.id === props.task.parent_id) : undefined,
)
const editable = computed(
  () =>
    props.canManage && !['cancelled', 'completed', 'awaiting_review'].includes(props.task.status),
)
const ownerName = computed(() => memberName(props.task.owner_id))
const acceptance = computed(() => {
  switch (props.task.status) {
    case 'awaiting_review':
      return { label: '待验收', tone: 'warning' as const }
    case 'completed':
      return { label: '验收通过', tone: 'success' as const }
    case 'rejected':
      return { label: '已驳回', tone: 'danger' as const }
    case 'cancelled':
      return { label: '已取消', tone: 'neutral' as const }
    default:
      return { label: '未提交验收', tone: 'neutral' as const }
  }
})
const priority = computed(() => {
  const labels: Record<string, string> = { low: '低', normal: '普通', high: '高', urgent: '紧急' }
  return labels[props.task.priority] ?? props.task.priority
})
const tabs = computed(() => {
  const base: { id: TabId; label: string }[] = [
    { id: 'progress', label: '工作进展' },
    { id: 'delivery', label: '交付与验收' },
    { id: 'relations', label: '关联' },
    { id: 'activity', label: '活动' },
  ]
  if (editable.value) base.push({ id: 'settings', label: '设置' })
  return base
})

function memberName(id: string) {
  const member = props.members.find((m) => m.user_id === id)
  return member?.display_name || member?.username || '已失效成员'
}
function submitReport(input: ProgressReportInput) {
  emit('report', input)
}
</script>

<template>
  <section class="flex min-h-0 flex-col" aria-label="任务详情">
    <header class="border-b border-line px-5 py-4">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div class="min-w-0">
          <p class="text-meta text-ink-subtle">
            {{ task.depth }} 级 · {{ task.is_leaf ? '执行任务' : '汇总任务' }}
          </p>
          <h3 class="mt-1 text-lg font-semibold leading-6 text-ink">{{ task.title }}</h3>
        </div>
        <div class="flex shrink-0 items-center gap-2">
          <button v-if="editable" class="btn btn-sm" type="button" @click="emit('edit')">
            编辑任务
          </button>
          <button
            v-if="task.can_execute && (task.status === 'pending' || task.status === 'rejected')"
            class="btn btn-primary btn-sm"
            type="button"
            @click="emit('start', task)"
          >
            {{ task.status === 'rejected' ? '开始修正' : '开始任务' }}
          </button>
        </div>
      </div>
      <div class="mt-3 flex flex-wrap items-center gap-2 text-xs">
        <span class="badge">{{ taskStatusLabel[task.status] }}</span>
        <span class="badge">优先级 {{ priority }}</span>
        <span v-if="task.blocked" class="badge border-warning/30 bg-warning-soft text-warning"
          >依赖阻塞</span
        >
        <span class="text-ink-subtle">负责人</span>
        <span class="text-ink">{{ ownerName }}</span>
        <template v-if="task.collaborator_ids.length">
          <span class="text-ink-subtle">协作者</span>
          <span class="text-ink">{{ task.collaborator_ids.map(memberName).join('、') }}</span>
        </template>
      </div>
    </header>

    <div class="border-b border-line px-5 py-4">
      <dl class="grid grid-cols-3 gap-4">
        <div>
          <dt class="field-label">最近上报进度</dt>
          <dd class="mt-1 text-lg font-semibold text-ink">{{ task.actual_progress }}%</dd>
          <p class="mt-0.5 text-meta text-ink-subtle">执行结果，不等于验收</p>
        </div>
        <div>
          <dt class="field-label">汇总有效进度</dt>
          <dd class="mt-1 text-lg font-semibold text-ink">
            {{ percent(task.effective_progress_bps) }}
          </dd>
          <p class="mt-0.5 text-meta text-ink-subtle">
            {{ task.is_leaf ? '执行中最高 79%，待验收 80%' : '按后代叶子任务加权汇总' }}
          </p>
        </div>
        <div>
          <dt class="field-label">验收状态</dt>
          <dd class="mt-1">
            <span
              class="inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-medium"
              :class="{
                'border-accent/30 bg-accent-soft text-accent-strong': acceptance.tone === 'success',
                'border-warning/30 bg-warning-soft text-warning': acceptance.tone === 'warning',
                'border-danger/30 bg-danger-soft text-danger': acceptance.tone === 'danger',
                'border-line bg-surface-muted text-ink-muted': acceptance.tone === 'neutral',
              }"
              >{{ acceptance.label }}</span
            >
          </dd>
          <p class="mt-0.5 text-meta text-ink-subtle">正式验收独立于进度上报</p>
        </div>
      </dl>
    </div>

    <nav
      class="flex gap-1 overflow-x-auto border-b border-line px-5 pt-3"
      aria-label="任务详情导航"
    >
      <button
        v-for="item in tabs"
        :key="item.id"
        type="button"
        class="rounded-t-md border-b-2 px-3 py-2 text-xs font-medium transition-colors"
        :class="
          tab === item.id
            ? 'border-accent text-accent-strong'
            : 'border-transparent text-ink-muted hover:text-ink'
        "
        :aria-pressed="tab === item.id"
        @click="tab = item.id"
      >
        {{ item.label }}
      </button>
    </nav>

    <div class="min-h-0 flex-1 overflow-y-auto px-5 py-4">
      <template v-if="tab === 'progress'">
        <div class="space-y-4">
          <p v-if="task.description" class="whitespace-pre-wrap text-sm leading-6 text-ink-muted">
            {{ task.description }}
          </p>
          <p
            v-else
            class="rounded-lg border border-dashed border-line px-4 py-3 text-xs text-ink-subtle"
          >
            暂无任务描述。
          </p>
          <ProgressReportPanel
            v-if="task.is_leaf"
            :task="task"
            :reports="reports"
            :loading="reportsLoading"
            :can-report="task.can_execute && task.status === 'in_progress'"
            :busy="busy"
            @submit="submitReport"
            @refresh="emit('refreshReports')"
          />
          <p v-else class="rounded-lg bg-surface-muted px-4 py-3 text-xs leading-5 text-ink-muted">
            汇总任务不接收进度上报。进度来自后代叶子任务；用「关联」查看子任务，或进入子任务上报。
          </p>
        </div>
      </template>

      <template v-else-if="tab === 'delivery'">
        <div v-if="task.is_leaf" class="space-y-4">
          <SubmissionPanel
            :key="`submission:${team}:${project}:${task.id}:${task.version}`"
            :team="team"
            :project="project"
            :task="task"
            @changed="emit('changed')"
          />
        </div>
        <div v-else class="rounded-lg bg-surface-muted px-4 py-3 text-xs leading-5 text-ink-muted">
          汇总任务没有独立交付。请在叶子任务中提交成果并完成验收。
        </div>
      </template>

      <template v-else-if="tab === 'relations'">
        <div class="space-y-5">
          <section>
            <h4 class="text-xs font-semibold tracking-wide text-ink-subtle uppercase">任务层级</h4>
            <div class="mt-2 space-y-1.5 text-sm">
              <p v-if="parentTask" class="text-ink-muted">
                父任务：<span class="text-ink">{{ parentTask.title }}</span>
              </p>
              <p v-else class="text-ink-muted">这是一级任务。</p>
              <div v-if="children.length">
                <p class="text-ink-muted">子任务</p>
                <ul class="mt-1 space-y-1">
                  <li v-for="child in children" :key="child.id" class="text-ink">
                    · {{ child.title }}
                    <span class="text-xs text-ink-subtle"
                      >{{ taskStatusLabel[child.status] }} ·
                      {{ percent(child.effective_progress_bps) }}</span
                    >
                  </li>
                </ul>
              </div>
            </div>
          </section>

          <section class="border-t border-line pt-4">
            <h4 class="text-xs font-semibold tracking-wide text-ink-subtle uppercase">前置依赖</h4>
            <p v-if="task.blocked" class="mt-2 text-xs text-warning">
              前置任务尚未正式验收。取消前置任务不会解除阻塞。
            </p>
            <p v-if="!deps.length" class="mt-2 text-xs text-ink-muted">暂无可见的前置依赖。</p>
            <ul class="mt-2 space-y-2">
              <li
                v-for="d in deps"
                :key="d.id"
                class="rounded-lg border border-line px-3 py-2 text-sm"
              >
                <div class="flex flex-wrap items-center justify-between gap-2">
                  <span class="text-ink">{{
                    tasks.find((t) => t.id === d.predecessor_id)?.title ?? '前置任务'
                  }}</span>
                  <span class="badge">{{
                    d.ignored_at
                      ? '已忽略'
                      : tasks.find((t) => t.id === d.predecessor_id)?.status === 'completed'
                        ? '已正式完成'
                        : '待正式完成'
                  }}</span>
                </div>
                <p v-if="d.ignored_at" class="mt-1 text-xs text-ink-muted">
                  {{ d.ignore_reason }} · {{ formatDateTime(d.ignored_at) }}
                </p>
                <form
                  v-else-if="editable"
                  class="mt-2 flex items-end gap-2"
                  @submit.prevent="emit('ignore', task, d.id, ignoreReasons[d.id] ?? '')"
                >
                  <label class="form-label grow">
                    忽略原因
                    <input
                      v-model="ignoreReasons[d.id]"
                      required
                      maxlength="1000"
                      class="form-input"
                    />
                  </label>
                  <button class="btn btn-sm" type="submit">忽略</button>
                </form>
              </li>
            </ul>
            <form
              v-if="canManage && task.is_leaf && task.status === 'pending'"
              class="mt-3 flex flex-wrap items-end gap-2"
              @submit.prevent="emit('dependency', task, predecessor)"
            >
              <label class="form-label grow">
                添加前置任务
                <select v-model="predecessor" required class="form-input">
                  <option value="" disabled>请选择</option>
                  <option
                    v-for="t in tasks.filter(
                      (t) => t.id !== task.id && t.is_leaf && t.status !== 'cancelled',
                    )"
                    :key="t.id"
                    :value="t.id"
                  >
                    {{ t.title }}
                  </option>
                </select>
              </label>
              <button class="btn btn-sm" type="submit">添加依赖</button>
            </form>
          </section>
        </div>
      </template>

      <template v-else-if="tab === 'activity'">
        <div class="flex items-center justify-between">
          <h4 class="text-xs font-semibold tracking-wide text-ink-subtle uppercase">任务活动</h4>
          <button
            class="btn btn-ghost btn-sm"
            type="button"
            :disabled="historyLoading"
            @click="emit('history', task)"
          >
            {{ historyLoading ? '加载中…' : '刷新' }}
          </button>
        </div>
        <p v-if="historyLoading && !history.length" class="mt-3 text-xs text-ink-muted">
          正在加载活动记录…
        </p>
        <ol v-else-if="history.length" class="mt-3 space-y-3">
          <li
            v-for="h in history"
            :key="h.id"
            class="border-l-2 border-line pl-3 text-xs text-ink-muted"
          >
            <p class="font-medium text-ink">{{ h.event_type }}</p>
            <p class="mt-0.5">{{ formatDateTime(h.created_at) }}</p>
            <p class="mt-1">
              {{ taskStatusLabel[h.old_status] }} → {{ taskStatusLabel[h.new_status] }} · 上报
              {{ h.old_progress }}% → {{ h.new_progress }}%
            </p>
            <p v-if="h.reason" class="mt-1 whitespace-pre-wrap text-ink">{{ h.reason }}</p>
          </li>
        </ol>
        <p v-else class="mt-3 text-xs text-ink-muted">暂无活动记录。</p>
      </template>

      <template v-else-if="tab === 'settings'">
        <div class="space-y-5">
          <form
            class="grid gap-3 rounded-lg border border-line p-3 sm:grid-cols-2"
            @submit.prevent="emit('weight', task, weight, weightReason)"
          >
            <label class="form-label">
              新权重
              <input
                v-model.number="weight"
                type="number"
                min="1"
                max="1000000"
                required
                class="form-input"
              />
            </label>
            <label class="form-label">
              权重修改原因
              <input v-model="weightReason" required maxlength="1000" class="form-input" />
            </label>
            <button class="btn sm:col-span-2" type="submit">变更权重并保留快照</button>
          </form>
          <form
            class="flex flex-wrap items-end gap-2 rounded-lg border border-line p-3"
            @submit.prevent="emit('move', task, parent || null)"
          >
            <label class="form-label grow">
              移动到父任务
              <select v-model="parent" aria-label="移动到父任务" class="form-input">
                <option value="">无（一级任务）</option>
                <option
                  v-for="t in tasks.filter((t) => t.id !== task.id && canSelectParent(t))"
                  :key="t.id"
                  :value="t.id"
                >
                  {{ t.depth }} 级 · {{ t.title }}
                </option>
              </select>
            </label>
            <button class="btn" type="submit">移动子树</button>
          </form>
          <form
            v-if="task.can_cancel"
            class="flex flex-wrap items-end gap-2 rounded-lg border border-danger/20 bg-danger-soft/40 p-3"
            @submit.prevent="emit('cancel', task, cancelReason)"
          >
            <label class="form-label grow">
              取消原因
              <input v-model="cancelReason" required maxlength="1000" class="form-input" />
            </label>
            <button class="btn btn-danger" type="submit">取消任务</button>
          </form>
          <p v-if="task.cancellation_reason" class="text-xs text-ink-muted">
            取消原因：{{ task.cancellation_reason }}
          </p>
        </div>
      </template>
    </div>
  </section>
</template>
