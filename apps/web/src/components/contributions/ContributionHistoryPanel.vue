<script setup lang="ts">
import type { ContributionHistory } from '@/api/contributions'
import type { ProjectMember } from '@/api/projects'
import ContributionVersionPanel from './ContributionVersionPanel.vue'
import { formatDateTime } from '@/utils/format'
defineProps<{ history: ContributionHistory; members: ProjectMember[] }>()
const eventLabels: Record<string, string> = {
  case_created: '建立案例',
  draft_updated: '更新草案',
  share_changed: '调整份额',
  submitted: '提交待核定',
  withdrawn: '撤回草案',
  confirmed: '正式核定',
  revision_created: '创建修订',
  dispute_raised: '发起争议',
  dispute_resolved: '独立裁定',
  abandoned: '放弃草案',
  notice_prepared: '准备告知',
  receipt_acknowledged: '本人确认收到',
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
            {{ formatDateTime(event.created_at) }} · {{ event.actor_id }}
          </p>
          <p class="mt-1 whitespace-pre-wrap">{{ event.reason }}</p>
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
