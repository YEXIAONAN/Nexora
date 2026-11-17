<script setup lang="ts">
import { reactive } from 'vue'
import type { CreateProject, Project } from '@/api/projects'
import type { TeamMember } from '@/api/teams'
const props = defineProps<{ project?: Project; members: TeamMember[]; defaultLead?: string }>()
const emit = defineEmits<{ save: [CreateProject] }>()
const localDate = (value?: string | null) =>
  value
    ? new Date(new Date(value).getTime() - new Date(value).getTimezoneOffset() * 60000)
        .toISOString()
        .slice(0, 16)
    : ''
const form = reactive({
  name: props.project?.name ?? '',
  description: props.project?.description ?? '',
  lead: props.project?.lead_id ?? props.defaultLead ?? '',
  start: localDate(props.project?.planned_start_at),
  end: localDate(props.project?.planned_end_at),
})
function save() {
  emit('save', {
    name: form.name.trim(),
    description: form.description,
    lead_id: form.lead,
    planned_start_at: form.start ? new Date(form.start).toISOString() : null,
    planned_end_at: form.end ? new Date(form.end).toISOString() : null,
  })
}
</script>
<template>
  <form class="grid gap-4 sm:grid-cols-2" @submit.prevent="save">
    <label class="form-label sm:col-span-2"
      >项目名称<input v-model="form.name" required maxlength="120" class="form-input"
    /></label>
    <label class="form-label sm:col-span-2"
      >项目描述<textarea v-model="form.description" maxlength="4000" rows="3" class="form-input" />
    </label>
    <label v-if="!project" class="form-label sm:col-span-2"
      >项目负责人<select aria-label="项目负责人" v-model="form.lead" required class="form-input">
        <option disabled value="">选择负责人</option>
        <option v-for="m in members" :key="m.user_id" :value="m.user_id">
          {{ m.display_name || m.username }}
        </option>
      </select></label
    >
    <label class="form-label"
      >计划开始<input v-model="form.start" type="datetime-local" class="form-input"
    /></label>
    <label class="form-label"
      >计划结束<input v-model="form.end" type="datetime-local" class="form-input"
    /></label>
    <div class="sm:col-span-2">
      <button class="btn btn-primary" type="submit">{{ project ? '保存项目' : '创建项目' }}</button>
    </div>
  </form>
</template>
