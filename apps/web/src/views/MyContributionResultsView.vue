<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { ApiError } from '@/api/client'
import * as api from '@/api/contributions'
import { listProjectMembers, listProjects, type Project, type ProjectMember } from '@/api/projects'
import { useContributionResource } from '@/composables/useContributionResource'
import {
  formatBasisPoints,
  memberDisplayName,
  versionStatusLabel,
  verifiedReceipt,
} from '@/components/contributions/contributionPresentation'
import { useAuthStore } from '@/stores/auth'
import { useTeamsStore } from '@/stores/teams'
import { formatDateTime } from '@/utils/format'

interface CaseRead {
  detail: api.ContributionCaseDetail
  evidence: api.ContributionEvidence
  versions: api.ContributionVersion[]
}
interface ResultRow extends CaseRead {
  project: Project
  members: ProjectMember[]
  result: api.ContributionResult
  version: api.ContributionVersion
}
const route = useRoute(),
  auth = useAuthStore(),
  teams = useTeamsStore()
const teamId = computed(() => String(route.params.teamId ?? ''))
watch([teamId, () => teams.teams], ([id]) => teams.selectTeam(id), {
  immediate: true,
  flush: 'sync',
})
const key = computed(() =>
  auth.user && (!teams.currentTeamId || teams.currentTeamId === teamId.value)
    ? `${teamId.value}/${auth.user.id}`
    : null,
)
const read = ref<Record<string, boolean>>({})
const { data, loading, pending, error, reload, mutate } = useContributionResource(
  keyGetter,
  async (context, signal) => {
    const [team] = context.split('/')
    const projects = await listProjects(team!, signal)
    const unavailable: string[] = []
    const grouped = await Promise.all(
      projects.map(async (project): Promise<ResultRow[]> => {
        try {
          const results = await api.listMyContributionResults(team!, project.id, signal)
          if (!results.length) return []
          const members = await listProjectMembers(team!, project.id, signal)
          // One cache per loader invocation only: nothing survives a Team,
          // Project or identity switch and no data is shared across contexts.
          const caseIds = [...new Set(results.map((result) => result.notice.case_id))]
          const cases = new Map<string, CaseRead>()
          await Promise.all(
            caseIds.map(async (caseId) => {
              const [detail, evidence, versions] = await Promise.all([
                api.getContribution(team!, project.id, caseId, signal),
                api.getContributionEvidence(team!, project.id, caseId, signal),
                api.listContributionVersions(team!, project.id, caseId, signal),
              ])
              cases.set(caseId, { detail, evidence, versions })
            }),
          )
          return results.map((result) => {
            const loaded = cases.get(result.notice.case_id)
            if (!loaded) throw new ApiError('结果版本无法核对，请刷新重试')
            const version = loaded.versions.find((v) => v.id === result.notice.case_version_id)
            if (!version || version.confirmation?.id !== result.confirmation.id)
              throw new ApiError('结果版本无法核对，请刷新重试')
            return { project, members, result, ...loaded, version }
          })
        } catch (reason) {
          if (reason instanceof ApiError && [403, 404].includes(reason.status ?? 0)) {
            unavailable.push(project.name)
            return []
          }
          throw reason
        }
      }),
    )
    return { rows: grouped.flat(), unavailable }
  },
)
function keyGetter() {
  return key.value
}
watch(
  key,
  () => {
    read.value = {}
  },
  { immediate: true, flush: 'sync' },
)
watch(data, () => {
  read.value = {}
})
function ownShare(row: ResultRow) {
  return row.result.shares.find((s) => s.user_id === auth.user?.id)
}
function canReceive(row: ResultRow) {
  return (
    row.project.status === 'active' &&
    row.result.notice.recipient_id === auth.user?.id &&
    row.result.notice.channel === 'authenticated_ack' &&
    !verifiedReceipt(row.result.notice, row.result.confirmation)
  )
}
async function receive(row: ResultRow) {
  if (!read.value[row.result.notice.id] || !canReceive(row) || pending.value) return
  // Capture the exact displayed result; a conflict requires a fresh review and explicit click.
  const { notice, confirmation } = row.result
  await mutate((context, signal) =>
    api.acknowledgeContributionReceipt(
      context.split('/')[0]!,
      row.project.id,
      notice.case_id,
      notice.id,
      {
        case_version_id: notice.case_version_id,
        confirmation_id: confirmation.id,
        content_digest: confirmation.version_digest,
        expected_version: row.detail.case.revision,
        client_request_id: crypto.randomUUID(),
        received: true,
      },
      signal,
    ),
  )
}
</script>

<template>
  <div class="space-y-6">
    <header class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <p class="text-xs text-ink-subtle">当前团队 · 本人结果</p>
        <h2 class="mt-1 text-2xl font-semibold">我的贡献结果</h2>
        <p class="mt-2 max-w-2xl text-sm leading-6 text-ink-muted">
          查看向你准备的正式核定结果。每份结果独立确认收件；历史核定始终保留。
        </p>
      </div>
      <button class="btn" :disabled="loading || pending" @click="reload()">刷新结果</button>
    </header>
    <p v-if="error" role="alert" class="rounded-lg bg-danger-soft p-4 text-sm text-danger">
      {{ error }}
    </p>
    <p v-if="loading" role="status" class="card p-8 text-sm text-ink-muted">
      正在读取本人贡献结果…
    </p>
    <p v-if="auth.user && !key" role="status" class="card p-5 text-sm text-ink-muted">
      当前团队与页面不一致，请从当前团队项目列表重新进入。
    </p>
    <template v-if="data">
      <p
        v-if="data.unavailable.length"
        role="status"
        class="rounded-lg bg-warning-soft p-4 text-sm text-warning"
      >
        部分项目当前没有贡献读取权限，未展示其结果。{{ data.unavailable.join('、') }}
      </p>
      <section v-if="!data.rows.length" class="card p-8">
        <h3 class="font-semibold">暂无可查看的贡献结果</h3>
        <p class="mt-2 text-sm text-ink-muted">
          正式核定后，获授权人员须为你准备结果告知。未准备告知不会被视为送达。
        </p>
      </section>
      <article
        v-for="row in data.rows"
        :key="row.result.notice.id"
        class="card min-w-0"
        data-testid="contribution-result"
      >
        <div class="card-header flex-wrap">
          <div>
            <p class="text-xs text-ink-subtle">{{ row.project.name }}</p>
            <h3 class="mt-1 font-semibold">{{ row.evidence.task_title }}</h3>
          </div>
          <span class="badge"
            >版本 {{ row.version.version }} · {{ versionStatusLabel(row.version.status) }}</span
          >
        </div>
        <div class="card-body space-y-5">
          <p class="text-sm text-ink-muted">
            {{
              row.detail.case.effective_confirmed_version_id === row.version.id
                ? '当前正式有效版本'
                : '历史核定版本，当前有效结果请查看案例'
            }}<span v-if="!row.detail.case.undisputed_confirmed">
              · 存在争议或当前不计入无争议正式贡献</span
            >
          </p>
          <div class="grid gap-5 sm:grid-cols-2">
            <div>
              <p class="text-xs text-ink-subtle">本人正式核定份额</p>
              <p class="mt-1 text-2xl font-semibold">
                {{ ownShare(row) ? formatBasisPoints(ownShare(row)!.share_bps) : '无本人份额' }}
              </p>
            </div>
            <div>
              <p class="text-xs text-ink-subtle">实际收件状态</p>
              <p class="mt-1 text-sm font-medium">
                {{
                  verifiedReceipt(row.result.notice, row.result.confirmation)
                    ? '已确认收到（不代表同意）'
                    : '尚无可信收件凭据'
                }}
              </p>
              <p
                v-if="verifiedReceipt(row.result.notice, row.result.confirmation)"
                class="mt-1 text-xs text-ink-muted"
              >
                {{ formatDateTime(row.result.notice.receipt?.acknowledged_at) }}
              </p>
            </div>
          </div>
          <div class="space-y-2 text-sm leading-6">
            <p><strong>核定摘要：</strong>{{ row.result.summary || '无摘要' }}</p>
            <p><strong>贡献说明：</strong>{{ ownShare(row)?.basis || '无说明' }}</p>
            <p><strong>贡献声明：</strong>{{ ownShare(row)?.statement || '无声明' }}</p>
            <p class="text-xs text-ink-subtle">说明与声明为人工整理，真实性须结合证据判断。</p>
            <p><strong>核定理由：</strong>{{ row.result.confirmation.rationale }}</p>
          </div>
          <dl class="grid gap-3 text-xs sm:grid-cols-2">
            <div>
              <dt class="text-ink-subtle">核定者</dt>
              <dd class="mt-1 break-all">
                {{ memberDisplayName(row.members, row.result.confirmation.confirmed_by) }}
              </dd>
            </div>
            <div>
              <dt class="text-ink-subtle">核定时间</dt>
              <dd class="mt-1">{{ formatDateTime(row.result.confirmation.confirmed_at) }}</dd>
            </div>
            <div>
              <dt class="text-ink-subtle">具体版本 ID</dt>
              <dd class="mt-1 break-all">{{ row.result.notice.case_version_id }}</dd>
            </div>
            <div>
              <dt class="text-ink-subtle">核定 ID</dt>
              <dd class="mt-1 break-all">{{ row.result.confirmation.id }}</dd>
            </div>
            <div class="sm:col-span-2">
              <dt class="text-ink-subtle">内容摘要 SHA-256</dt>
              <dd class="mt-1 break-all font-mono">{{ row.result.confirmation.version_digest }}</dd>
            </div>
          </dl>
          <form
            v-if="canReceive(row)"
            class="space-y-3 rounded-lg border border-line bg-surface-muted p-4"
            @submit.prevent="receive(row)"
          >
            <p class="text-sm font-medium">确认收到，仅表示你已收到此结果，不代表同意贡献分配。</p>
            <label class="flex items-start gap-2 text-sm"
              ><input
                v-model="read[row.result.notice.id]"
                type="checkbox"
                class="mt-1"
                :disabled="pending"
              />我已阅读此结果及收件说明</label
            >
            <button
              class="btn btn-primary"
              :disabled="pending || loading || !read[row.result.notice.id]"
            >
              {{ pending ? '正在确认…' : '确认已收到' }}
            </button>
          </form>
          <RouterLink
            class="inline-block text-sm text-accent-strong"
            :to="{
              name: 'contribution-case',
              params: { teamId, projectId: row.project.id, caseId: row.detail.case.id },
            }"
            >查看证据、争议与完整历史 →</RouterLink
          >
        </div>
      </article>
    </template>
    <RouterLink
      class="inline-block text-sm text-accent-strong"
      :to="{ name: 'projects', params: { teamId } }"
      >← 项目</RouterLink
    >
  </div>
</template>
