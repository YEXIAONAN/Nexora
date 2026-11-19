<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import * as api from '@/api/contributions'
import { getProject, listProjectMembers } from '@/api/projects'
import { useAuthStore } from '@/stores/auth'
import { useTeamsStore } from '@/stores/teams'
import { useContributionResource } from '@/composables/useContributionResource'
import ContributionVersionPanel from '@/components/contributions/ContributionVersionPanel.vue'
import ContributionDraftEditor from '@/components/contributions/ContributionDraftEditor.vue'
import ContributionEvidencePanel from '@/components/contributions/ContributionEvidencePanel.vue'
import ContributionHistoryPanel from '@/components/contributions/ContributionHistoryPanel.vue'
import ContributionProxyDispute from '@/components/contributions/ContributionProxyDispute.vue'
import {
  caseStatusLabel,
  verifiedReceipt,
} from '@/components/contributions/contributionPresentation'
import { formatDateTime } from '@/utils/format'
const route = useRoute(),
  auth = useAuthStore(),
  teams = useTeamsStore()
const team = computed(() => String(route.params.teamId ?? '')),
  project = computed(() => String(route.params.projectId ?? '')),
  caseID = computed(() => String(route.params.caseId ?? ''))
watch([team, () => teams.teams], ([id]) => teams.selectTeam(id), { immediate: true, flush: 'sync' })
const context = computed(() =>
  auth.user && (!teams.currentTeamId || teams.currentTeamId === team.value)
    ? JSON.stringify([team.value, project.value, caseID.value, auth.user.id])
    : null,
)
const reason = ref(''),
  disputeReason = ref(''),
  resolutionReason = ref(''),
  immutableAcknowledged = ref(false),
  conflictUser = ref(''),
  conflictKind = ref(''),
  conflictDetails = ref('')
const resource = useContributionResource(
  () => context.value,
  async (scope, signal) => {
    const [t, p, c] = JSON.parse(scope) as [string, string, string]
    const [detail, history, evidence, members, eligibility, projectInfo] = await Promise.all([
      api.getContribution(t, p, c, signal),
      api.getContributionHistory(t, p, c, signal),
      api.getContributionEvidence(t, p, c, signal),
      listProjectMembers(t, p, signal),
      api.getContributionEligibility(t, p, signal),
      getProject(t, p, signal),
    ])
    return { detail, history, evidence, members, eligibility, projectInfo }
  },
)
const { data, loading, pending, error } = resource
const permissions = computed(() => data.value?.detail.permissions)
const openDisputes = computed(() => data.value?.history.disputes.filter((d) => !d.resolution) ?? [])
function clearForms() {
  reason.value = ''
  disputeReason.value = ''
  resolutionReason.value = ''
  immutableAcknowledged.value = false
  conflictUser.value = ''
  conflictKind.value = ''
  conflictDetails.value = ''
}
watch(context, clearForms, { flush: 'sync' })
watch(
  () => data.value?.detail.case.revision,
  () => {
    if (data.value) clearForms()
  },
)
function command(
  reasonText: string,
  versionID = data.value?.detail.latest.id,
): api.ContributionVersionCommand | null {
  const d = data.value?.detail
  return d && versionID
    ? {
        expected_version: d.case.revision,
        client_request_id: crypto.randomUUID(),
        reason: reasonText,
        case_version_id: versionID,
      }
    : null
}
function write<T>(fn: (t: string, p: string, c: string, signal: AbortSignal) => Promise<T>) {
  return resource.mutate((scope, signal) => {
    const [t, p, c] = JSON.parse(scope) as [string, string, string]
    return fn(t, p, c, signal)
  })
}
function versionAction(action: 'submit' | 'withdraw' | 'abandon' | 'confirm' | 'revise') {
  const d = data.value?.detail,
    body = command(reason.value)
  if (!d || !body || !reason.value.trim()) return
  const permitted = {
    submit: d.permissions.can_submit,
    withdraw: d.permissions.can_withdraw,
    abandon: d.permissions.can_edit,
    confirm: d.permissions.can_confirm && immutableAcknowledged.value,
    revise: d.permissions.can_revise,
  }
  if (!permitted[action]) return
  const calls = {
    submit: api.submitContribution,
    withdraw: api.withdrawContribution,
    abandon: api.abandonContribution,
    confirm: api.confirmContribution,
    revise: api.revisionsContribution,
  }
  void write((t, p, c, signal) => calls[action](t, p, c, body, signal))
}
function saveDraft(
  input: Pick<api.ContributionDraftRequest, 'summary' | 'evidence_snapshot' | 'reason'>,
) {
  const body = command(input.reason)
  if (body && permissions.value?.can_edit)
    void write((t, p, c, s) => api.saveContributionDraft(t, p, c, { ...body, ...input }, s))
}
function saveShare(
  user: string,
  input: Pick<
    api.ContributionShareRequest,
    'share_bps' | 'basis' | 'statement' | 'evidence_refs' | 'reason'
  >,
) {
  const body = command(input.reason)
  if (body && permissions.value?.can_edit)
    void write((t, p, c, s) => api.setContributionShare(t, p, c, user, { ...body, ...input }, s))
}
function removeShare(user: string, message: string) {
  const body = command(message)
  if (body && permissions.value?.can_edit)
    void write((t, p, c, s) => api.removeContributionShare(t, p, c, user, body, s))
}
function raiseDispute() {
  const effective = data.value?.detail.effective,
    body = command(disputeReason.value, effective?.id)
  if (
    body &&
    effective?.confirmation &&
    permissions.value?.can_dispute &&
    disputeReason.value.trim()
  ) {
    const input = { ...body, confirmation_id: effective.confirmation.id }
    void write((t, p, c, s) => api.raiseContributionDispute(t, p, c, input, s))
  }
}
function proxyDispute(body: api.ContributionDisputeRequest) {
  if (
    data.value?.eligibility.can_edit_cases &&
    permissions.value?.can_dispute &&
    body.expected_version === data.value.detail.case.revision
  )
    void write((t, p, c, s) => api.raiseContributionDispute(t, p, c, body, s))
}
function resolve(dispute: api.ContributionDispute, decision: 'uphold' | 'overturn') {
  const d = data.value?.detail
  if (!d || !dispute.can_resolve || dispute.resolution || !resolutionReason.value.trim()) return
  const body = {
    expected_version: d.case.revision,
    client_request_id: crypto.randomUUID(),
    reason: resolutionReason.value,
    decision,
  }
  void write((t, p, c, s) => api.resolveContributionDispute(t, p, c, dispute.id, body, s))
}
function prepareNotice(user: string) {
  const d = data.value?.detail,
    effective = d?.effective,
    body = command(reason.value, effective?.id)
  if (!d || !body || !effective?.confirmation || !d.permissions.can_notify || !reason.value.trim())
    return
  const input = { ...body, confirmation_id: effective.confirmation.id, recipient_id: user }
  void write((t, p, c, s) => api.prepareContributionNotice(t, p, c, input, s))
}
function recordConflict() {
  const d = data.value?.detail,
    user = data.value?.eligibility.can_edit_cases ? conflictUser.value : auth.user?.id
  if (
    !d ||
    !user ||
    !conflictKind.value.trim() ||
    !conflictDetails.value.trim() ||
    !reason.value.trim()
  )
    return
  const input = {
    expected_version: d.case.revision,
    client_request_id: crypto.randomUUID(),
    reason: reason.value,
    user_id: user,
    kind: conflictKind.value,
    details: conflictDetails.value,
  }
  void write((t, p, c, s) => api.recordContributionConflict(t, p, c, input, s))
}
const memberName = (id: string) =>
  data.value?.members.find((m) => m.user_id === id)?.display_name ||
  data.value?.members.find((m) => m.user_id === id)?.username ||
  id
</script>
<template>
  <div class="mx-auto max-w-6xl space-y-5">
    <header class="space-y-3">
      <RouterLink
        :to="{ name: 'project-contributions', params: { teamId: team, projectId: project } }"
        class="text-sm text-ink-muted"
        >← 返回贡献核算</RouterLink
      >
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 class="text-xl font-semibold">{{ data?.evidence.task_title || '贡献案例' }}</h1>
          <p v-if="data" class="mt-1 text-sm text-ink-muted">
            {{ caseStatusLabel(data.detail.case.case_workflow_status) }} · 案例修订号
            {{ data.detail.case.revision }}
          </p>
        </div>
        <button class="btn" :disabled="loading || pending" @click="resource.reload()">
          刷新案例
        </button>
      </div>
    </header>
    <p v-if="error" role="alert" class="rounded-lg bg-danger-soft p-3 text-sm text-danger">
      {{ error }}
    </p>
    <p v-if="loading" role="status" class="text-sm text-ink-muted">正在读取权威贡献数据…</p>
    <p v-if="pending" role="status" class="text-sm text-ink-muted">正在提交，请勿重复操作…</p>
    <p
      v-if="auth.user && context === null"
      role="status"
      class="rounded-lg border border-line p-4 text-sm text-ink-muted"
    >
      当前团队与页面不一致，请从当前团队项目列表重新进入。
    </p>
    <template v-if="data">
      <p
        v-if="data.detail.case.case_workflow_status === 'disputed'"
        class="rounded-lg bg-warning-soft p-4 text-sm text-warning"
      >
        存在未决争议，等待独立裁定。原核定事实仍保留，当前不得作为无争议正式结果。
      </p>
      <p
        v-if="data.detail.case.case_workflow_status === 'revising'"
        class="rounded-lg bg-surface-muted p-4 text-sm"
      >
        正在修订。最新草案尚未生效；旧有效版本与新工作版本分别展示。
      </p>
      <div class="grid items-start gap-5 lg:grid-cols-2">
        <section class="space-y-3" data-testid="contribution-latest">
          <h2 class="font-semibold">最新工作版本</h2>
          <ContributionVersionPanel :version="data.detail.latest" :members="data.members" />
        </section>
        <section class="space-y-3" data-testid="contribution-effective">
          <h2 class="font-semibold">正式有效版本</h2>
          <ContributionVersionPanel
            v-if="data.detail.effective"
            :version="data.detail.effective"
            :members="data.members"
            effective
          />
          <p v-else class="rounded-lg border border-line bg-surface p-4 text-sm text-ink-muted">
            尚无正式有效版本。草案及待核定版本均不是正式贡献。
          </p>
          <p v-if="data.detail.effective" class="text-xs text-ink-muted">
            {{
              data.detail.case.undisputed_confirmed
                ? '当前核定无未决争议。'
                : '当前结果存在争议或尚不具备无争议有效状态。'
            }}
          </p>
        </section>
      </div>
      <ContributionEvidencePanel :evidence="data.evidence" :team="team" :project="project" />
      <ContributionDraftEditor
        v-if="permissions?.can_edit"
        :version="data.detail.latest"
        :members="data.members"
        :busy="pending || loading"
        @save="saveDraft"
        @share="saveShare"
        @remove="removeShare"
      />
      <section class="card" aria-label="核定与工作流操作">
        <div class="card-header"><h2 class="font-semibold">独立核定与案例操作</h2></div>
        <fieldset class="card-body space-y-4" :disabled="pending || loading">
          <p class="text-sm leading-6 text-ink-muted">
            核定资格由服务端逐案提供。受益人、提案人、成果作者、任务审核人及已知利益冲突者必须回避；裁定另须回避原核定者和争议当事人。所有操作仍由服务端实时复核。
          </p>
          <p class="text-xs text-ink-muted">
            {{
              data.eligibility.explicit_appraiser
                ? '当前具有明确核定授权；具体案例仍需独立资格。'
                : '当前没有明确核定授权。'
            }}
          </p>
          <p
            v-if="data.detail.latest.status === 'pending_review' && !permissions?.can_confirm"
            class="rounded-lg bg-warning-soft p-3 text-sm text-warning"
          >
            当前账号不可核定此版本。请核对明确授权、当前成员资格、提案/受益/审核关系及已记录利益冲突；无人独立合格时继续等待。
          </p>
          <label
            v-if="
              permissions?.can_edit ||
              permissions?.can_withdraw ||
              permissions?.can_submit ||
              permissions?.can_confirm ||
              permissions?.can_revise ||
              permissions?.can_notify ||
              (auth.user && data.projectInfo.status === 'active')
            "
            class="form-label"
            >操作原因<input v-model="reason" maxlength="1000" class="form-input"
          /></label>
          <div v-if="permissions?.can_confirm" class="space-y-3 rounded-lg border border-line p-4">
            <p class="text-sm">
              即将正式核定 V{{
                data.detail.latest.version
              }}，将形成不可变核定历史。确认前请检查成果证据、所有份额及与旧有效版本的差异。
            </p>
            <label class="flex items-start gap-2 text-sm"
              ><input
                v-model="immutableAcknowledged"
                type="checkbox"
                class="mt-1"
              />我已理解：正式核定将形成不可变核定历史</label
            ><button
              class="btn btn-primary"
              :disabled="!immutableAcknowledged || !reason.trim()"
              @click="versionAction('confirm')"
            >
              正式核定
            </button>
          </div>
          <div class="flex flex-wrap gap-2">
            <button
              v-if="permissions?.can_submit"
              class="btn btn-primary"
              :disabled="!reason.trim()"
              @click="versionAction('submit')"
            >
              提交待核定</button
            ><button
              v-if="permissions?.can_withdraw"
              class="btn"
              :disabled="!reason.trim()"
              @click="versionAction('withdraw')"
            >
              撤回待核定版本</button
            ><button
              v-if="permissions?.can_revise"
              class="btn"
              :disabled="!reason.trim()"
              @click="versionAction('revise')"
            >
              创建修订草案</button
            ><button
              v-if="permissions?.can_edit"
              class="btn text-danger"
              :disabled="!reason.trim()"
              @click="versionAction('abandon')"
            >
              放弃此草案
            </button>
          </div>
          <p
            v-if="permissions?.can_edit && !permissions?.can_submit"
            class="text-xs text-ink-muted"
          >
            当前草案尚不满足提交条件；需完整填写人工份额、依据及证据并恰好合计 10000
            基点。放弃不能替代正式核算或绕过归档约束。
          </p>
        </fieldset>
      </section>
      <section
        v-if="data.detail.effective && permissions?.can_notify"
        class="card"
        aria-label="结果告知与实际收件"
      >
        <div class="card-header"><h2 class="font-semibold">结果告知与实际收件</h2></div>
        <div class="card-body space-y-3">
          <p class="text-xs leading-5 text-ink-muted">
            准备告知只使在队贡献者能够在“我的贡献结果”查看；不代表已送达。必须由本人展示结果后显式确认，缺少可信收件证据时归档继续受阻。先填写上方操作原因。
          </p>
          <div
            v-for="share in data.detail.effective.shares"
            :key="share.user_id"
            class="space-y-2 rounded-lg border border-line p-3 text-sm"
            :data-testid="`prepare-notice-${share.user_id}`"
          >
            <p>
              {{ memberName(share.user_id) }}
              <span v-if="share.membership_state === 'historical'" class="badge">历史成员</span>
            </p>
            <ul class="space-y-1 text-xs text-ink-muted">
              <li
                v-for="notice in data.history.notices.filter(
                  (n) =>
                    n.recipient_id === share.user_id &&
                    n.case_version_id === data!.detail.effective!.id,
                )"
                :key="notice.id"
              >
                {{
                  verifiedReceipt(notice, data!.detail.effective!.confirmation!)
                    ? `已有本人收件凭据 · ${formatDateTime(notice.receipt!.acknowledged_at)}`
                    : '仅已准备告知 · 尚无真实收件证据'
                }}
              </li>
            </ul>
            <p v-if="share.membership_state === 'historical'" class="text-xs text-warning">
              未开放离队者普通结果读取或代收件渠道；缺少真实收件证据时继续受阻。
            </p>
            <button
              v-else-if="
                !data.history.notices.some(
                  (n) =>
                    n.recipient_id === share.user_id &&
                    n.case_version_id === data!.detail.effective!.id &&
                    n.channel === 'authenticated_ack',
                )
              "
              class="btn"
              :disabled="pending || loading || !reason.trim()"
              @click="prepareNotice(share.user_id)"
            >
              准备收件告知
            </button>
          </div>
        </div>
      </section>
      <section class="card" aria-label="争议与独立裁定">
        <div class="card-header">
          <h2 class="font-semibold">争议与独立裁定</h2>
          <span class="badge">{{ openDisputes.length }} 项未决</span>
        </div>
        <fieldset class="card-body space-y-4" :disabled="pending || loading">
          <form
            v-if="permissions?.can_dispute && data.detail.effective?.confirmation"
            class="space-y-3"
            @submit.prevent="raiseDispute"
          >
            <p class="text-xs text-ink-muted">
              对正式有效版本 V{{ data.detail.effective.version }}
              发起本人异议；确认收到不代表同意分配。
            </p>
            <label class="form-label"
              >异议原因<textarea
                v-model="disputeReason"
                required
                maxlength="1000"
                rows="3"
                class="form-input"
              /></label
            ><button type="submit" class="btn" :disabled="!disputeReason.trim()">发起异议</button>
          </form>
          <ContributionProxyDispute
            v-if="data.eligibility.can_edit_cases && permissions?.can_dispute"
            :team="team"
            :project="project"
            :detail="data.detail"
            :busy="pending || loading"
            @submit="proxyDispute"
          />
          <p v-if="!data.history.disputes.length" class="text-sm text-ink-muted">暂无争议记录。</p>
          <article
            v-for="dispute in data.history.disputes"
            :key="dispute.id"
            class="space-y-2 rounded-lg border border-line p-4"
          >
            <p class="text-sm font-medium">
              {{ dispute.resolution ? '已裁定' : '等待独立裁定' }} ·
              {{ memberName(dispute.raised_by) }} · {{ formatDateTime(dispute.created_at) }}
            </p>
            <p class="whitespace-pre-wrap text-sm">{{ dispute.reason }}</p>
            <p class="break-all text-xs text-ink-muted">
              绑定版本 {{ dispute.case_version_id }} · 核定 {{ dispute.confirmation_id }}
            </p>
            <p v-if="dispute.on_behalf_of" class="text-xs text-ink-muted">
              经验证代提：{{ memberName(dispute.on_behalf_of) }} ·
              {{ dispute.verification_method }} · {{ dispute.verification_note }}
            </p>
            <div
              v-if="dispute.resolution"
              class="space-y-1 rounded-lg bg-surface-muted p-3 text-sm"
            >
              <p>
                {{
                  dispute.resolution.decision === 'uphold'
                    ? '维持原核定'
                    : '推翻原核定；原记录保留，进入后续草案'
                }}
                · {{ memberName(dispute.resolution.resolved_by) }}
              </p>
              <p class="whitespace-pre-wrap">{{ dispute.resolution.reason }}</p>
              <p class="text-xs text-ink-muted">
                {{ formatDateTime(dispute.resolution.resolved_at) }}
              </p>
            </div>
            <div v-else-if="dispute.can_resolve" class="space-y-3">
              <label class="form-label"
                >裁定原因<textarea
                  v-model="resolutionReason"
                  required
                  maxlength="1000"
                  rows="3"
                  class="form-input"
                />
              </label>
              <p class="text-xs text-ink-muted">
                裁定形成不可变历史；推翻将撤销当前有效结果并保留原核定。
              </p>
              <div class="flex flex-wrap gap-2">
                <button
                  class="btn"
                  :disabled="!resolutionReason.trim()"
                  @click="resolve(dispute, 'uphold')"
                >
                  维持原核定</button
                ><button
                  class="btn text-danger"
                  :disabled="!resolutionReason.trim()"
                  @click="resolve(dispute, 'overturn')"
                >
                  推翻原核定
                </button>
              </div>
            </div>
          </article>
        </fieldset>
      </section>
      <section class="card" aria-label="直接利益冲突记录">
        <div class="card-header"><h2 class="font-semibold">利益冲突记录</h2></div>
        <fieldset :disabled="pending || loading" class="card-body space-y-3">
          <p v-if="!data.history.conflicts.length" class="text-sm text-ink-muted">
            暂无已记录直接利益冲突；这不代表不存在未申报冲突。
          </p>
          <ul class="space-y-2 text-sm">
            <li v-for="conflict in data.history.conflicts" :key="conflict.id">
              {{ memberName(conflict.user_id) }} · {{ conflict.kind }} · {{ conflict.details }}
            </li>
          </ul>
          <details v-if="auth.user && data.projectInfo.status === 'active'">
            <summary class="cursor-pointer text-sm font-medium">补充直接利益冲突记录</summary>
            <form class="mt-3 space-y-3" @submit.prevent="recordConflict">
              <label v-if="data.eligibility.can_edit_cases" class="form-label"
                >冲突人员<select v-model="conflictUser" required class="form-input">
                  <option value="">请选择</option>
                  <option v-for="m in data.members" :key="m.user_id" :value="m.user_id">
                    {{ m.display_name || m.username }}
                  </option>
                </select></label
              >
              <p v-else class="text-xs text-ink-muted">仅记录本人冲突。</p>
              <label class="form-label"
                >冲突类型<input
                  v-model="conflictKind"
                  required
                  maxlength="100"
                  class="form-input" /></label
              ><label class="form-label"
                >冲突详情<textarea
                  v-model="conflictDetails"
                  required
                  maxlength="1000"
                  class="form-input"
                />
              </label>
              <p class="text-xs text-ink-muted">
                原因使用上方“操作原因”。记录不可删除，服务端会重新检查独立资格。
              </p>
              <button
                type="submit"
                class="btn"
                :disabled="
                  !reason.trim() ||
                  !conflictKind.trim() ||
                  !conflictDetails.trim() ||
                  (data.eligibility.can_edit_cases && !conflictUser)
                "
              >
                记录利益冲突
              </button>
            </form>
          </details>
        </fieldset>
      </section>
      <ContributionHistoryPanel :history="data.history" :members="data.members" />
    </template>
  </div>
</template>
