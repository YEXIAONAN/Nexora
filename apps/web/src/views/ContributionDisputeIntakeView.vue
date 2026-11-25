<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { receiveContributionDisputeRequest } from '@/api/contributions'
import { contributionError } from '@/components/contributions/contributionPresentation'
import AlertBanner from '@/components/ui/AlertBanner.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import { useAuthStore } from '@/stores/auth'
const auth = useAuthStore()
const userId = computed(() => auth.user?.id ?? null)
const fields = reactive({
  team_id: '',
  project_id: '',
  case_id: '',
  case_version_id: '',
  confirmation_id: '',
  reason: '',
})
const labels = {
  team_id: '团队 ID',
  project_id: '项目 ID',
  case_id: '案例 ID',
  case_version_id: '核定版本 ID',
  confirmation_id: '核定记录 ID',
}
const pending = ref(false),
  error = ref(''),
  requestID = ref(''),
  received = ref('')
let generation = 0,
  controller: AbortController | null = null
watch(
  fields,
  () => {
    requestID.value = ''
    received.value = ''
  },
  { flush: 'sync' },
)
watch(
  userId,
  () => {
    generation++
    controller?.abort()
    pending.value = false
    error.value = ''
    received.value = ''
    Object.assign(fields, {
      team_id: '',
      project_id: '',
      case_id: '',
      case_version_id: '',
      confirmation_id: '',
      reason: '',
    })
  },
  { flush: 'sync' },
)
async function submit() {
  if (pending.value || !userId.value) return
  const captured = ++generation
  controller = new AbortController()
  pending.value = true
  error.value = ''
  received.value = ''
  requestID.value ||= crypto.randomUUID()
  try {
    const result = await receiveContributionDisputeRequest(
      { ...fields, client_request_id: requestID.value },
      controller.signal,
    )
    if (captured === generation && result.received) received.value = result.request_id
  } catch (reason) {
    if (captured === generation) error.value = contributionError(reason)
  } finally {
    if (captured === generation) pending.value = false
  }
}
onBeforeUnmount(() => {
  generation++
  controller?.abort()
})
</script>
<template>
  <div class="mx-auto max-w-2xl page-stack">
    <PageHeader
      eyebrow="贡献治理"
      title="本人异议请求受理"
      description="使用你已持有的准确标识提交本人声明。此入口仅受理请求，不提供项目、成员或贡献结果查询，也不显示核验结果。"
    />
    <AlertBanner v-if="error" tone="danger" :message="error" />
    <AlertBanner v-if="received" tone="success" title="请求已受理">
      <p class="text-body">
        受理编号：<span class="break-all font-mono">{{ received }}</span>
      </p>
      <p class="mt-1 text-caption">
        受理不表示核验成功、正式争议已经建立或你已收到核定结果。后续须由有权限人员按治理流程核对。
      </p>
    </AlertBanner>
    <form class="card" @submit.prevent="submit">
      <fieldset class="card-body space-y-4" :disabled="pending || !userId">
        <label v-for="(label, key) in labels" :key="key" class="form-label"
          >{{ label
          }}<input
            v-model.trim="fields[key]"
            class="form-input"
            required
            pattern="[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}" /></label
        ><label class="form-label"
          >本人异议声明<textarea
            v-model="fields.reason"
            class="form-textarea"
            required
            maxlength="1000"
            rows="5"
          />
        </label>
        <p class="text-caption leading-5 text-ink-muted">
          此请求不恢复离队成员的普通团队数据访问权。缺少可信收件证据时，项目归档继续受阻。
        </p>
        <button class="btn btn-primary" :disabled="!fields.reason.trim()">
          {{ pending ? '正在提交…' : '提交本人请求' }}
        </button>
      </fieldset>
    </form>
  </div>
</template>
