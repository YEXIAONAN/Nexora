<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import type { ProjectMember } from '@/api/projects'
import { percent, taskStatusLabel, type Dependency, type Task, type TaskHistory } from '@/api/tasks'
import { formatDateTime } from '@/utils/format'
const props = defineProps<{
  task: Task
  tasks: Task[]
  dependencies: Dependency[]
  members: ProjectMember[]
  canManage: boolean
  history: TaskHistory[]
  historyLoading: boolean
}>()
const emit = defineEmits<{
  edit: []
  start: [Task]
  cancel: [Task, string]
  progress: [Task, number]
  weight: [Task, number, string]
  move: [Task, string | null]
  dependency: [Task, string]
  ignore: [Task, string, string]
  history: [Task]
}>()
const actual = ref(props.task.actual_progress),
  weight = ref(props.task.weight),
  weightReason = ref(''),
  cancelReason = ref(''),
  parent = ref(props.task.parent_id ?? ''),
  predecessor = ref('')
const reasons = reactive<Record<string, string>>({})
const deps = computed(() => props.dependencies.filter((d) => d.task_id === props.task.id))
const name = (id: string) =>
  props.members.find((m) => m.user_id === id)?.display_name ||
  props.members.find((m) => m.user_id === id)?.username ||
  '已失效成员'
const title = (id: string) => props.tasks.find((t) => t.id === id)?.title ?? '前置任务'
</script>
<template>
  <section class="card min-w-0">
    <div class="card-header">
      <div>
        <p class="text-xs text-ink-subtle">
          {{ task.depth }} 级 · {{ task.is_leaf ? '执行任务' : '汇总任务' }}
        </p>
        <h3 class="mt-1 text-lg font-semibold">{{ task.title }}</h3>
      </div>
      <button
        v-if="canManage && task.status !== 'cancelled'"
        class="btn"
        type="button"
        @click="emit('edit')"
      >
        编辑任务
      </button>
    </div>
    <div class="card-body space-y-5">
      <div class="flex flex-wrap items-center gap-2">
        <span class="badge">{{ taskStatusLabel[task.status] }}</span
        ><span v-if="task.blocked" class="badge text-warning">依赖阻塞</span
        ><span class="ml-auto text-sm font-semibold text-accent-strong"
          >有效进度 {{ percent(task.effective_progress_bps) }}</span
        >
      </div>
      <p class="whitespace-pre-wrap text-sm leading-6 text-ink-muted">
        {{ task.description || '暂无任务描述' }}
      </p>
      <dl class="grid grid-cols-2 gap-3 text-xs">
        <div>
          <dt class="text-ink-subtle">负责人</dt>
          <dd class="mt-1">{{ name(task.owner_id) }}</dd>
        </div>
        <div>
          <dt class="text-ink-subtle">协作者</dt>
          <dd class="mt-1">{{ task.collaborator_ids.map(name).join('、') || '暂无' }}</dd>
        </div>
        <div>
          <dt class="text-ink-subtle">计划开始</dt>
          <dd class="mt-1">
            {{ task.planned_start_at ? formatDateTime(task.planned_start_at) : '未设置' }}
          </dd>
        </div>
        <div>
          <dt class="text-ink-subtle">计划结束</dt>
          <dd class="mt-1">
            {{ task.planned_end_at ? formatDateTime(task.planned_end_at) : '未设置' }}
          </dd>
        </div>
      </dl>
      <p
        v-if="!task.is_leaf"
        class="rounded-lg bg-surface-muted p-3 text-xs leading-5 text-ink-muted"
      >
        汇总任务的进度来自后代叶子任务，自身权重不参与项目计算。
      </p>
      <template v-if="task.is_leaf">
        <p class="text-xs text-ink-muted">
          实际进度 {{ task.actual_progress }}% · 权重 {{ task.weight }}。执行中的有效进度最高为
          79%，正式完成需要验收记录。
        </p>
        <button
          v-if="task.can_execute && task.status === 'pending'"
          class="btn btn-primary"
          type="button"
          @click="emit('start', task)"
        >
          开始任务
        </button>
        <form
          v-if="task.can_execute && task.status === 'in_progress'"
          class="flex flex-wrap items-end gap-2"
          @submit.prevent="emit('progress', task, actual)"
        >
          <label class="form-label grow"
            >实际进度（0–99%）<input
              v-model.number="actual"
              type="number"
              min="0"
              max="99"
              required
              class="form-input" /></label
          ><button class="btn btn-primary" type="submit">记录进度</button>
        </form>
        <form
          v-if="task.can_cancel"
          class="flex flex-wrap items-end gap-2"
          @submit.prevent="emit('cancel', task, cancelReason)"
        >
          <label class="form-label grow"
            >取消原因<input
              v-model="cancelReason"
              required
              maxlength="1000"
              class="form-input" /></label
          ><button class="btn text-danger" type="submit">取消任务</button>
        </form>
        <p v-if="task.cancellation_reason" class="text-sm text-ink-muted">
          取消原因：{{ task.cancellation_reason }}
        </p>
        <form
          v-if="canManage && task.status !== 'cancelled'"
          class="grid gap-3 rounded-lg border border-line p-3 sm:grid-cols-2"
          @submit.prevent="emit('weight', task, weight, weightReason)"
        >
          <label class="form-label"
            >新权重<input
              v-model.number="weight"
              type="number"
              min="1"
              max="1000000"
              required
              class="form-input" /></label
          ><label class="form-label"
            >权重修改原因<input
              v-model="weightReason"
              required
              maxlength="1000"
              class="form-input" /></label
          ><button class="btn sm:col-span-2" type="submit">变更权重并保留快照</button>
        </form>
      </template>
      <form
        v-if="canManage && task.status !== 'cancelled'"
        class="flex flex-wrap items-end gap-2"
        @submit.prevent="emit('move', task, parent || null)"
      >
        <label class="form-label grow"
          >移动到父任务<select aria-label="移动到父任务" v-model="parent" class="form-input">
            <option value="">无（一级任务）</option>
            <option
              v-for="t in tasks.filter(
                (t) => t.id !== task.id && t.depth < 3 && t.status === 'pending',
              )"
              :key="t.id"
              :value="t.id"
            >
              {{ t.title }}
            </option>
          </select></label
        ><button class="btn" type="submit">移动子树</button>
      </form>
      <div class="space-y-3 border-t border-line pt-4">
        <h4 class="text-sm font-semibold">前置依赖</h4>
        <p v-if="task.blocked" class="text-xs text-warning">
          前置任务尚未正式验收。取消前置任务不会解除阻塞。
        </p>
        <p v-if="!deps.length" class="text-xs text-ink-muted">暂无可见的前置依赖</p>
        <div v-for="d in deps" :key="d.id" class="rounded-lg border border-line p-3">
          <p class="text-sm">
            {{ title(d.predecessor_id) }}
            <span class="badge">{{ d.ignored_at ? '已显式忽略' : '待正式完成' }}</span>
          </p>
          <p v-if="d.ignored_at" class="mt-2 text-xs text-ink-muted">
            {{ d.ignore_reason }} · {{ formatDateTime(d.ignored_at) }}
          </p>
          <form
            v-else-if="canManage"
            class="mt-3 flex items-end gap-2"
            @submit.prevent="emit('ignore', task, d.id, reasons[d.id] ?? '')"
          >
            <label class="form-label grow"
              >忽略原因<input
                v-model="reasons[d.id]"
                required
                maxlength="1000"
                class="form-input" /></label
            ><button class="btn" type="submit">忽略此依赖</button>
          </form>
        </div>
        <form
          v-if="canManage && task.is_leaf && task.status === 'pending'"
          class="flex flex-wrap items-end gap-2"
          @submit.prevent="emit('dependency', task, predecessor)"
        >
          <label class="form-label grow"
            >选择前置任务<select
              aria-label="选择前置任务"
              v-model="predecessor"
              required
              class="form-input"
            >
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
            </select></label
          ><button class="btn" type="submit">添加依赖</button>
        </form>
      </div>
      <div class="border-t border-line pt-4">
        <button
          class="btn btn-ghost"
          type="button"
          :disabled="historyLoading"
          @click="emit('history', task)"
        >
          {{ historyLoading ? '加载中…' : '查看任务历史' }}
        </button>
        <ol class="mt-3 space-y-3">
          <li
            v-for="h in history"
            :key="h.id"
            class="border-l-2 border-line pl-3 text-xs text-ink-muted"
          >
            <p>{{ h.event_type }} · {{ formatDateTime(h.created_at) }}</p>
            <p class="mt-1">
              {{ taskStatusLabel[h.old_status] }} → {{ taskStatusLabel[h.new_status] }} · 实际
              {{ h.old_progress }}% → {{ h.new_progress }}% · 权重 {{ h.old_weight }} →
              {{ h.new_weight }}
            </p>
            <p v-if="h.reason" class="mt-1">{{ h.reason }}</p>
          </li>
        </ol>
      </div>
    </div>
  </section>
</template>
