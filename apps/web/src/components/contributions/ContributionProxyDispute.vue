<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import * as api from '@/api/contributions'
import { useContributionResource } from '@/composables/useContributionResource'
import { useAuthStore } from '@/stores/auth'
import { formatDateTime } from '@/utils/format'
const props = defineProps<{
  team: string
  project: string
  detail: api.ContributionCaseDetail
  busy: boolean
}>()
const emit = defineEmits<{ submit: [body: api.ContributionDisputeRequest] }>()
const auth = useAuthStore(),
  input = ref(''),
  lookup = ref(''),
  acknowledged = ref(false)
const context = computed(() =>
  lookup.value && auth.user
    ? JSON.stringify([
        props.team,
        props.project,
        props.detail.case.id,
        props.detail.case.revision,
        lookup.value,
        auth.user.id,
      ])
    : null,
)
const resource = useContributionResource(
  () => context.value,
  async (scope, signal) => {
    const [t, p, c, , id] = JSON.parse(scope) as [string, string, string, number, string, string]
    return api.getVerifiedContributionDisputeRequest(t, p, c, id, signal)
  },
)
const { data, loading, error } = resource
const matches = computed(
  () =>
    !!data.value &&
    data.value.id === lookup.value &&
    data.value.case_version_id === props.detail.effective?.id &&
    data.value.confirmation_id === props.detail.effective?.confirmation?.id &&
    data.value.subject_id !== auth.user?.id,
)
watch(
  () => `${props.team}/${props.project}/${props.detail.case.id}/${props.detail.case.revision}`,
  () => {
    lookup.value = ''
    input.value = ''
    acknowledged.value = false
  },
  { flush: 'sync' },
)
watch(
  input,
  () => {
    lookup.value = ''
    acknowledged.value = false
  },
  { flush: 'sync' },
)
function inspect() {
  if (props.busy || loading.value) return
  const requested = input.value.trim()
  if (!requested) return
  acknowledged.value = false
  if (lookup.value === requested) void resource.reload()
  else lookup.value = requested
}
function submit() {
  const evidence = data.value
  if (props.busy || loading.value || !matches.value || !acknowledged.value || !evidence) return
  emit('submit', {
    expected_version: props.detail.case.revision,
    client_request_id: crypto.randomUUID(),
    reason: evidence.statement,
    case_version_id: evidence.case_version_id,
    confirmation_id: evidence.confirmation_id,
    on_behalf_of: evidence.subject_id,
    identity_request_id: evidence.id,
  })
}
</script>
<template>
  <details class="rounded-lg border border-line p-4">
    <summary class="cursor-pointer text-sm font-medium">核对本人请求凭据并代提异议</summary>
    <div class="mt-3 space-y-3">
      <p class="text-xs leading-5 text-ink-muted">
        仅能引用已存在、经服务端核验且绑定此有效核定的本人声明。受理回执本身不代表核验通过；代提不等于结果送达，也不会开放离队者项目数据。
      </p>
      <form class="flex flex-wrap items-end gap-2" @submit.prevent="inspect">
        <label class="form-label grow"
          >本人请求标识<input v-model="input" required class="form-input" /></label
        ><button type="submit" class="btn" :disabled="busy || loading || !input.trim()">
          核对请求凭据
        </button>
      </form>
      <p v-if="error" role="alert" class="text-sm text-danger">{{ error }}</p>
      <p v-if="loading" role="status" class="text-xs text-ink-muted">正在核对实际凭据…</p>
      <div v-if="data" class="space-y-2 rounded-lg bg-surface-muted p-3 text-xs text-ink-muted">
        <p>声明人：{{ data.subject_id }}</p>
        <p class="break-all">版本：{{ data.case_version_id }} · 核定：{{ data.confirmation_id }}</p>
        <p class="whitespace-pre-wrap">原始声明：{{ data.statement }}</p>
        <p class="break-all">声明摘要：{{ data.content_digest }}</p>
        <p>服务端核验时间：{{ formatDateTime(data.verified_at) }}</p>
        <p v-if="!matches" class="text-danger">
          凭据不匹配当前有效核定，或属于当前操作者本人，不能据此代提。
        </p>
      </div>
      <form v-if="matches" class="space-y-3" @submit.prevent="submit">
        <label class="flex items-start gap-2 text-sm"
          ><input
            v-model="acknowledged"
            type="checkbox"
            class="mt-1"
          />已核对声明人、版本和原始声明；按原文代提</label
        ><button class="btn" type="submit" :disabled="busy || loading || !acknowledged">
          依据已核验本人请求代提异议
        </button>
      </form>
    </div>
  </details>
</template>
