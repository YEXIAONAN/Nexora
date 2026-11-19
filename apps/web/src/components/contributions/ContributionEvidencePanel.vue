<script setup lang="ts">
import type { ContributionEvidence } from '@/api/contributions'
import { attachmentURL } from '@/api/submissions'
import { formatDateTime } from '@/utils/format'
defineProps<{ evidence: ContributionEvidence; team: string; project: string }>()
function safeURL(value: string) {
  try {
    const u = new URL(value)
    // Secure transport is required outside local development; credentials in
    // URLs and non-HTTP schemes are never rendered as active links.
    const loopback = ['localhost', '127.0.0.1', '::1', '[::1]'].includes(u.hostname)
    const allowed = u.protocol === 'https:' || (u.protocol === 'http:' && loopback)
    return allowed && !u.username && !u.password ? value : undefined
  } catch {
    return undefined
  }
}
</script>
<template>
  <section class="card" aria-label="正式验收证据">
    <div class="card-header"><h2 class="font-semibold">正式验收证据</h2></div>
    <div class="card-body space-y-3 text-sm">
      <h3 class="font-medium">{{ evidence.task_title }}</h3>
      <p class="whitespace-pre-wrap">{{ evidence.summary || '成果内容见附件与引用' }}</p>
      <p class="text-xs text-ink-muted">
        成果作者 {{ evidence.author_id }} · {{ formatDateTime(evidence.submitted_at) }}
      </p>
      <ul class="space-y-2">
        <li v-for="(ref, i) in evidence.evidence_references" :key="i">
          <a
            :href="safeURL(ref.url)"
            target="_blank"
            rel="noopener noreferrer"
            class="break-all text-accent-strong"
            >{{ ref.label || ref.url }}</a
          >
          · {{ ref.kind }}
        </li>
      </ul>
      <p
        v-if="evidence.attachments.length"
        class="rounded-lg bg-surface-muted p-3 text-xs leading-5 text-ink-muted"
      >
        附件下载沿用原任务读取权限；核定授权不会自动增加附件权限。若无法读取，请先由负责人通过现有审核人授权明确提供读取权限，核对证据后再核定。
      </p>
      <ul class="space-y-2">
        <li v-for="file in evidence.attachments" :key="file.id">
          <a
            :href="
              attachmentURL(
                team,
                project,
                evidence.task_id,
                evidence.submission_version_id,
                file.id,
              )
            "
            target="_blank"
            rel="noopener noreferrer"
            class="text-accent-strong"
            >下载 {{ file.file_name }}</a
          >
          · {{ file.byte_size }} 字节
          <p class="break-all text-xs text-ink-muted">SHA256：{{ file.sha256 }}</p>
        </li>
      </ul>
      <div class="rounded-lg bg-surface-muted p-3">
        <p>审核通过 · {{ evidence.reviewer_id }} · {{ formatDateTime(evidence.reviewed_at) }}</p>
        <p class="mt-2 whitespace-pre-wrap">{{ evidence.comment }}</p>
        <p class="mt-2 break-all text-xs text-ink-muted">审核记录：{{ evidence.review_id }}</p>
      </div>
      <p class="text-xs text-ink-muted">
        成果事实与人工贡献判断分别展示。任务负责人、协作者、提交次数和代码行数均不用于自动分配贡献份额。
      </p>
    </div>
  </section>
</template>
