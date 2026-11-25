<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import * as api from '@/api/contributions'
import { getProject, listProjectMembers } from '@/api/projects'
import { getTeam } from '@/api/teams'
import { useContributionResource } from '@/composables/useContributionResource'
import {
  caseStatusLabel,
  memberDisplayName,
} from '@/components/contributions/contributionPresentation'
import AlertBanner from '@/components/ui/AlertBanner.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import { useAuthStore } from '@/stores/auth'
import { useTeamsStore } from '@/stores/teams'
import { formatDateTime } from '@/utils/format'

const route = useRoute(),
  auth = useAuthStore(),
  teams = useTeamsStore()
const teamId = computed(() => String(route.params.teamId ?? ''))
watch([teamId, () => teams.teams], ([id]) => teams.selectTeam(id), {
  immediate: true,
  flush: 'sync',
})
const projectId = computed(() => String(route.params.projectId ?? ''))
const key = computed(() =>
  auth.user && (!teams.currentTeamId || teams.currentTeamId === teamId.value)
    ? `${teamId.value}/${projectId.value}/${auth.user.id}`
    : null,
)
const reason = ref(''),
  user = ref('')
const { data, loading, pending, error, reload, mutate } = useContributionResource(
  () => key.value,
  async (context, signal) => {
    const [team, project] = context.split('/')
    const [p, t, members, eligibility, appraisers, cases] = await Promise.all([
      getProject(team!, project!, signal),
      getTeam(team!, signal),
      listProjectMembers(team!, project!, signal),
      api.getContributionEligibility(team!, project!, signal),
      api.listContributionAppraisers(team!, project!, signal),
      api.listMyAppraisableContributions(team!, project!, signal),
    ])
    return { project: p, team: t, members, eligibility, appraisers, cases }
  },
)
watch(
  key,
  () => {
    reason.value = ''
    user.value = ''
  },
  { immediate: true, flush: 'sync' },
)
watch(data, () => {
  reason.value = ''
  user.value = ''
})
const active = computed(() => data.value?.project.status === 'active')
function mayManage(target: string, revoke = false) {
  const d = data.value
  if (!d || !active.value || (!revoke && target === auth.user?.id)) return false
  return (
    (d.project.can_manage && d.project.lead_id === auth.user?.id) ||
    (target === d.project.lead_id &&
      (['owner', 'admin'].includes(d.team.my_role ?? '') || d.eligibility.explicit_appraiser)) ||
    (revoke && target === auth.user?.id)
  )
}
const candidates = computed(
  () =>
    data.value?.members.filter(
      (m) =>
        m.active &&
        mayManage(m.user_id) &&
        !data.value?.appraisers.some(
          (a) => a.user_id === m.user_id && a.status === 'active' && a.qualified,
        ),
    ) ?? [],
)
function memberName(id: string) {
  return memberDisplayName(data.value?.members, id)
}
async function change(appraiser?: api.ContributionAppraiser) {
  const target = appraiser?.user_id ?? user.value
  if (!reason.value.trim() || !mayManage(target, !!appraiser) || pending.value) return
  const previous = data.value?.appraisers.find((a) => a.user_id === target)
  const input = {
    expected_version: previous?.version ?? 0,
    client_request_id: crypto.randomUUID(),
    reason: reason.value.trim(),
  }
  await mutate((context, signal) => {
    const [team, project] = context.split('/')
    return appraiser
      ? api.revokeContributionAppraiser(team!, project!, target, input, signal)
      : api.grantContributionAppraiser(team!, project!, { ...input, user_id: target }, signal)
  })
}
</script>

<template>
  <div class="page-stack">
    <PageHeader
      eyebrow="项目贡献治理"
      title="独立核定工作台"
      :description="`${data?.project.name || '项目'}：核定需要明确授权及独立资格。项目负责人、受益人、提案人和成果审核关系均须按实际案例回避。`"
      :crumbs="[
        { label: '项目', to: `/teams/${teamId}/projects` },
        { label: data?.project.name || '项目', to: `/teams/${teamId}/projects/${projectId}` },
        { label: '贡献核算', to: `/teams/${teamId}/projects/${projectId}/contributions` },
        { label: '独立核定工作台' },
      ]"
    >
      <template #actions>
        <button class="btn" :disabled="loading || pending" @click="reload()">刷新资格与待办</button>
      </template>
    </PageHeader>
    <AlertBanner v-if="error" tone="danger" :message="error" />
    <div v-if="loading" class="card space-y-2 p-4">
      <div v-for="i in 3" :key="i" class="skeleton h-20" />
    </div>
    <AlertBanner
      v-if="auth.user && !key"
      tone="warning"
      message="当前团队与页面不一致，请从当前团队项目列表重新进入。"
    />
    <template v-if="data">
      <section class="card" aria-label="本人核定资格">
        <div class="card-header">
          <h3 class="card-title">本人核定资格</h3>
          <span class="badge">授权版本 {{ data.eligibility.authorization_version }}</span>
        </div>
        <div class="card-body space-y-2">
          <p class="text-body">
            {{
              data.eligibility.explicit_appraiser
                ? '服务端已确认明确核定授权'
                : '未获得当前有效的明确核定授权'
            }}
          </p>
          <p class="text-caption leading-5 text-ink-muted">
            明确授权不等于可以核定所有案例。服务端逐案检查受益份额、提案、成果作者、审核人、直接利益冲突及成员状态；无独立核定人时继续等待。
          </p>
        </div>
      </section>
      <section class="card">
        <div class="card-header">
          <div>
            <h3 class="card-title">待核定案例</h3>
            <p class="card-description">存在利益冲突的案例不会列入此待办。</p>
          </div>
          <span class="badge">{{ data.cases.length }}</span>
        </div>
        <div class="card-body space-y-4">
          <EmptyState
            v-if="!data.cases.length"
            icon="inbox"
            title="暂无符合本人独立资格的待核定案例"
            description="案例提交待核定后，符合独立资格的核定人会在这里看到。"
          />
          <RouterLink
            v-for="item in data.cases"
            :key="item.id"
            class="block rounded-control border border-line p-4 transition-colors hover:bg-surface-hover"
            :to="{ name: 'contribution-case', params: { teamId, projectId, caseId: item.id } }"
            ><span class="badge">{{ caseStatusLabel(item.case_workflow_status) }}</span>
            <p class="mt-2 text-body break-all">案例 {{ item.id }}</p>
            <p class="mt-1 text-caption text-ink-muted">
              打开成果证据、分配对比与不可变核定确认 →
            </p></RouterLink
          >
        </div>
      </section>
      <section class="card" aria-label="核定人授权管理">
        <div class="card-header"><h3 class="card-title">核定人授权管理</h3></div>
        <fieldset :disabled="pending" class="card-body space-y-4">
          <p class="text-caption leading-5 text-ink-muted">
            负责人可以授权其他有效项目成员；负责人本人须由有资格的其他人员授权。所有变更均由服务端重新校验。
          </p>
          <label
            v-if="
              candidates.length ||
              data.appraisers.some((a) => a.status === 'active' && mayManage(a.user_id, true))
            "
            class="form-label"
            >核定授权原因<input v-model="reason" class="form-input" required maxlength="1000"
          /></label>
          <p v-if="!data.appraisers.length" class="text-sm text-ink-muted">尚无明确授权记录。</p>
          <ul class="divide-y divide-line">
            <li
              v-for="appraiser in data.appraisers"
              :key="appraiser.user_id"
              class="flex flex-wrap items-start gap-3 py-3"
            >
              <div class="min-w-0 grow">
                <p class="text-sm font-medium break-all">
                  {{ memberName(appraiser.user_id) }} ·
                  {{
                    appraiser.status === 'revoked'
                      ? '已撤销'
                      : appraiser.qualified
                        ? '有效授权'
                        : '成员资格已失效'
                  }}
                </p>
                <p class="mt-1 text-xs text-ink-muted">
                  授权版本 {{ appraiser.version }} · {{ formatDateTime(appraiser.granted_at) }}
                </p>
                <p class="mt-1 text-xs text-ink-muted">{{ appraiser.reason }}</p>
              </div>
              <button
                v-if="appraiser.status === 'active' && mayManage(appraiser.user_id, true)"
                class="btn text-danger"
                :disabled="!reason.trim()"
                @click="change(appraiser)"
              >
                撤销核定权
              </button>
            </li>
          </ul>
          <form
            v-if="candidates.length"
            class="grid gap-3 sm:grid-cols-[1fr_auto]"
            @submit.prevent="change()"
          >
            <label class="form-label"
              >选择核定人<select v-model="user" class="form-input" required>
                <option value="">请选择有效项目成员</option>
                <option v-for="member in candidates" :key="member.user_id" :value="member.user_id">
                  {{ member.display_name || member.username }}
                </option>
              </select></label
            ><button class="btn self-end" :disabled="!user || !reason.trim()">授予核定权</button>
          </form>
        </fieldset>
      </section>
    </template>
    <RouterLink
      class="inline-block text-sm text-accent-strong"
      :to="{ name: 'project-contributions', params: { teamId, projectId } }"
      >← 贡献核算</RouterLink
    >
  </div>
</template>
