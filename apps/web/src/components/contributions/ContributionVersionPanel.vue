<script setup lang="ts">
import type { ContributionVersion } from '@/api/contributions'
import type { ProjectMember } from '@/api/projects'
import { formatDateTime } from '@/utils/format'
import { formatBasisPoints as percent, memberDisplayName } from './contributionPresentation'
const props = defineProps<{
  version: ContributionVersion
  members?: ProjectMember[]
  effective?: boolean
}>()
const statusLabels: Record<ContributionVersion['status'], string> = {
  draft: '草案 · 尚未正式核定',
  pending_review: '待独立核定',
  confirmed: '已确认',
  superseded: '已被后续版本替代',
  overturned: '已被裁定推翻',
  abandoned: '已放弃的草案',
}
const name = (id: string) => memberDisplayName(props.members, id)
</script>
<template>
  <article class="space-y-3 rounded-lg border border-line p-4">
    <h3 class="font-semibold">V{{ version.version }} · {{ statusLabels[version.status] }}</h3>
    <p v-if="effective" class="text-sm text-ink-muted">
      此版本由正式有效指针指定；最新草案不会覆盖它。
    </p>
    <p class="whitespace-pre-wrap text-sm">{{ version.summary || '暂无贡献说明' }}</p>
    <details v-if="Object.keys(version.evidence_snapshot).length" class="text-xs text-ink-muted">
      <summary class="cursor-pointer">人工证据快照</summary>
      <pre class="mt-2 overflow-auto whitespace-pre-wrap">{{
        JSON.stringify(version.evidence_snapshot, null, 2)
      }}</pre>
    </details>
    <p class="break-all text-xs text-ink-muted">版本标识：{{ version.id }}</p>
    <p v-if="!version.shares.length" class="text-sm text-ink-muted">尚未填写贡献份额。</p>
    <ul class="space-y-3">
      <li
        v-for="share in version.shares"
        :key="share.user_id"
        class="rounded-lg bg-surface-muted p-3 text-sm"
      >
        <p class="font-medium">
          {{ name(share.user_id) }} · {{ percent(share.share_bps) }}
          <span v-if="share.membership_state === 'historical'" class="badge">历史成员</span>
        </p>
        <p class="mt-2 whitespace-pre-wrap">贡献依据：{{ share.basis || '未填写' }}</p>
        <p class="mt-1 whitespace-pre-wrap">贡献说明：{{ share.statement || '未填写' }}</p>
        <ul class="mt-2 space-y-1 text-xs text-ink-muted">
          <li v-for="(e, i) in share.evidence_refs" :key="i">
            {{
              { objective_fact: '客观事实', self_statement: '本人陈述', third_party: '第三方佐证' }[
                e.kind
              ]
            }}
            · {{ e.source }}：{{ e.reference }}
          </li>
        </ul>
      </li>
    </ul>
    <p class="text-xs text-ink-muted">
      合计 {{ percent(version.total_bps) }}（{{ version.total_bps }} / 10000 基点）
    </p>
    <div
      v-if="version.confirmation"
      class="space-y-1 border-t border-line pt-3 text-xs text-ink-muted"
      data-testid="immutable-confirmation"
    >
      <p>
        不可变核定记录 · {{ name(version.confirmation.confirmed_by) }} ·
        {{ formatDateTime(version.confirmation.confirmed_at) }}
      </p>
      <p class="whitespace-pre-wrap">核定理由：{{ version.confirmation.rationale }}</p>
      <p class="break-all">核定标识：{{ version.confirmation.id }}</p>
      <p class="break-all">内容摘要：{{ version.confirmation.version_digest }}</p>
    </div>
  </article>
</template>
