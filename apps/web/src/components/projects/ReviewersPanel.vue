<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { ApiError } from '@/api/client'
import * as api from '@/api/submissions'
import type { ProjectMember } from '@/api/projects'
const props = defineProps<{
  team: string
  project: string
  canManage: boolean
  members: ProjectMember[]
}>()
const emit = defineEmits<{ changed: [] }>()
const key = computed(() => `${props.team}/${props.project}`)
const reviewers = ref<api.Reviewer[]>([]),
  confirmed = ref(''),
  loading = ref(false),
  busy = ref(false),
  error = ref(''),
  user = ref(''),
  reason = ref('')
let generation = 0,
  mutationGeneration = 0
async function load() {
  const scope = key.value,
    gen = ++generation,
    { team, project } = props
  confirmed.value = ''
  reviewers.value = []
  loading.value = true
  error.value = ''
  try {
    const rows = await api.listReviewers(team, project)
    if (scope === key.value && gen === generation) {
      reviewers.value = rows
      confirmed.value = scope
    }
  } catch (e) {
    if (scope === key.value && gen === generation)
      error.value = e instanceof ApiError ? e.message : '审核授权加载失败'
  } finally {
    if (scope === key.value && gen === generation) loading.value = false
  }
}
async function change(reviewer?: api.Reviewer) {
  if (confirmed.value !== key.value || busy.value || !props.canManage || !reason.value.trim())
    return
  const scope = key.value,
    gen = generation,
    mutation = ++mutationGeneration,
    { team, project } = props,
    selected = user.value,
    message = reason.value
  busy.value = true
  error.value = ''
  try {
    if (reviewer) await api.revokeReviewer(team, project, reviewer, message)
    else await api.grantReviewer(team, project, selected, message)
    if (scope === key.value && gen === generation) {
      reason.value = ''
      user.value = ''
      await load()
      if (scope === key.value && mutation === mutationGeneration) emit('changed')
    }
  } catch (e) {
    if (scope === key.value && gen === generation)
      error.value = e instanceof ApiError ? e.message : '审核授权操作失败'
  } finally {
    if (scope === key.value && mutation === mutationGeneration) busy.value = false
  }
}
watch(
  key,
  () => {
    mutationGeneration++
    busy.value = false
    reason.value = ''
    user.value = ''
    void load()
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  mutationGeneration++
  generation++
  mutationGeneration++
})
</script>
<template>
  <section class="space-y-3 border-t border-line pt-5" aria-label="审核人授权">
    <h3 class="text-sm font-semibold">审核人授权</h3>
    <p class="text-xs text-ink-muted">
      项目负责人默认可审核。成员需获得明确授权；所有人均不能审核自己的提交。
    </p>
    <p v-if="error" role="alert" class="text-sm text-danger">{{ error }}</p>
    <p v-if="loading" class="text-xs text-ink-muted">正在加载审核人…</p>
    <fieldset v-if="confirmed === key" :disabled="busy" class="space-y-3">
      <div
        v-for="r in reviewers"
        :key="r.user_id"
        class="flex flex-wrap items-center gap-2 text-sm"
      >
        <span class="grow"
          >{{ r.display_name || r.username }} ·
          {{ r.default_lead ? '项目负责人' : r.active ? '已授权审核' : '授权已失效' }}</span
        ><button
          v-if="canManage && r.active && !r.default_lead"
          class="btn text-danger"
          :disabled="!reason.trim()"
          @click="change(r)"
        >
          撤销审核权
        </button>
      </div>
      <form v-if="canManage" class="grid gap-3" @submit.prevent="change()">
        <label class="form-label"
          >授权或撤销原因<input v-model="reason" required maxlength="1000" class="form-input"
        /></label>
        <label class="form-label"
          >选择审核人<select v-model="user" class="form-input" aria-label="选择审核人">
            <option value="">请选择有效项目成员</option>
            <option
              v-for="m in members.filter(
                (m) => m.active && !reviewers.some((r) => r.user_id === m.user_id && r.active),
              )"
              :key="m.user_id"
              :value="m.user_id"
            >
              {{ m.display_name || m.username }}
            </option>
          </select></label
        >
        <button class="btn" type="submit" :disabled="!user || !reason.trim()">授予审核权</button>
      </form>
    </fieldset>
  </section>
</template>
