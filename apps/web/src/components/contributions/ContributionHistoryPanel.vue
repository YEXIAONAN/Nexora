<script setup lang="ts">
import type { ContributionHistory } from '@/api/contributions'
import type { ProjectMember } from '@/api/projects'
import ContributionVersionPanel from './ContributionVersionPanel.vue'
import { memberDisplayName } from './contributionPresentation'
import { formatDateTime } from '@/utils/format'
defineProps<{ history: ContributionHistory; members: ProjectMember[] }>()
const eventLabels: Record<string, string> = {
  case_created: '建立案例',
  version_created: '创建工作版本',
  version_draft: '草案内容或状态变更',
  version_pending_review: '版本提交待核定',
  version_confirmed: '版本已确认',
  version_superseded: '版本已被替代',
  version_overturned: '版本已被推翻',
  version_abandoned: '草案已放弃',
  share_adjusted: '人工调整贡献份额',
  confirmed: '正式独立核定',
  disputed: '发起争议',
  resolution_uphold: '独立裁定：维持原核定',
  resolution_overturn: '独立裁定：推翻原核定',
  contribution_notifications: '准备结果告知（不代表送达）',
  contribution_attestations: '记录核验凭据',
  contribution_conflicts: '登记直接利益冲突',
}
</script>
<template>
  <section class="card" aria-label="完整贡献历史">
    <div class="card-header"><h2 class="font-semibold">历史版本、核定与裁定</h2></div>
    <div class="card-body space-y-4">
      <p class="text-xs leading-5 text-ink-muted">
        历史核定与裁定不可修改。被替代、被推翻及已放弃的版本仍完整保留；修订不能抹去过去的决定。
      </p>
      <details
        v-for="version in history.versions"
        :key="version.id"
        class="rounded-lg border border-line p-3"
      >
        <summary class="cursor-pointer text-sm font-medium">
          V{{ version.version }} ·
          {{
            {
              draft: '草案',
              pending_review: '待核定',
              confirmed: '已确认',
              superseded: '已被替代',
              overturned: '已被推翻',
              abandoned: '已放弃',
            }[version.status]
          }}
        </summary>
        <div class="mt-3"><ContributionVersionPanel :version="version" :members="members" /></div>
      </details>
      <h3 class="text-sm font-semibold">事件记录</h3>
      <p v-if="!history.events.length" class="text-xs text-ink-muted">暂无事件记录。</p>
      <ol class="space-y-3">
        <li
          v-for="event in history.events"
          :key="event.id"
          class="border-l-2 border-line pl-3 text-xs text-ink-muted"
        >
          <p>
            {{ eventLabels[event.event_type] || event.event_type }} ·
            {{ formatDateTime(event.created_at) }} ·
            {{ memberDisplayName(members, event.actor_id) }}
          </p>
          <p class="mt-1 whitespace-pre-wrap">
            {{ event.reason === 'contribution evidence recorded' ? '记录贡献证据' : event.reason }}
          </p>
          <p class="mt-1 break-all">
            版本 {{ event.case_version_id || '案例级事件' }} · 请求 {{ event.request_id }}
          </p>
        </li>
      </ol>
      <details v-if="history.adjustments.length">
        <summary class="cursor-pointer text-sm font-medium">
          份额调整记录（{{ history.adjustments.length }}）
        </summary>
        <ul class="mt-3 space-y-2 text-xs text-ink-muted">
          <li v-for="(a, i) in history.adjustments" :key="i">
            {{ a.user_id }}：{{ a.old_bps ?? '无' }} → {{ a.new_bps ?? '删除' }} 基点 ·
            {{ a.reason }} · {{ formatDateTime(a.created_at) }}
          </li>
        </ul>
      </details>
    </div>
  </section>
</template>
