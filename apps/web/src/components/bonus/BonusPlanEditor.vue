<script setup lang="ts">
import { computed, ref } from 'vue'
import type { BonusPlan, BonusPlanInput, BonusSource } from '@/api/bonus'
import { bonusAmountInput, formatBonusAmount, parseBonusAmount } from './bonusPresentation'
import {
  buildBonusPlan,
  draftFromPlan,
  draftFromSource,
  sumDraftLines,
  type DraftSource,
} from './bonusPlan'

const props = defineProps<{
  plan: BonusPlan | null
  availableSources: BonusSource[]
  sourcesAvailable: boolean
  busy: boolean
}>()
const emit = defineEmits<{ save: [input: BonusPlanInput, reason: string]; cancel: [] }>()
const total = ref(bonusAmountInput(props.plan?.total_budget_minor ?? 0))
const sources = ref<DraftSource[]>((props.plan?.sources ?? []).map(draftFromPlan))
const reason = ref('')
const selectedSource = ref('')
const error = ref('')
const available = computed(() =>
  props.availableSources.filter(
    (s) => !sources.value.some((d) => d.confirmation_id === s.confirmation_id),
  ),
)
const totalMinor = computed(() => parseBonusAmount(total.value))
const budgets = computed(() => {
  let sum = 0n
  for (const source of sources.value) {
    const budget = parseBonusAmount(source.budget)
    if (budget === null) return null
    sum += BigInt(budget)
  }
  return sum
})
const planBalance = computed(() =>
  totalMinor.value === null || budgets.value === null
    ? null
    : BigInt(totalMinor.value) - budgets.value,
)
const budgetExceeded = computed(() => planBalance.value !== null && planBalance.value < 0n)
function sourceBalance(source: DraftSource): bigint | null {
  const budget = parseBonusAmount(source.budget),
    sum = sumDraftLines(source)
  return budget === null || sum === null ? null : BigInt(budget) - sum
}
function addSource() {
  const source = available.value.find((s) => s.confirmation_id === selectedSource.value)
  if (!source) {
    error.value = '请先选择已确认贡献来源。'
    return
  }
  sources.value.push(draftFromSource(source))
  selectedSource.value = ''
  error.value = ''
}
function submit() {
  if (props.busy) return
  error.value = ''
  try {
    const input = buildBonusPlan(total.value, sources.value)
    if (!reason.value.trim()) throw new Error('请填写本次方案保存理由。')
    emit('save', input, reason.value.trim())
  } catch (e) {
    error.value = e instanceof Error ? e.message : '请检查方案内容'
  }
}
</script>

<template>
  <form class="card" @submit.prevent="submit">
    <div class="card-header">
      <div>
        <h3 class="card-title">人工方案编辑与预览</h3>
        <p class="card-description">
          所有输入单位为人民币元，最多两位小数。贡献比例仅供参考，不自动生成金额。保存不会提交核定。
        </p>
      </div>
    </div>
    <fieldset :disabled="busy" class="space-y-5 p-5">
      <div class="grid gap-4 sm:grid-cols-3">
        <label class="form-label"
          >方案总预算（元）<input v-model="total" class="form-input" inputmode="decimal" required
        /></label>
        <div>
          <p class="field-label">来源预算合计</p>
          <p class="field-value">
            {{ budgets === null ? '待填写有效金额' : formatBonusAmount(budgets) }}
          </p>
        </div>
        <div>
          <p class="field-label">全方案未分配余额（未分配至来源）</p>
          <p class="field-value" :class="{ 'text-danger': budgetExceeded }">
            {{ planBalance === null ? '待填写有效金额' : formatBonusAmount(planBalance) }}
          </p>
        </div>
      </div>
      <p v-if="budgetExceeded" role="alert" class="text-sm text-danger">
        各来源预算合计超出总预算，不能保存。
      </p>
      <div v-if="sourcesAvailable" class="flex flex-wrap items-end gap-2">
        <label class="form-label min-w-0 flex-1"
          >添加已确认贡献来源<select v-model="selectedSource" class="form-select">
            <option value="">请选择来源</option>
            <option
              v-for="source in available"
              :key="source.confirmation_id"
              :value="source.confirmation_id"
            >
              {{ source.task_title }} · {{ source.confirmation_id.slice(0, 8) }}
            </option>
          </select></label
        >
        <button type="button" class="btn" @click="addSource">添加来源</button>
        <p v-if="!available.length" class="w-full text-caption text-ink-muted">
          没有其他可添加的已确认贡献来源。
        </p>
      </div>
      <p v-else class="text-caption text-ink-muted">
        贡献来源目录不可用，仍可编辑已有来源。请刷新或联系项目管理者检查目录权限。
      </p>
      <p
        v-if="sourcesAvailable && availableSources.length === 0 && sources.length === 0"
        role="status"
        class="rounded-control border border-warning/25 bg-warning-soft p-4 text-caption leading-5 text-warning"
      >
        当前项目还没有已正式核定的贡献。可以先保存总预算，但在提交独立核定前必须至少添加一个已确认贡献来源；请先在「贡献核算」中完成正式核定。
      </p>
      <p
        v-if="!sources.length"
        class="rounded-control bg-surface-muted p-4 text-caption text-ink-muted"
      >
        尚未选择贡献来源。可先保存总预算，再逐项制定人工方案。
      </p>
      <section
        v-for="(source, index) in sources"
        :key="source.confirmation_id"
        class="space-y-4 rounded-lg border border-line p-4"
      >
        <div class="flex items-center justify-between gap-3">
          <h4 class="text-sm font-semibold">{{ source.task_title }}</h4>
          <button class="btn btn-ghost btn-sm" type="button" @click="sources.splice(index, 1)">
            移除来源
          </button>
        </div>
        <div class="grid gap-4 sm:grid-cols-2">
          <label class="form-label"
            >来源预算（元）<input
              v-model="source.budget"
              class="form-input"
              inputmode="decimal"
              required
          /></label>
          <div>
            <p class="field-label">来源未分配余额</p>
            <p class="field-value">
              {{
                sourceBalance(source) === null
                  ? '待填写有效金额'
                  : formatBonusAmount(sourceBalance(source)!)
              }}
            </p>
          </div>
        </div>
        <p
          v-if="sourceBalance(source) !== null && sourceBalance(source)! < 0n"
          role="alert"
          class="text-caption text-danger"
        >
          成员金额超出来源预算，不能保存。
        </p>
        <label class="form-label"
          >来源预算理由<input v-model="source.reason" class="form-input" required
        /></label>
        <div
          v-for="line in source.lines"
          :key="line.user_id"
          class="grid gap-3 border-t border-line pt-3 sm:grid-cols-2 lg:grid-cols-4"
        >
          <div>
            <p class="text-body font-medium">{{ line.display_name }}</p>
            <p class="mt-1 text-caption text-ink-muted">
              已确认贡献 {{ (line.share_bps / 100).toFixed(2) }}%
            </p>
            <label class="mt-2 flex items-center gap-2 text-caption"
              ><input
                v-model="line.eligible"
                type="checkbox"
                class="checkbox"
                @change="line.amount = '0.00'"
              />具备奖金资格</label
            >
          </div>
          <label v-if="line.eligible" class="form-label"
            >人工金额（元）<input
              v-model="line.amount"
              inputmode="decimal"
              class="form-input"
              required
          /></label>
          <p v-else class="text-caption leading-6 text-ink-muted">无资格 · 金额固定 ¥0.00</p>
          <label class="form-label sm:col-span-2"
            >资格与金额理由<input
              v-model="line.reason"
              required
              class="form-input"
              placeholder="包括无资格或金额为零的原因"
          /></label>
        </div>
      </section>
      <label class="form-label"
        >本次保存理由<textarea v-model="reason" required rows="2" class="form-textarea" />
      </label>
      <p
        v-if="error"
        role="alert"
        class="rounded-control bg-danger-soft p-3 text-caption text-danger"
      >
        {{ error }}
      </p>
      <div class="flex gap-2">
        <button class="btn btn-primary" type="submit">
          {{ busy ? '保存中…' : '保存人工方案' }}</button
        ><button class="btn" type="button" @click="emit('cancel')">取消编辑</button>
      </div>
    </fieldset>
  </form>
</template>
