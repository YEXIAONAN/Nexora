<script setup lang="ts">
import { computed, reactive, watch } from 'vue'
import type { ProjectMember } from '@/api/projects'
import { canSelectParent, type CreateTask, type Task } from '@/api/tasks'
const props = defineProps<{
  task?: Task
  tasks: Task[]
  members: ProjectMember[]
  defaultOwner: string
}>()
const emit = defineEmits<{ save: [CreateTask]; close: [] }>()
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
const inactiveCollaborators = computed(() =>
  form.collaborators.filter((id) => !props.members.some((m) => m.user_id === id && m.active)),
)
function memberName(id: string) {
  const member = props.members.find((m) => m.user_id === id)
  return member?.display_name || member?.username || id
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
  <section class="card">
    <div class="card-header">
      <h3 class="font-semibold">{{ task ? '编辑任务' : '创建任务' }}</h3>
      <button class="btn btn-ghost" type="button" @click="emit('close')">收起</button>
    </div>
    <form class="card-body grid gap-4 sm:grid-cols-2" @submit.prevent="save">
      <label class="form-label sm:col-span-2"
        >任务标题<input v-model="form.title" required maxlength="200" class="form-input"
      /></label>
      <label class="form-label sm:col-span-2"
        >任务描述<textarea v-model="form.description" class="form-input" maxlength="8000" />
      </label>
      <label class="form-label"
        >任务负责人<select aria-label="任务负责人" v-model="form.owner" class="form-input" required>
          <option value="" disabled>请选择</option>
          <option v-for="m in members.filter((m) => m.active)" :key="m.user_id" :value="m.user_id">
            {{ m.display_name || m.username }}
          </option>
        </select></label
      >
      <label class="form-label"
        >优先级<select aria-label="优先级" v-model="form.priority" class="form-input">
          <option value="low">低</option>
          <option value="normal">普通</option>
          <option value="high">高</option>
          <option value="urgent">紧急</option>
        </select></label
      >
      <label class="form-label sm:col-span-2"
        >协作者（可多选）<select
          aria-label="协作者（可多选）"
          v-model="form.collaborators"
          multiple
          class="form-input"
          size="3"
        >
          <option
            v-for="m in members.filter((m) => m.active && m.user_id !== form.owner)"
            :key="m.user_id"
            :value="m.user_id"
          >
            {{ m.display_name || m.username }}
          </option>
        </select></label
      >
      <div
        v-if="inactiveCollaborators.length"
        class="rounded-lg border border-warning/40 bg-warning-soft p-3 sm:col-span-2"
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
              class="btn"
              type="button"
              :aria-label="`移除失效协作者 ${memberName(id)}`"
              @click="removeCollaborator(id)"
            >
              移除失效协作者
            </button>
          </li>
        </ul>
      </div>
      <template v-if="!task"
        ><label class="form-label"
          >父任务<select aria-label="父任务" v-model="form.parent" class="form-input">
            <option value="">无（一级任务）</option>
            <option v-for="t in tasks.filter(canSelectParent)" :key="t.id" :value="t.id">
              {{ t.depth }} 级 · {{ t.title }}
            </option>
          </select></label
        ><label class="form-label"
          >工作量权重<input
            v-model.number="form.weight"
            type="number"
            min="1"
            max="1000000"
            required
            class="form-input" /></label
      ></template>
      <label class="form-label"
        >计划开始<input v-model="form.start" type="datetime-local" class="form-input" /></label
      ><label class="form-label"
        >计划结束<input v-model="form.end" type="datetime-local" class="form-input"
      /></label>
      <div class="sm:col-span-2">
        <button class="btn btn-primary" type="submit" :disabled="inactiveCollaborators.length > 0">
          {{ task ? '保存任务' : '保存新任务' }}
        </button>
      </div>
    </form>
  </section>
</template>
