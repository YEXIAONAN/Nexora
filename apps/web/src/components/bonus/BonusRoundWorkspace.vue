<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import * as api from '@/api/bonus'
import { ApiError } from '@/api/client'
import AlertBanner from '@/components/ui/AlertBanner.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import { formatDateTime } from '@/utils/format'
import BonusPlanEditor from './BonusPlanEditor.vue'
import { bonusPlanStatusLabel, formatBonusAmount } from './bonusPresentation'

const props = defineProps<{ teamId: string; projectId: string; roundId: string }>()
const emit = defineEmits<{ changed: [] }>()
const auth = useAuthStore()
const ui = useUiStore()
const workspace = ref<api.BonusWorkspace | null>(null)
const projectWorkspace = ref<api.BonusProjectWorkspace | null>(null)
const revision = ref<number | null>(null)
const loading = ref(false),
  busy = ref(false),
  forbidden = ref(false),
  stale = ref(false)
const error = ref(''),
  workspaceError = ref(''),
  projectError = ref(''),
  success = ref('')
const editing = ref(false)
const action = ref<api.BonusAction | null>(null)
const actionReason = ref(''),
  acknowledged = ref(false)
const paymentUser = ref(''),
  reference = ref(''),
  paymentReason = ref(''),
  paymentAcknowledged = ref(false)
const grant = reactive({
  subject_id: '',
  role_code: 'editor' as api.BonusGrantRole,
  revoke: false,
  reason: '',
})
let controller: AbortController | null = null
let epoch = 0
let operation = 0

const actionInfo: Record<api.BonusAction, { label: string; explanation: string }> = {
  submit: {
    label: '提交独立核定',
    explanation:
      '提交当前已保存方案，进入待审状态。核定人不得是方案编辑者、受益人或贡献提案核定人。',
  },
  return: {
    label: '退回修改',
    explanation: '将待审方案退回草稿，负责人可按退回理由修改后重新提交。',
  },
  confirm: {
    label: '独立核定',
    explanation:
      '请核对所有来源、资格、金额与理由。核定后仍需单独发布；后端会检查方案编辑者、受益人及贡献提案核定人的利益冲突。',
  },
  publish: {
    label: '发布正式结果',
    explanation: '发布当前已核定方案，使成员可查看本人结果。发布不执行实际转账。',
  },
  revise: {
    label: '新建修订版本',
    explanation:
      '仅新增草稿版本，不覆盖原历史记录。新版本独立核定后替换正式版本，另行发布后成员才可查看新结果。',
  },
}
const roleLabels: Record<api.BonusGrantRole, string> = {
  editor: '方案编辑',
  budget: '预算金额',
  eligibility: '资格维护',
  confirmer: '独立核定',
  auditor: '审计查看',
}
const actions = computed(() =>
  (Object.keys(actionInfo) as api.BonusAction[]).filter(
    (key) => workspace.value?.permissions[key] === true,
  ),
)
const canGrant = computed(() => projectWorkspace.value?.can_create === true)
const members = computed(
  () => projectWorkspace.value?.members.filter((m) => m.user_id !== auth.user?.id) ?? [],
)
const plan = computed(() => workspace.value?.version ?? null)
const totals = computed(() => {
  const map = new Map<string, { user_id: string; display_name: string; amount: bigint }>()
  for (const source of plan.value?.sources ?? [])
    for (const line of source.lines) {
      const item = map.get(line.user_id) ?? {
        user_id: line.user_id,
        display_name: line.display_name,
        amount: 0n,
      }
      if (line.eligible) item.amount += BigInt(line.amount_minor)
      map.set(line.user_id, item)
    }
  return [...map.values()]
})
const recorded = computed(() => new Set(workspace.value?.payments.map((p) => p.user_id) ?? []))
const chosenPayment = computed(() => totals.value.find((m) => m.user_id === paymentUser.value))

type StepState = 'done' | 'current' | 'todo'
const steps = computed<{ label: string; hint: string; state: StepState }[]>(() => {
  const current = plan.value
  const published = Boolean(current?.published_at)
  const paid = (workspace.value?.payments.length ?? 0) > 0
  const status = current?.status
  const reviewDone = status === 'confirmed' || status === 'superseded'
  const submitted = status === 'pending_review' || reviewDone
  return [
    { label: '批次创建', hint: '独立资金来源', state: 'done' },
    {
      label: '方案编制',
      hint: '人工预算与资格',
      state: reviewDone || submitted ? 'done' : 'current',
    },
    {
      label: '提交核定',
      hint: '冻结提交版本',
      state: reviewDone ? 'done' : submitted ? 'current' : 'todo',
    },
    {
      label: '独立核定',
      hint: '利益冲突检查',
      state: reviewDone ? (published ? 'done' : 'current') : 'todo',
    },
    {
      label: '发布结果',
      hint: '成员可查看本人金额',
      state: published ? 'done' : 'todo',
    },
    {
      label: '模拟发放',
      hint: '记录非实际转账',
      state: paid ? 'done' : published ? 'current' : 'todo',
    },
  ]
})
function stepClass(state: StepState): string {
  if (state === 'done') return 'border-accent/30 bg-accent-soft text-accent-strong'
  if (state === 'current') return 'border-warning/30 bg-warning-soft text-warning'
  return 'border-line bg-surface text-ink-subtle'
}
function stepDotClass(state: StepState): string {
  if (state === 'done') return 'bg-accent text-accent-contrast'
  if (state === 'current') return 'bg-warning text-white'
  return 'bg-surface-muted text-ink-subtle'
}
function memberName(id: string) {
  return (
    projectWorkspace.value?.members.find((m) => m.user_id === id)?.display_name ??
    workspace.value?.grants.find((g) => g.subject_id === id)?.display_name ??
    totals.value.find((m) => m.user_id === id)?.display_name ??
    id
  )
}
function failure(e: unknown) {
  if (!(e instanceof ApiError)) return '操作失败，请刷新核对状态后重试。'
  const fields = Object.values(e.fields ?? {}).join('；')
  const hint =
    e.status === 409
      ? ' 数据已发生变化，请刷新后重新检查方案及权限再操作。'
      : e.status === 403
        ? ' 权限或独立性检查未通过；平台管理员不自动拥有奖金权限，核定人不能是方案编辑者、受益人或贡献提案核定人。'
        : !e.status
          ? ' 操作结果尚未确认，请刷新核对后再尝试，避免重复操作。'
          : ''
  return `${e.message}${fields ? `（${fields}）` : ''}${hint}`
}
function resetForms() {
  editing.value = false
  action.value = null
  actionReason.value = ''
  acknowledged.value = false
  paymentUser.value = ''
  reference.value = ''
  paymentReason.value = ''
  paymentAcknowledged.value = false
  Object.assign(grant, { subject_id: '', role_code: 'editor', revoke: false, reason: '' })
}
async function load() {
  const current = ++epoch
  controller?.abort()
  controller = new AbortController()
  const signal = controller.signal
  const { teamId, projectId, roundId } = props
  loading.value = true
  revision.value = null
  workspace.value = null
  projectWorkspace.value = null
  forbidden.value = false
  stale.value = false
  workspaceError.value = ''
  projectError.value = ''
  resetForms()
  const [metadata, roundWorkspace, project] = await Promise.allSettled([
    api.getBonusRound(teamId, projectId, roundId, signal),
    api.getBonusWorkspace(teamId, projectId, roundId, signal),
    api.getBonusProjectWorkspace(teamId, projectId, signal),
  ])
  if (current !== epoch) return
  if (metadata.status === 'fulfilled') revision.value = metadata.value.revision
  else {
    stale.value = true
    error.value = `无法刷新批次版本：${failure(metadata.reason)}`
  }
  if (roundWorkspace.status === 'fulfilled') {
    workspace.value = roundWorkspace.value
    revision.value = Math.max(revision.value ?? 0, roundWorkspace.value.round.revision)
  } else if (
    roundWorkspace.reason instanceof ApiError &&
    [403, 404].includes(roundWorkspace.reason.status ?? 0)
  )
    forbidden.value = true
  else workspaceError.value = failure(roundWorkspace.reason)
  if (project.status === 'fulfilled') projectWorkspace.value = project.value
  else if (!(project.reason instanceof ApiError && [403, 404].includes(project.reason.status ?? 0)))
    projectError.value = failure(project.reason)
  loading.value = false
}
async function refresh() {
  if (busy.value) return
  error.value = ''
  success.value = ''
  await load()
  emit('changed')
}
// All writes share the same revision, CSRF-enabled client, duplicate-submit guard and context epoch.
async function execute(
  reason: string,
  label: string,
  send: (command: api.BonusCommand, signal: AbortSignal) => Promise<api.BonusCommandResult>,
) {
  if (busy.value || loading.value || stale.value || revision.value === null) return
  if (!reason.trim()) {
    error.value = '请填写本次操作理由。'
    return
  }
  const current = epoch
  const operationID = ++operation
  const signal = controller!.signal
  busy.value = true
  error.value = ''
  success.value = ''
  try {
    const result = await send(
      {
        reason: reason.trim(),
        expected_revision: revision.value,
        client_request_id: crypto.randomUUID(),
      },
      signal,
    )
    if (current !== epoch) return
    revision.value = result.revision
    success.value = `${label}成功。`
    await load()
    if (operationID === operation) emit('changed')
  } catch (e) {
    if (current !== epoch) return
    error.value = failure(e)
    if (e instanceof ApiError && (e.status === 409 || !e.status)) stale.value = true
  } finally {
    if (operationID === operation) busy.value = false
  }
}
function save(input: api.BonusPlanInput, reason: string) {
  if (!workspace.value?.permissions.edit) return
  const { teamId, projectId, roundId } = props
  void execute(reason, '人工方案保存', (command, signal) =>
    api.saveBonusPlan(teamId, projectId, roundId, { ...input, ...command }, signal),
  )
}
function chooseAction(next: api.BonusAction) {
  action.value = next
  actionReason.value = ''
  acknowledged.value = false
  error.value = ''
}
function transition() {
  const selected = action.value
  if (!selected || !workspace.value?.permissions[selected]) return
  if (!acknowledged.value) {
    error.value = '请确认已了解此次操作的影响。'
    return
  }
  const { teamId, projectId, roundId } = props
  void execute(actionReason.value, actionInfo[selected].label, (command, signal) =>
    api.transitionBonusRound(teamId, projectId, roundId, selected, command, signal),
  )
}
async function changeGrant() {
  if (!canGrant.value) return
  if (!auth.user?.id || !members.value.some((m) => m.user_id === grant.subject_id)) {
    error.value = '请选择其他活跃项目成员，不可自我授权或撤销自己的授权。'
    return
  }
  if (grant.revoke) {
    const target = members.value.find((m) => m.user_id === grant.subject_id)
    const ok = await ui.confirm({
      title: '撤销奖金授权',
      message: `确认撤销 ${target?.display_name ?? '该成员'} 的「${roleLabels[grant.role_code]}」角色？撤销后其相关操作权限立即失效，历史记录保留。`,
      confirmLabel: '确认撤销授权',
      tone: 'danger',
    })
    if (!ok) return
  }
  const { teamId, projectId, roundId } = props
  const input = { subject_id: grant.subject_id, role_code: grant.role_code, revoke: grant.revoke }
  void execute(grant.reason, grant.revoke ? '撤销授权' : '新增授权', (command, signal) =>
    api.changeBonusGrant(teamId, projectId, roundId, { ...input, ...command }, signal),
  )
}
function choosePayment(userId: string) {
  paymentUser.value = userId
  reference.value = ''
  paymentReason.value = ''
  paymentAcknowledged.value = false
}
function pay() {
  if (
    !workspace.value?.permissions.record_payment ||
    !chosenPayment.value ||
    recorded.value.has(paymentUser.value)
  )
    return
  if (!reference.value.trim() || !paymentAcknowledged.value) {
    error.value = '请填写模拟发放参考号，并确认这不是实际转账。'
    return
  }
  const { teamId, projectId, roundId } = props
  const input = { user_id: paymentUser.value, reference: reference.value.trim() }
  void execute(paymentReason.value, '模拟发放记录', (command, signal) =>
    api.recordBonusPayment(teamId, projectId, roundId, { ...input, ...command }, signal),
  )
}
watch(
  () => [props.teamId, props.projectId, props.roundId, auth.user?.id],
  () => {
    operation++
    error.value = ''
    success.value = ''
    busy.value = false
    void load()
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  epoch++
  operation++
  controller?.abort()
})
</script>

<template>
  <section class="space-y-4" aria-label="奖金管理工作区">
    <div class="flex flex-wrap items-end justify-between gap-3">
      <div>
        <h3 class="card-title">奖金方案工作区</h3>
        <p class="card-description">
          六个步骤分开办理：批次创建 → 方案编制 → 提交核定 → 独立核定 → 发布 →
          模拟发放。金额仅向显式授权成员开放。
        </p>
      </div>
      <button class="btn btn-sm" :disabled="loading || busy" @click="refresh">刷新工作区</button>
    </div>

    <ol class="grid gap-2 sm:grid-cols-3 xl:grid-cols-6" aria-label="奖金流程步骤">
      <li
        v-for="(step, index) in steps"
        :key="step.label"
        class="flex items-center gap-2.5 rounded-control border px-3 py-2.5"
        :class="stepClass(step.state)"
      >
        <span
          class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-micro font-semibold"
          :class="stepDotClass(step.state)"
          >{{ index + 1 }}</span
        >
        <span class="min-w-0">
          <span class="block truncate text-meta font-semibold">{{ step.label }}</span>
          <span class="block truncate text-micro opacity-80">{{ step.hint }}</span>
        </span>
      </li>
    </ol>

    <AlertBanner v-if="success" tone="success" :message="success" />
    <AlertBanner v-if="error" tone="danger" :message="error" />
    <div v-if="loading" class="card space-y-2 p-4">
      <p class="sr-only" role="status">正在加载授权、方案和历史…</p>
      <div v-for="i in 3" :key="i" class="skeleton h-16" />
    </div>
    <template v-else>
      <AlertBanner
        v-if="forbidden"
        tone="info"
        title="你尚未获得此批次的金额访问授权"
        message="批次元数据与版本历史仍可查看，本人正式结果请前往「我的奖金」。创建者和平台管理员不会自动获得金额权限；如需编辑方案、核定或发放，请由项目负责人或团队管理员授予对应角色。"
      />
      <AlertBanner
        v-if="workspaceError"
        tone="danger"
        :message="`金额工作区加载失败：${workspaceError}`"
      />
      <AlertBanner
        v-if="projectError"
        tone="danger"
        :message="`成员与贡献目录加载失败：${projectError}`"
      />
      <AlertBanner
        v-if="stale"
        tone="warning"
        message="当前版本不可继续写入，请使用「刷新工作区」获取最新 revision 后重新操作。"
      />

      <template v-if="workspace">
        <section v-if="plan" class="card">
          <div class="card-header">
            <div>
              <h4 class="card-title">
                方案 v{{ plan.version }} · {{ bonusPlanStatusLabel(plan) }}
              </h4>
              <p class="card-description">
                {{
                  plan.published_at
                    ? `发布于 ${formatDateTime(plan.published_at)}`
                    : '尚未发布，不代表已发放'
                }}
              </p>
            </div>
            <button
              v-if="workspace.permissions.edit && !editing && !stale"
              class="btn btn-sm"
              :disabled="busy"
              @click="editing = true"
            >
              编辑人工方案
            </button>
          </div>
          <div class="grid gap-4 p-5 sm:grid-cols-2">
            <div>
              <p class="field-label">方案总预算</p>
              <p class="mt-1 text-xl font-semibold">
                {{ formatBonusAmount(plan.total_budget_minor) }}
              </p>
            </div>
            <div>
              <p class="field-label">全方案未分配余额（未分配至来源）</p>
              <p class="mt-1 text-xl font-semibold">
                {{ formatBonusAmount(plan.plan_unallocated_minor) }}
              </p>
            </div>
          </div>
          <p
            v-if="plan.rationale"
            class="border-t border-line px-5 py-3 text-caption text-ink-muted"
          >
            方案理由：{{ plan.rationale }}
          </p>
          <div class="divide-y divide-line border-t border-line">
            <details v-for="source in plan.sources" :key="source.confirmation_id" class="px-5 py-3">
              <summary class="cursor-pointer text-body font-medium">
                {{ source.task_title }}
                <span class="ml-2 font-normal text-ink-muted"
                  >预算 {{ formatBonusAmount(source.source_budget_minor) }} · 余额
                  {{ formatBonusAmount(source.source_unallocated_minor) }}</span
                >
              </summary>
              <p class="my-3 text-caption text-ink-muted">来源理由：{{ source.reason }}</p>
              <div class="table-wrap">
                <table class="data-table min-w-[520px]">
                  <thead>
                    <tr>
                      <th>成员 / 贡献参考</th>
                      <th>资格</th>
                      <th>人工金额</th>
                      <th>人工理由</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="line in source.lines" :key="line.user_id">
                      <td>{{ line.display_name }} · {{ (line.share_bps / 100).toFixed(2) }}%</td>
                      <td>{{ line.eligible ? '有资格' : '无资格' }}</td>
                      <td class="whitespace-nowrap">
                        {{ formatBonusAmount(line.amount_minor) }}
                      </td>
                      <td class="min-w-40">{{ line.reason }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </details>
            <p v-if="!plan.sources.length" class="p-5 text-caption text-ink-muted">
              暂无贡献来源，请由方案负责人添加已确认贡献并人工分配。
            </p>
          </div>
        </section>
        <section v-else class="card">
          <EmptyState
            icon="layers"
            title="尚未编制奖金方案"
            description="初始预算为 0。金额负责人可基于已正式核定的贡献编制人工方案，不会自动套用贡献比例。"
          >
            <template v-if="workspace.permissions.edit && !editing && !stale" #action>
              <button
                type="button"
                class="btn btn-primary btn-sm"
                :disabled="busy"
                @click="editing = true"
              >
                编制人工方案
              </button>
            </template>
          </EmptyState>
        </section>

        <BonusPlanEditor
          v-if="editing && workspace.permissions.edit && !stale"
          :plan="plan"
          :available-sources="projectWorkspace?.sources ?? []"
          :sources-available="!!projectWorkspace"
          :busy="busy"
          @save="save"
          @cancel="editing = false"
        />
        <section v-if="!editing && !stale" class="card">
          <div class="card-header">
            <div>
              <h4 class="card-title">独立核定与发布</h4>
              <p class="card-description">
                草稿 → 待审 → 已核定未发布 →
                已发布。核定人与方案编辑者、受益人、贡献提案核定人必须独立；后端拒绝时会显示具体原因。
              </p>
            </div>
          </div>
          <div class="card-body">
            <div v-if="actions.length" class="flex flex-wrap gap-2">
              <button
                v-for="item in actions"
                :key="item"
                class="btn btn-sm"
                :disabled="busy"
                @click="chooseAction(item)"
              >
                {{ actionInfo[item].label }}
              </button>
            </div>
            <p v-else class="text-caption text-ink-muted">
              当前状态下你没有核定或发布操作权限。请由相应显式授权成员处理。
            </p>
            <form v-if="action" class="mt-4 border-t border-line pt-4" @submit.prevent="transition">
              <fieldset :disabled="busy" class="space-y-3">
                <h5 class="text-body font-medium">{{ actionInfo[action].label }}</h5>
                <p class="text-caption leading-5 text-ink-muted">
                  {{ actionInfo[action].explanation }}
                </p>
                <label class="form-label"
                  >操作理由<textarea
                    v-model="actionReason"
                    class="form-textarea"
                    rows="2"
                    required
                  /></label
                ><label class="flex items-center gap-2 text-caption text-ink-muted"
                  ><input
                    v-model="acknowledged"
                    type="checkbox"
                    class="checkbox"
                    required
                  />我已核对当前方案，了解上述操作影响</label
                >
                <div class="flex gap-2">
                  <button type="submit" class="btn btn-primary">
                    {{ busy ? '处理中…' : `确认${actionInfo[action].label}` }}</button
                  ><button type="button" class="btn" @click="action = null">取消</button>
                </div>
              </fieldset>
            </form>
          </div>
        </section>

        <section class="card">
          <div class="card-header">
            <div>
              <h4 class="card-title">成员汇总与模拟发放</h4>
              <p class="card-description">
                按当前版本来源逐项整数累加。模拟发放记录非实际转账；发放金额由后端按已发布结果生成，不可手工指定。
              </p>
            </div>
          </div>
          <div v-if="totals.length" class="divide-y divide-line">
            <div v-for="member in totals" :key="member.user_id" class="list-row text-body">
              <div>
                {{ member.display_name }}
                <span class="ml-3 font-medium">{{ formatBonusAmount(member.amount) }}</span>
              </div>
              <span v-if="recorded.has(member.user_id)" class="badge"
                >已记录模拟发放 · 不可重复</span
              ><button
                v-else-if="
                  workspace.permissions.record_payment &&
                  plan?.published_at &&
                  member.amount > 0n &&
                  !stale
                "
                class="btn btn-sm"
                :disabled="busy"
                @click="choosePayment(member.user_id)"
              >
                记录 {{ member.display_name }} 的模拟发放</button
              ><span v-else class="text-caption text-ink-muted">{{
                member.amount === 0n ? '金额为零，无需发放' : '未记录模拟发放'
              }}</span>
            </div>
          </div>
          <EmptyState
            v-else
            icon="award"
            title="暂无成员金额"
            description="发布正式结果后，成员金额会按已核定方案显示。"
          />
          <form
            v-if="paymentUser && chosenPayment && !stale && workspace.permissions.record_payment"
            class="border-t border-line p-5"
            @submit.prevent="pay"
          >
            <fieldset :disabled="busy" class="space-y-3">
              <h5 class="text-body font-medium">
                模拟记录：{{ chosenPayment.display_name }} ·
                {{ formatBonusAmount(chosenPayment.amount) }}
              </h5>
              <label class="form-label"
                >模拟发放参考号<input v-model="reference" class="form-input" required /></label
              ><label class="form-label"
                >记录理由<textarea
                  v-model="paymentReason"
                  class="form-textarea"
                  rows="2"
                  required
                /></label
              ><label class="flex items-center gap-2 text-caption text-ink-muted"
                ><input
                  v-model="paymentAcknowledged"
                  type="checkbox"
                  class="checkbox"
                  required
                />我确认这仅是模拟发放记录，不执行实际转账</label
              >
              <div class="flex gap-2">
                <button class="btn btn-primary" type="submit">
                  {{ busy ? '记录中…' : '确认记录模拟发放' }}</button
                ><button class="btn" type="button" @click="paymentUser = ''">取消</button>
              </div>
            </fieldset>
          </form>
          <details class="border-t border-line p-5">
            <summary class="cursor-pointer text-body font-medium">
              模拟发放历史（{{ workspace.payments.length }}）
            </summary>
            <ul class="mt-3 divide-y divide-line">
              <li
                v-for="(payment, i) in workspace.payments"
                :key="i"
                class="space-y-1 py-3 text-caption"
              >
                <p class="text-ink">
                  {{ memberName(payment.user_id) }} ·
                  {{ formatBonusAmount(payment.amount_minor) }} ·
                  {{ formatDateTime(payment.recorded_at) }}
                </p>
                <p class="break-all text-ink-muted">参考号：{{ payment.reference }}</p>
              </li>
            </ul>
            <p v-if="!workspace.payments.length" class="mt-3 text-caption text-ink-muted">
              尚无模拟发放记录。
            </p>
          </details>
        </section>
      </template>

      <section v-if="canGrant || workspace" class="card">
        <div class="card-header">
          <div>
            <h4 class="card-title">显式授权</h4>
            <p class="card-description">
              每次选择一个角色，同一成员可分别授予多个角色。不自动继承 ROOT
              权限，不可自我授权。核定角色仍需通过独立性检查。
            </p>
          </div>
          <span class="badge">{{ workspace?.grants.length ?? 0 }} 条</span>
        </div>
        <ul v-if="workspace?.grants.length" class="divide-y divide-line">
          <li
            v-for="g in workspace.grants"
            :key="`${g.subject_id}-${g.role_code}`"
            class="list-row"
          >
            <span class="text-body text-ink">{{ g.display_name }}</span>
            <span class="badge">{{
              roleLabels[g.role_code as api.BonusGrantRole] ?? g.role_code
            }}</span>
          </li>
        </ul>
        <p v-else class="px-5 py-4 text-caption text-ink-muted">
          {{
            workspace
              ? '暂无显式授权记录。'
              : '当前无金额工作区读取权，不展示授权清单；项目管理者仍可指定成员新增或撤销角色。'
          }}
        </p>
        <form
          v-if="canGrant && !stale"
          class="border-t border-line p-5"
          @submit.prevent="changeGrant"
        >
          <fieldset :disabled="busy" class="space-y-4">
            <div class="grid gap-3 sm:grid-cols-3">
              <label class="form-label"
                >授权成员<select v-model="grant.subject_id" required class="form-select">
                  <option value="">请选择其他成员</option>
                  <option v-for="m in members" :key="m.user_id" :value="m.user_id">
                    {{ m.display_name }}
                  </option>
                </select></label
              ><label class="form-label"
                >角色<select v-model="grant.role_code" class="form-select">
                  <option v-for="(label, role) in roleLabels" :key="role" :value="role">
                    {{ label }}
                  </option>
                </select></label
              ><label class="form-label"
                >授权操作<select v-model="grant.revoke" class="form-select">
                  <option :value="false">新增授权</option>
                  <option :value="true">撤销授权</option>
                </select></label
              >
            </div>
            <label class="form-label"
              >授权变更理由<textarea
                v-model="grant.reason"
                required
                rows="2"
                class="form-textarea"
              /></label
            ><button class="btn" type="submit">
              {{ busy ? '处理中…' : grant.revoke ? '确认撤销授权' : '确认新增授权' }}
            </button>
          </fieldset>
        </form>
      </section>

      <section v-if="workspace" class="card">
        <div class="card-header">
          <div>
            <h4 class="card-title">操作历史</h4>
            <p class="card-description">所有奖金命令、金额调整与授权变更均保留理由与操作人。</p>
          </div>
          <span class="badge">{{ workspace.events.length }} 条</span>
        </div>
        <ol v-if="workspace.events.length" class="divide-y divide-line">
          <li v-for="(event, i) in workspace.events" :key="i" class="space-y-1 px-5 py-3">
            <div class="flex flex-wrap justify-between gap-2 text-caption">
              <span class="font-medium text-ink"
                >{{ event.event_type }} · {{ memberName(event.actor_id) }}</span
              ><time class="text-ink-subtle">{{ formatDateTime(event.created_at) }}</time>
            </div>
            <p class="whitespace-pre-wrap break-words text-caption text-ink-muted">
              {{ event.reason }}
            </p>
          </li>
        </ol>
        <p v-else class="p-5 text-caption text-ink-muted">暂无操作历史。</p>
      </section>
    </template>
  </section>
</template>
