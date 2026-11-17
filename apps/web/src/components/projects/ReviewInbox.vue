<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { ApiError } from '@/api/client'
import { pendingReviews, type PendingReview } from '@/api/submissions'
import { formatDateTime } from '@/utils/format'
const props = defineProps<{ team: string }>()
const items = ref<PendingReview[]>([]),
  loading = ref(false),
  error = ref(''),
  confirmed = ref('')
let generation = 0
async function load() {
  const team = props.team,
    gen = ++generation
  confirmed.value = ''
  loading.value = true
  items.value = []
  error.value = ''
  try {
    const rows = await pendingReviews(team)
    if (team === props.team && gen === generation) {
      items.value = rows
      confirmed.value = team
    }
  } catch (e) {
    if (team === props.team && gen === generation)
      error.value = e instanceof ApiError ? e.message : '待审核成果加载失败'
  } finally {
    if (team === props.team && gen === generation) loading.value = false
  }
}
watch(
  () => props.team,
  () => void load(),
  { immediate: true },
)
onBeforeUnmount(() => {
  generation++
})
</script>
<template>
  <section class="card" aria-label="待我审核">
    <div class="card-header">
      <h3 class="font-semibold">待我审核</h3>
      <button class="btn btn-ghost" :disabled="loading" @click="load">刷新待审核</button>
    </div>
    <div class="card-body space-y-3">
      <p v-if="error" role="alert" class="text-sm text-danger">{{ error }}</p>
      <p v-if="loading" class="text-sm text-ink-muted">正在加载待审核成果…</p>
      <template v-if="confirmed === team"
        ><p v-if="!items.length" class="text-sm text-ink-muted">暂无需要你独立审核的成果。</p>
        <RouterLink
          v-for="item in items"
          :key="item.submission_version_id"
          :to="{
            name: 'project-detail',
            params: { teamId: team, projectId: item.project_id },
            query: { task: item.task_id },
          }"
          class="block rounded-lg border border-line p-3 text-sm hover:bg-surface-muted"
          ><p>{{ item.project_name }} · {{ item.task_title }} · V{{ item.version }}</p>
          <p class="mt-1 text-xs text-ink-muted">
            {{ item.author_name }} · {{ formatDateTime(item.submitted_at) }}
          </p></RouterLink
        ></template
      >
    </div>
  </section>
</template>
