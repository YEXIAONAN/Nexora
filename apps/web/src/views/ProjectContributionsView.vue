<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import * as api from '@/api/contributions'
import * as projects from '@/api/projects'
import * as tasks from '@/api/tasks'
import * as submissions from '@/api/submissions'
import { useContributionResource } from '@/composables/useContributionResource'
import { useAuthStore } from '@/stores/auth'
import { useTeamsStore } from '@/stores/teams'
const route = useRoute(),
  router = useRouter(),
  teams = useTeamsStore(),
  auth = useAuthStore()
const team = computed(() => String(route.params.teamId ?? '')),
  project = computed(() => String(route.params.projectId ?? ''))
watch([team, () => teams.teams], ([id]) => teams.selectTeam(id), { immediate: true, flush: 'sync' })
const context = computed(() =>
  auth.user && (!teams.currentTeamId || teams.currentTeamId === team.value)
    ? JSON.stringify([team.value, project.value, auth.user.id])
    : null,
)
const reason = ref(''),
  filter = ref('all')
const statusLabels: Record<api.ContributionCase['case_workflow_status'], string> = {
  draft: '草案',
  pending_review: '待核定',
  confirmed: '已确认',
  disputed: '存在争议',
  revising: '修订中',
  abandoned: '已放弃',
}
const resource = useContributionResource(
  () => context.value,
  async (scope, signal) => {
    const [t, p] = JSON.parse(scope) as [string, string]
    const [projectInfo, eligibility, cases, tree] = await Promise.all([
      projects.getProject(t, p, signal),
      api.getContributionEligibility(t, p, signal),
      api.listContributions(t, p, signal),
      tasks.getTaskTree(t, p),
    ])
    const accepted = tree.tasks.filter(
      (task) =>
        task.status === 'completed' &&
        task.current_submission_version_id &&
        !cases.some(
          (c) =>
            c.task_id === task.id && c.submission_version_id === task.current_submission_version_id,
        ),
    )
    const candidates = await Promise.all(
      accepted.map(async (task) => {
        const versions = await submissions.listVersions(t, p, task.id)
        const version = versions.find(
          (v) => v.id === task.current_submission_version_id && v.review?.decision === 'approve',
        )
        return version?.review ? { task, version, review: version.review } : null
      }),
    )
    return {
      projectInfo,
      eligibility,
      cases,
      tasks: tree.tasks,
      candidates: candidates.filter((v) => v !== null),
    }
  },
)
const { data, loading, pending, error } = resource
const visible = computed(
  () =>
    data.value?.cases.filter(
      (c) => filter.value === 'all' || c.case_workflow_status === filter.value,
    ) ?? [],
)
watch(
  context,
  () => {
    reason.value = ''
    filter.value = 'all'
  },
  { flush: 'sync' },
)
async function create(taskID: string, reviewID: string) {
  if (!data.value?.eligibility.can_edit_cases || !reason.value.trim()) return
  const original = context.value
  const body = {
    task_id: taskID,
    review_id: reviewID,
    client_request_id: crypto.randomUUID(),
    reason: reason.value,
  }
  const result = await resource.mutate((scope, signal) => {
    const [t, p] = JSON.parse(scope) as [string, string]
    return api.createContribution(t, p, body, signal)
  })
  if (result?.case_id && original === context.value)
    await router.push({
      name: 'contribution-case',
      params: { teamId: team.value, projectId: project.value, caseId: result.case_id },
    })
}
</script>
<template>
  <div class="mx-auto max-w-6xl space-y-5">
    <header class="space-y-3">
      <RouterLink :to="`/teams/${team}/projects/${project}`" class="text-sm text-ink-muted"
        >← 返回项目详情</RouterLink
      >
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 class="text-xl font-semibold">贡献核算</h1>
          <p class="mt-1 text-sm text-ink-muted">
            {{ data?.projectInfo.name || '项目贡献工作区' }}
          </p>
        </div>
        <nav class="flex flex-wrap gap-2" aria-label="贡献工作区导航">
          <RouterLink
            :to="{ name: 'contribution-appraisal', params: { teamId: team, projectId: project } }"
            class="btn"
            >独立核定工作台</RouterLink
          ><RouterLink
            :to="{ name: 'my-contribution-results', params: { teamId: team } }"
            class="btn"
            >我的贡献结果</RouterLink
          ><button class="btn" :disabled="loading || pending" @click="resource.reload()">
            刷新贡献数据
          </button>
        </nav>
      </div>
    </header>
    <p v-if="error" role="alert" class="rounded-lg bg-danger-soft p-3 text-sm text-danger">
      {{ error }}
    </p>
    <p v-if="loading" role="status" class="text-sm text-ink-muted">正在读取贡献工作区…</p>
    <p
      v-if="auth.user && context === null"
      role="status"
      class="rounded-lg border border-line p-4 text-sm text-ink-muted"
    >
      当前团队与页面不一致，请从当前团队项目列表重新进入。
    </p>
    <template v-if="data">
      <p class="rounded-lg border border-line bg-surface p-4 text-sm leading-6 text-ink-muted">
        最新工作版本用于整理与核定，正式有效版本用于表达已核定结果。草案不计作正式贡献；存在争议时不得视为无争议的正式结果。
      </p>
      <section class="card" aria-label="已验收待建案成果">
        <div class="card-header">
          <h2 class="font-semibold">已验收、尚未建立贡献案例</h2>
          <span class="badge">{{ data.candidates.length }}</span>
        </div>
        <div class="card-body space-y-4">
          <p v-if="!data.candidates.length" class="text-sm text-ink-muted">
            当前没有可见的待建案成果。
          </p>
          <label v-if="data.eligibility.can_edit_cases && data.candidates.length" class="form-label"
            >建案原因<input v-model="reason" maxlength="1000" class="form-input"
          /></label>
          <article
            v-for="candidate in data.candidates"
            :key="candidate.review.id"
            data-testid="contribution-candidate"
            class="space-y-2 rounded-lg border border-line p-4"
          >
            <h3 class="font-medium">
              {{ candidate.task.title }} · 成果 V{{ candidate.version.version }}
            </h3>
            <p class="whitespace-pre-wrap text-sm text-ink-muted">
              {{ candidate.version.summary }}
            </p>
            <p class="text-xs text-ink-muted">
              真实审核通过：{{ candidate.review.reviewer_name }} · {{ candidate.review.comment }}
            </p>
            <button
              v-if="data.eligibility.can_edit_cases"
              class="btn"
              :disabled="pending || !reason.trim()"
              @click="create(candidate.task.id, candidate.review.id)"
            >
              创建贡献案例
            </button>
          </article>
          <p v-if="!data.eligibility.can_edit_cases" class="text-xs text-ink-muted">
            当前账号没有整理案例权限。可见范围与操作资格由服务端决定。
          </p>
        </div>
      </section>
      <section class="card" aria-label="贡献案例列表">
        <div class="card-header flex-wrap">
          <h2 class="font-semibold">贡献案例</h2>
          <label class="form-label"
            >案例状态<select v-model="filter" class="form-input">
              <option value="all">全部状态</option>
              <option v-for="(label, status) in statusLabels" :key="status" :value="status">
                {{ label }}
              </option>
            </select></label
          >
        </div>
        <div class="card-body space-y-3">
          <p v-if="!visible.length" class="text-sm text-ink-muted">此状态下暂无可见贡献案例。</p>
          <article
            v-for="c in visible"
            :key="c.id"
            class="space-y-2 rounded-lg border border-line p-4"
          >
            <div class="flex flex-wrap items-center justify-between gap-3">
              <RouterLink
                :to="{
                  name: 'contribution-case',
                  params: { teamId: team, projectId: project, caseId: c.id },
                }"
                class="font-medium text-accent-strong"
                >{{ data.tasks.find((t) => t.id === c.task_id)?.title || c.task_id }}</RouterLink
              ><span class="badge">{{ statusLabels[c.case_workflow_status] }}</span>
            </div>
            <dl class="grid gap-2 text-xs text-ink-muted sm:grid-cols-2">
              <div>
                <dt>最新工作版本</dt>
                <dd class="mt-1 break-all">{{ c.latest_version_id }}</dd>
              </div>
              <div>
                <dt>正式有效版本</dt>
                <dd class="mt-1 break-all">
                  {{ c.effective_confirmed_version_id || '尚无正式有效版本' }}
                </dd>
              </div>
            </dl>
            <p v-if="c.case_workflow_status === 'disputed'" class="text-sm text-warning">
              存在未决争议，等待独立裁定。
            </p>
            <p v-if="c.case_workflow_status === 'revising'" class="text-sm text-ink-muted">
              修订尚未生效；请分别查看旧有效版本与最新工作版本。
            </p>
            <p v-if="c.effective_confirmed_version_id" class="text-xs text-ink-muted">
              {{ c.undisputed_confirmed ? '当前有无争议的有效核定' : '当前不得作为无争议正式结果' }}
            </p>
            <p class="text-xs text-ink-subtle">
              案例修订号 {{ c.revision }} · 历史确认与裁定在案例中完整保留
            </p>
          </article>
        </div>
      </section>
    </template>
  </div>
</template>
