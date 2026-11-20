<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import type { ProjectMember } from '@/api/projects'
import type { TeamMember } from '@/api/teams'
import { canSelectParent, type CreateTask, type Task } from '@/api/tasks'
const props = defineProps<{
  task?: Task
  tasks: Task[]
  members: ProjectMember[]
  teamMembers: TeamMember[]
  defaultOwner: string
  canManageMembers: boolean
}>()
const emit = defineEmits<{ save: [CreateTask]; close: []; manageMembers: [] }>()
const localDate = (value?: string | null) =>
  value
    ? new Date(new Date(value).getTime() - new Date(value).getTimezoneOffset() * 60000)
        .toISOString()
        .slice(0, 16)
    : ''
const form = reactive({
  title: props.task?.title ?? '',
  description: props.task?.description ?? '',
  owner: props.task?.owner_id ?? props.defaultOwner,
  collaborators: [...(props.task?.collaborator_ids ?? [])],
  priority: props.task?.priority ?? 'normal',
  parent: props.task?.parent_id ?? '',
  weight: props.task?.weight ?? 1,
  start: localDate(props.task?.planned_start_at),
  end: localDate(props.task?.planned_end_at),
})
const search = ref('')
const activeMembers = computed(() => props.members.filter((m) => m.active))
const inactiveCollaborators = computed(() =>
  form.collaborators.filter((id) => !props.members.some((m) => m.user_id === id && m.active)),
)
const selectable = computed(() => activeMembers.value.filter((m) => m.user_id !== form.owner))
const filtered = computed(() => {
  const term = search.value.trim().toLowerCase()
  if (!term) return selectable.value
  return selectable.value.filter(
    (m) =>
      (m.display_name ?? '').toLowerCase().includes(term) ||
      (m.username ?? '').toLowerCase().includes(term),
  )
})
const ownerName = computed(() => memberName(form.owner))
const teamMembersOutsideProject = computed(() => {
  const projectIds = new Set(props.members.map((m) => m.user_id))
  return props.teamMembers.filter((m) => !projectIds.has(m.user_id))
})
const emptyReason = computed(() => {
  if (selectable.value.length > 0) return ''
  if (activeMembers.value.length === 0) return '项目还没有有效成员，请先在「项目成员」中添加成员。'
  if (activeMembers.value.length === 1)
    return '项目目前只有负责人一个成员，负责人不能同时作为协作者。'
  if (teamMembersOutsideProject.value.length > 0) {
    return '团队中还有其他成员尚未加入本项目。'
  }
  return '没有其他可选的活跃项目成员。'
})
function memberName(id: string) {
  const member = props.members.find((m) => m.user_id === id)
  return member?.display_name || member?.username || id
}
function toggleCollaborator(id: string) {
  form.collaborators = form.collaborators.includes(id)
    ? form.collaborators.filter((current) => current !== id)
    : [...form.collaborators, id]
}
function removeCollaborator(id: string) {
  form.collaborators = form.collaborators.filter((current) => current !== id)
}
watch(
  () => form.owner,
  (id) => {
    form.collaborators = form.collaborators.filter((u) => u !== id)
  },
)
function save() {
  emit('save', {
    title: form.title.trim(),
    description: form.description,
    owner_id: form.owner,
    collaborator_ids: [...form.collaborators],
    priority: form.priority,
    parent_id: form.parent || null,
    weight: form.weight,
    planned_start_at: form.start ? new Date(form.start).toISOString() : null,
    planned_end_at: form.end ? new Date(form.end).toISOString() : null,
  })
}
</script>
<template>
  <form class="flex min-h-0 flex-1 flex-col" @submit.prevent="save">
    <div class="min-h-0 flex-1 space-y-5 overflow-y-auto px-5 py-5">
      <label class="form-label">
        任务标题
        <input v-model="form.title" required maxlength="200" class="form-input" />
      </label>
      <label class="form-label">
        任务描述
        <textarea v-model="form.description" class="form-input" rows="3" maxlength="8000" />
      </label>

      <div class="grid gap-4 sm:grid-cols-2">
        <label class="form-label">
          负责人
          <select v-model="form.owner" required class="form-input">
            <option value="" disabled>请选择</option>
            <option v-for="m in activeMembers" :key="m.user_id" :value="m.user_id">
              {{ m.display_name || m.username }}
            </option>
          </select>
        </label>
        <label class="form-label">
          优先级
          <select v-model="form.priority" class="form-input">
            <option value="low">低</option>
            <option value="normal">普通</option>
            <option value="high">高</option>
            <option value="urgent">紧急</option>
          </select>
        </label>
      </div>

      <fieldset class="rounded-xl border border-line">
        <legend class="px-2 text-xs font-medium text-ink-muted">协作者（可选，多选）</legend>
        <div class="border-b border-line px-4 py-3">
          <div v-if="form.collaborators.length" class="flex flex-wrap gap-1.5">
            <span
              v-for="id in form.collaborators"
              :key="id"
              class="inline-flex items-center gap-1.5 rounded-md border border-accent/30 bg-accent-soft px-2 py-1 text-xs font-medium text-accent-strong"
            >
              {{ memberName(id) }}
              <button
                type="button"
                class="rounded p-0.5 hover:bg-accent/10"
                :aria-label="`取消选择 ${memberName(id)}`"
                @click="removeCollaborator(id)"
              >
                ×
              </button>
            </span>
          </div>
          <p v-else class="text-xs text-ink-muted">尚未选择协作者。负责人会默认负责该任务。</p>
        </div>
        <div class="px-4 py-3">
          <template v-if="selectable.length">
            <label class="form-label">
              搜索项目成员
              <input
                v-model="search"
                type="search"
                class="form-input"
                placeholder="按姓名或用户名搜索"
              />
            </label>
            <ul
              class="mt-2 max-h-44 space-y-1 overflow-y-auto"
              role="group"
              aria-label="可选协作者"
            >
              <li v-for="m in filtered" :key="m.user_id">
                <label
                  class="flex cursor-pointer items-center gap-2.5 rounded-lg px-2 py-1.5 text-sm hover:bg-surface-muted"
                >
                  <input
                    type="checkbox"
                    class="h-4 w-4 accent-[var(--accent)]"
                    :checked="form.collaborators.includes(m.user_id)"
                    @change="toggleCollaborator(m.user_id)"
                  />
                  <span class="grow">{{ m.display_name || m.username }}</span>
                  <span class="text-[11px] text-ink-subtle">@{{ m.username }}</span>
                </label>
              </li>
            </ul>
            <p v-if="!filtered.length" class="mt-2 text-xs text-ink-muted">
              没有匹配「{{ search }}」的项目成员。
            </p>
            <button
              v-if="canManageMembers"
              type="button"
              class="mt-2 text-xs text-accent-strong hover:underline"
              @click="emit('manageMembers')"
            >
              需要更多成员？前往添加项目成员
            </button>
          </template>
          <div v-else class="space-y-2">
            <p class="text-xs leading-5 text-ink-muted">{{ emptyReason }}</p>
            <button
              v-if="canManageMembers"
              type="button"
              class="btn btn-sm"
              @click="emit('manageMembers')"
            >
              前往添加项目成员
            </button>
            <p v-else class="text-xs text-ink-subtle">
              你没有添加项目成员的权限，请联系项目负责人处理。
            </p>
          </div>
        </div>
      </fieldset>

      <div
        v-if="inactiveCollaborators.length"
        class="rounded-lg border border-warning/40 bg-warning-soft p-3"
      >
        <p class="text-xs text-warning">以下协作者的成员资格已失效，请移除后保存任务。</p>
        <ul class="mt-2 space-y-2">
          <li
            v-for="id in inactiveCollaborators"
            :key="id"
            class="flex items-center justify-between gap-3"
          >
            <span class="text-sm text-ink-muted">{{ memberName(id) }} · 已失效</span>
            <button
              class="btn btn-sm"
              type="button"
              :aria-label="`移除失效协作者 ${memberName(id)}`"
              @click="removeCollaborator(id)"
            >
              移除
            </button>
          </li>
        </ul>
      </div>

      <template v-if="!task">
        <div class="grid gap-4 sm:grid-cols-2">
          <label class="form-label">
            父任务
            <select v-model="form.parent" aria-label="父任务" class="form-input">
              <option value="">无（一级任务）</option>
              <option v-for="t in tasks.filter(canSelectParent)" :key="t.id" :value="t.id">
                {{ t.depth }} 级 · {{ t.title }}
              </option>
            </select>
          </label>
          <label class="form-label">
            工作量权重
            <input
              v-model.number="form.weight"
              type="number"
              min="1"
              max="1000000"
              required
              class="form-input"
            />
          </label>
        </div>
      </template>

      <div class="grid gap-4 sm:grid-cols-2">
        <label class="form-label">
          计划开始
          <input v-model="form.start" type="datetime-local" class="form-input" />
        </label>
        <label class="form-label">
          计划结束
          <input v-model="form.end" type="datetime-local" class="form-input" />
        </label>
      </div>
      <p class="text-[11px] text-ink-subtle">
        负责人：{{ ownerName || '未选择' }} · 任务树最多三级，执行中的任务不能转为汇总节点。
      </p>
    </div>
    <div class="flex items-center justify-end gap-2 border-t border-line px-5 py-3">
      <button class="btn" type="button" @click="emit('close')">取消</button>
      <button
        class="btn btn-primary"
        type="submit"
        :disabled="inactiveCollaborators.length > 0 || !form.owner"
      >
        {{ task ? '保存任务' : '创建任务' }}
      </button>
    </div>
  </form>
</template>
