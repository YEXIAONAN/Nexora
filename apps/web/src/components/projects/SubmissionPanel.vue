<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { ApiError } from '@/api/client'
import * as api from '@/api/submissions'
import { transitionTask, type Task } from '@/api/tasks'
import { formatDateTime } from '@/utils/format'
const props = defineProps<{ team: string; project: string; task: Task }>()
const emit = defineEmits<{ changed: [] }>()
const context = computed(
  () => `${props.team}/${props.project}/${props.task.id}/${props.task.version}`,
)
const versions = ref<api.SubmissionVersion[]>([]),
  selectedId = ref(''),
  confirmed = ref('')
const loading = ref(false),
  busy = ref(false),
  error = ref(''),
  summary = ref(''),
  comment = ref(''),
  correction = ref('')
const references = ref<api.EvidenceReference[]>([])
const files = ref<{ file: File; key: string; attachment?: api.Attachment }[]>([])
const draftKey = ref(crypto.randomUUID())
let generation = 0
const ready = computed(() => confirmed.value === context.value && !loading.value)
const selected = computed(() => versions.value.find((v) => v.id === selectedId.value))
const canReview = computed(
  () =>
    ready.value &&
    props.task.can_review &&
    selected.value?.id === props.task.current_submission_version_id &&
    !selected.value?.review,
)
const describe = (e: unknown) =>
  e instanceof ApiError
    ? `${e.message}${e.fields ? ' · ' + Object.values(e.fields).join('；') : ''}`
    : '成果操作失败，请重试'
const safeURL = (url: string) => {
  try {
    const u = new URL(url)
    return ['http:', 'https:'].includes(u.protocol) && !u.username && !u.password ? url : undefined
  } catch {
    return undefined
  }
}
function selectVersion(id: string) {
  selectedId.value = id
  comment.value = ''
}
async function load() {
  const key = context.value,
    gen = ++generation,
    { team, project, task } = props
  loading.value = true
  confirmed.value = ''
  error.value = ''
  versions.value = []
  try {
    const result = await api.listVersions(team, project, task.id)
    if (key !== context.value || gen !== generation) return
    versions.value = result
    selectedId.value =
      result.find((v) => v.id === task.current_submission_version_id)?.id ?? result[0]?.id ?? ''
    confirmed.value = key
  } catch (e) {
    if (key === context.value && gen === generation) error.value = describe(e)
  } finally {
    if (key === context.value && gen === generation) loading.value = false
  }
}
function chooseFiles(event: Event) {
  const input = event.target as HTMLInputElement
  const chosen = Array.from(input.files ?? [])
  if (
    chosen.length > 5 ||
    chosen.some((f) => f.size > 10 * 1024 * 1024 || !f.size) ||
    chosen.reduce((n, f) => n + f.size, 0) > 25 * 1024 * 1024
  ) {
    error.value = '最多 5 个文件，单个不超过 10 MiB，合计不超过 25 MiB'
    input.value = ''
    files.value = []
    return
  }
  files.value = chosen.map((file) => ({ file, key: crypto.randomUUID() }))
  error.value = ''
}
async function submit() {
  if (!ready.value || busy.value || !props.task.can_submit) return
  const key = confirmed.value,
    gen = generation,
    { team, project, task } = props
  const draft = {
    client_request_id: draftKey.value,
    expected_version: task.version,
    summary: summary.value,
    references: references.value.map((r) => ({ ...r })),
    attachment_ids: [] as string[],
  }
  const pending = files.value.slice()
  busy.value = true
  error.value = ''
  try {
    for (const item of pending) {
      item.attachment ??= await api.uploadAttachment(team, project, task.id, item.file, item.key)
      if (key !== context.value || gen !== generation) return
      draft.attachment_ids.push(item.attachment.id)
    }
    if (key !== context.value || gen !== generation) return
    await api.submitVersion(team, project, task.id, draft)
    if (key === context.value && gen === generation) {
      summary.value = ''
      references.value = []
      files.value = []
      emit('changed')
    }
  } catch (e) {
    if (key === context.value && gen === generation) error.value = describe(e)
  } finally {
    if (key === context.value && gen === generation) busy.value = false
  }
}
async function review(decision: 'approve' | 'reject') {
  if (!canReview.value || busy.value || !comment.value.trim() || !selected.value) return
  const key = confirmed.value,
    gen = generation,
    { team, project, task } = props,
    version = selected.value.id
  const body = { expected_version: task.version, decision, comment: comment.value }
  busy.value = true
  error.value = ''
  try {
    await api.reviewVersion(team, project, task.id, version, body)
    if (key === context.value && gen === generation) {
      comment.value = ''
      emit('changed')
    }
  } catch (e) {
    if (key === context.value && gen === generation) error.value = describe(e)
  } finally {
    if (key === context.value && gen === generation) busy.value = false
  }
}
async function correct() {
  if (!ready.value || busy.value || !props.task.can_execute || !correction.value.trim()) return
  const key = confirmed.value,
    gen = generation,
    { team, project, task } = props,
    reason = correction.value
  busy.value = true
  error.value = ''
  try {
    await transitionTask(team, project, task, 'in_progress', reason)
    if (key === context.value && gen === generation) emit('changed')
  } catch (e) {
    if (key === context.value && gen === generation) error.value = describe(e)
  } finally {
    if (key === context.value && gen === generation) busy.value = false
  }
}
watch(
  [summary, () => JSON.stringify(references.value), () => files.value.map((f) => f.key).join(',')],
  () => {
    if (!busy.value) draftKey.value = crypto.randomUUID()
  },
)
watch(
  context,
  () => {
    busy.value = false
    summary.value = ''
    references.value = []
    files.value = []
    comment.value = ''
    correction.value = ''
    void load()
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  generation++
})
</script>
<template>
  <section class="card" aria-label="成果与验收">
    <div class="card-header">
      <h3 class="font-semibold">成果与验收</h3>
      <button class="btn btn-ghost" :disabled="loading || busy" @click="load">刷新成果</button>
    </div>
    <div class="card-body space-y-5">
      <p class="text-xs leading-5 text-ink-muted">
        每次提交保留独立版本。待验收为 80%，驳回为 0%，当前版本正式通过后才完成。
      </p>
      <p v-if="error" role="alert" class="text-sm text-danger">{{ error }}</p>
      <p v-if="loading" role="status" class="text-sm text-ink-muted">正在加载成果版本…</p>
      <fieldset v-if="ready" :disabled="busy" class="space-y-5">
        <form
          v-if="task.status === 'rejected' && task.can_execute"
          class="flex flex-wrap items-end gap-2"
          @submit.prevent="correct"
        >
          <label class="form-label grow"
            >补正原因<input
              v-model="correction"
              required
              maxlength="1000"
              class="form-input" /></label
          ><button class="btn" type="submit">进入补正</button>
        </form>
        <form
          v-if="task.can_submit"
          class="space-y-3 rounded-lg border border-line p-4"
          @submit.prevent="submit"
        >
          <h4 class="text-sm font-semibold">
            {{ versions.length ? '重新提交新版本' : '提交成果 V1' }}
          </h4>
          <label class="form-label"
            >成果说明<textarea
              v-model="summary"
              rows="4"
              maxlength="8000"
              class="form-input"
              placeholder="说明交付内容与验证方法"
            />
          </label>
          <div
            v-for="(item, index) in references"
            :key="index"
            class="grid gap-2 rounded-lg bg-surface-muted p-3"
          >
            <label class="form-label"
              >证据类型<select aria-label="证据类型" v-model="item.kind" class="form-input">
                <option value="link">外部链接</option>
                <option value="git_commit">Git Commit</option>
                <option value="pull_request">Pull Request</option>
              </select></label
            >
            <label class="form-label"
              >证据地址<input
                v-model="item.url"
                type="url"
                required
                maxlength="2048"
                class="form-input"
                placeholder="https://"
            /></label>
            <label class="form-label"
              >证据标签<input v-model="item.label" maxlength="200" class="form-input" /></label
            ><button class="btn btn-ghost" type="button" @click="references.splice(index, 1)">
              移除此链接
            </button>
          </div>
          <button
            class="btn"
            type="button"
            :disabled="references.length >= 20"
            @click="references.push({ kind: 'link', url: '', label: '' })"
          >
            添加证据链接
          </button>
          <label class="form-label"
            >成果附件<input
              type="file"
              multiple
              accept=".txt,.md,.csv,.pdf,.png,.jpg,.jpeg"
              class="form-input"
              @change="chooseFiles"
          /></label>
          <p class="text-xs text-ink-muted">
            最多 5 个文件；单个 10 MiB、合计 25 MiB。支持文本、PDF、PNG 和
            JPEG。附件仅供有权限的成员下载。
          </p>
          <ul class="space-y-1 text-xs text-ink-muted">
            <li v-for="item in files" :key="item.key">
              {{ item.file.name }} · {{ item.attachment ? '已暂存，等待版本绑定' : '提交时上传' }}
            </li>
          </ul>
          <button
            class="btn btn-primary"
            type="submit"
            :disabled="!summary.trim() && !references.length && !files.length"
          >
            {{ busy ? '正在提交…' : '提交新成果版本' }}
          </button>
        </form>
        <p v-if="task.status === 'awaiting_review'" class="rounded-lg bg-accent-soft p-3 text-sm">
          成果已提交，等待独立审核；任务尚未完成。
        </p>
        <p v-if="!versions.length" class="text-sm text-ink-muted">暂无成果版本。</p>
        <div v-else class="space-y-4">
          <div class="flex flex-wrap gap-2">
            <button
              v-for="v in versions"
              :key="v.id"
              class="btn"
              :aria-pressed="selectedId === v.id"
              @click="selectVersion(v.id)"
            >
              V{{ v.version }}{{ task.current_submission_version_id === v.id ? ' · 当前版本' : '' }}
            </button>
          </div>
          <article
            v-if="selected"
            class="space-y-3 rounded-lg border border-line p-4"
            data-testid="submission-detail"
          >
            <h4 class="font-semibold">成果 V{{ selected.version }} · {{ selected.author_name }}</h4>
            <p class="text-xs text-ink-subtle">{{ formatDateTime(selected.submitted_at) }}</p>
            <p class="whitespace-pre-wrap text-sm leading-6">
              {{ selected.summary || '成果内容见链接与附件' }}
            </p>
            <ul class="space-y-2 text-sm">
              <li v-for="(link, i) in selected.references" :key="i">
                <a
                  :href="safeURL(link.url)"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="break-all text-accent-strong"
                  >{{ link.label || link.url }}</a
                ><span class="ml-2 text-xs text-ink-subtle">{{ link.kind }}</span>
              </li>
            </ul>
            <ul class="space-y-2 text-sm">
              <li v-for="file in selected.attachments" :key="file.id">
                <a
                  :href="api.attachmentURL(team, project, task.id, selected.id, file.id)"
                  class="text-accent-strong"
                  >下载 {{ file.file_name }}</a
                ><span class="ml-2 text-xs text-ink-subtle">{{ file.byte_size }} 字节</span>
              </li>
            </ul>
            <div
              v-if="selected.review"
              class="rounded-lg bg-surface-muted p-3 text-sm"
              data-testid="review-decision"
            >
              <p>
                {{ selected.review.decision === 'approve' ? '审核通过' : '审核驳回' }} ·
                {{ selected.review.reviewer_name }} ·
                {{ formatDateTime(selected.review.reviewed_at) }}
              </p>
              <p class="mt-2 whitespace-pre-wrap">{{ selected.review.comment }}</p>
            </div>
            <form
              v-if="canReview"
              class="space-y-3 border-t border-line pt-3"
              @submit.prevent="review('approve')"
            >
              <p class="text-xs text-ink-muted">
                正在审核当前成果 V{{ selected.version }}，决定将绑定此版本。
              </p>
              <label class="form-label"
                >审核意见<textarea
                  v-model="comment"
                  required
                  maxlength="4000"
                  rows="3"
                  class="form-input"
                />
              </label>
              <div class="flex gap-2">
                <button class="btn btn-primary" type="submit" :disabled="!comment.trim()">
                  通过当前版本</button
                ><button
                  class="btn text-danger"
                  type="button"
                  :disabled="!comment.trim()"
                  @click="review('reject')"
                >
                  驳回当前版本
                </button>
              </div>
            </form>
          </article>
          <div class="border-t border-line pt-4">
            <h4 class="text-sm font-semibold">审核历史时间线</h4>
            <ol class="mt-3 space-y-3">
              <li
                v-for="v in [...versions].reverse()"
                :key="v.id"
                class="border-l-2 border-line pl-3 text-xs text-ink-muted"
              >
                <p>
                  V{{ v.version }} · {{ v.author_name }} 提交 · {{ formatDateTime(v.submitted_at) }}
                </p>
                <p v-if="v.review" class="mt-1">
                  {{ v.review.reviewer_name }}
                  {{ v.review.decision === 'approve' ? '通过' : '驳回' }}：{{ v.review.comment }}
                </p>
                <p v-else class="mt-1">待审核</p>
              </li>
            </ol>
          </div>
        </div>
      </fieldset>
    </div>
  </section>
</template>
