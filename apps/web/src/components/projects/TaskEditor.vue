<script setup lang="ts">
import { reactive, watch } from 'vue'
import type { ProjectMember } from '@/api/projects'
import type { CreateTask, Task } from '@/api/tasks'
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
      <template v-if="!task"
        ><label class="form-label"
          >父任务<select aria-label="父任务" v-model="form.parent" class="form-input">
            <option value="">无（一级任务）</option>
            <option
              v-for="t in tasks.filter((t) => t.depth < 3 && t.status === 'pending')"
              :key="t.id"
              :value="t.id"
            >
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
        <button class="btn btn-primary" type="submit">
          {{ task ? '保存任务' : '保存新任务' }}
        </button>
      </div>
    </form>
  </section>
</template>
