<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import type { Task, ProgressReport, ProgressReportInput } from '@/api/tasks'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{
  task: Task
  reports: ProgressReport[]
  loading: boolean
  canReport: boolean
  busy: boolean
}>()
const emit = defineEmits<{ submit: [ProgressReportInput]; refresh: [] }>()

const form = reactive({
  progress: props.task.actual_progress,
  summary: '',
  completed: '',
  pending: '',
  links: '',
})
const localError = ref('')

watch(
  () => props.task.id,
  () => {
    form.progress = props.task.actual_progress
    form.summary = ''
    form.completed = ''
    form.pending = ''
    form.links = ''
    localError.value = ''
  },
)

function evidenceLinks(): string[] {
  return form.links
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
}

function submit() {
  localError.value = ''
  if (!Number.isInteger(form.progress) || form.progress < 0 || form.progress > 99) {
    localError.value = '进度必须是 0–99 之间的整数。'
    return
  }
  if (!form.summary.trim()) {
    localError.value = '请填写本次工作内容。'
    return
  }
  const links = evidenceLinks()
  if (links.length > 20) {
    localError.value = '交付引用最多 20 条。'
    return
  }
  if (links.some((link) => link.length > 500)) {
    localError.value = '单条交付引用不能超过 500 个字符。'
    return
  }
  emit('submit', {
    reported_progress: form.progress,
    work_summary: form.summary.trim(),
    completed_work: form.completed.trim(),
    pending_work: form.pending.trim(),
    evidence_links: links,
    expected_version: props.task.version,
  })
}

function isLink(value: string): boolean {
  return /^https?:\/\//i.test(value)
}

function actorName(report: ProgressReport): string {
  return report.actor_display_name || report.actor_username || '成员'
}
</script>

<template>
  <section class="space-y-4" aria-label="工作进展">
    <div v-if="canReport" class="rounded-xl border border-line bg-surface p-4">
      <div class="flex flex-wrap items-baseline justify-between gap-2">
        <h4 class="text-sm font-semibold text-ink">上报本次进展</h4>
        <p class="text-[11px] text-ink-subtle">上报进展不会提交验收，也不会自动完成任务。</p>
      </div>
      <form class="mt-3 space-y-3" @submit.prevent="submit">
        <p v-if="localError" role="alert" class="text-xs text-danger">{{ localError }}</p>
        <div class="grid gap-3 sm:grid-cols-[120px_minmax(0,1fr)]">
          <label class="form-label">
            当前进度
            <span class="flex items-center gap-2">
              <input
                v-model.number="form.progress"
                type="range"
                min="0"
                max="99"
                class="w-full accent-[var(--accent)]"
                aria-label="当前进度"
              />
              <span class="w-12 text-right text-xs text-ink">{{ form.progress }}%</span>
            </span>
          </label>
          <label class="form-label">
            本次工作内容
            <textarea
              v-model="form.summary"
              required
              maxlength="4000"
              rows="2"
              class="form-input"
              placeholder="例：完成登录接口与错误处理，补齐回归测试"
            />
          </label>
        </div>
        <div class="grid gap-3 sm:grid-cols-2">
          <label class="form-label">
            已完成工作（可选）
            <textarea
              v-model="form.completed"
              maxlength="4000"
              rows="2"
              class="form-input"
              placeholder="已经交付或验证的部分"
            />
          </label>
          <label class="form-label">
            待完成 / 遇到的问题（可选）
            <textarea
              v-model="form.pending"
              maxlength="4000"
              rows="2"
              class="form-input"
              placeholder="剩余工作、风险或需要协助的事项"
            />
          </label>
        </div>
        <label class="form-label">
          交付证明（可选，每行一条链接 / Commit / PR）
          <textarea
            v-model="form.links"
            rows="2"
            class="form-input font-mono text-xs"
            placeholder="https://github.com/org/repo/pull/42&#10;commit 1a2b3c4"
          />
        </label>
        <div class="flex flex-wrap items-center justify-between gap-2">
          <p class="text-[11px] text-ink-subtle">
            文件附件暂未开放；请使用链接或 Commit 引用交付证据。
          </p>
          <button class="btn btn-primary btn-sm" type="submit" :disabled="busy">
            {{ busy ? '提交中…' : '提交进展' }}
          </button>
        </div>
      </form>
    </div>

    <div
      v-else
      class="rounded-xl border border-dashed border-line bg-surface-muted px-4 py-3 text-xs leading-5 text-ink-muted"
    >
      <template v-if="task.status === 'pending'">
        任务尚未开始。负责人或协作者点击「开始任务」后即可上报进展。
      </template>
      <template v-else-if="task.status === 'awaiting_review'">
        交付已提交，正在等待验收。验收驳回后可继续上报修正进展。
      </template>
      <template v-else-if="task.status === 'completed'">
        任务已通过验收。进展记录只读保留，作为执行过程证据。
      </template>
      <template v-else-if="task.status === 'cancelled'">任务已取消，不能再上报进展。</template>
      <template v-else-if="task.status === 'rejected'">
        交付被驳回。点击「开始修正」或到「交付与验收」进入补正，修正后再上报新的进展。
      </template>
      <template v-else>只有任务负责人或协作者可以上报进展。</template>
    </div>

    <div class="flex items-center justify-between">
      <h4 class="text-sm font-semibold text-ink">进展记录</h4>
      <button
        class="btn btn-ghost btn-sm"
        type="button"
        :disabled="loading"
        @click="emit('refresh')"
      >
        {{ loading ? '加载中…' : '刷新' }}
      </button>
    </div>

    <p v-if="loading && !reports.length" class="text-xs text-ink-muted">正在加载进展记录…</p>
    <ol v-else-if="reports.length" class="space-y-3">
      <li
        v-for="report in reports"
        :key="report.id"
        class="rounded-xl border border-line bg-surface p-4"
      >
        <div class="flex flex-wrap items-start justify-between gap-2">
          <div class="flex min-w-0 items-center gap-2.5">
            <span class="avatar h-7 w-7 text-[11px]">{{ actorName(report).slice(0, 1) }}</span>
            <div class="min-w-0">
              <p class="truncate text-sm font-medium text-ink">{{ actorName(report) }}</p>
              <p class="text-[11px] text-ink-subtle">{{ formatDateTime(report.created_at) }}</p>
            </div>
          </div>
          <span class="badge">上报 {{ report.reported_progress }}%</span>
        </div>
        <p class="mt-3 whitespace-pre-wrap text-sm leading-6 text-ink">
          {{ report.work_summary }}
        </p>
        <dl
          v-if="report.completed_work || report.pending_work"
          class="mt-3 grid gap-3 sm:grid-cols-2"
        >
          <div v-if="report.completed_work">
            <dt class="field-label">已完成</dt>
            <dd class="mt-1 whitespace-pre-wrap text-xs leading-5 text-ink-muted">
              {{ report.completed_work }}
            </dd>
          </div>
          <div v-if="report.pending_work">
            <dt class="field-label">待完成 / 问题</dt>
            <dd class="mt-1 whitespace-pre-wrap text-xs leading-5 text-ink-muted">
              {{ report.pending_work }}
            </dd>
          </div>
        </dl>
        <ul v-if="report.evidence_links.length" class="mt-3 flex flex-wrap gap-2">
          <li v-for="link in report.evidence_links" :key="link">
            <a
              v-if="isLink(link)"
              :href="link"
              target="_blank"
              rel="noopener noreferrer"
              class="badge text-accent-strong hover:underline"
              >{{ link }}</a
            >
            <span v-else class="badge font-mono">{{ link }}</span>
          </li>
        </ul>
      </li>
    </ol>
    <p
      v-else
      class="rounded-xl border border-dashed border-line px-4 py-6 text-center text-xs text-ink-muted"
    >
      还没有进展记录。第一次上报后，这里会保留每次工作说明和交付引用。
    </p>
  </section>
</template>
