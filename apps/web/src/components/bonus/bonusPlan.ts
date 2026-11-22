import type { BonusPlanInput, BonusPlanSource, BonusSource } from '@/api/bonus'
import { bonusAmountInput, parseBonusAmount } from './bonusPresentation'

export interface DraftSource {
  confirmation_id: string
  task_title: string
  budget: string
  reason: string
  lines: {
    user_id: string
    display_name: string
    share_bps: number
    eligible: boolean
    amount: string
    reason: string
  }[]
}
export function draftFromPlan(source: BonusPlanSource): DraftSource {
  return {
    confirmation_id: source.confirmation_id,
    task_title: source.task_title,
    budget: bonusAmountInput(source.source_budget_minor),
    reason: source.reason,
    lines: source.lines.map((line) => ({ ...line, amount: bonusAmountInput(line.amount_minor) })),
  }
}
export function draftFromSource(source: BonusSource): DraftSource {
  return {
    confirmation_id: source.confirmation_id,
    task_title: source.task_title,
    budget: '0.00',
    reason: '',
    // Confirmed contribution percentages are context only, never an allocation formula.
    lines: source.shares.map((share) => ({
      ...share,
      eligible: false,
      amount: '0.00',
      reason: '',
    })),
  }
}
export function sumDraftLines(source: DraftSource): bigint | null {
  let total = 0n
  for (const line of source.lines) {
    const amount = line.eligible ? parseBonusAmount(line.amount) : 0
    if (amount === null) return null
    total += BigInt(amount)
  }
  return total
}
export function buildBonusPlan(total: string, sources: DraftSource[]): BonusPlanInput {
  const totalMinor = parseBonusAmount(total)
  if (totalMinor === null)
    throw new Error('总预算须为非负人民币金额，最多两位小数且不超过 9,000,000,000,000 元。')
  let budgets = 0n
  const inputs = sources.map((source) => {
    const budget = parseBonusAmount(source.budget)
    if (budget === null) throw new Error(`${source.task_title}：来源预算格式无效或超出金额上限。`)
    budgets += BigInt(budget)
    let allocated = 0n
    const lines = source.lines.map((line) => {
      const amount = line.eligible ? parseBonusAmount(line.amount) : 0
      if (amount === null)
        throw new Error(`${source.task_title} · ${line.display_name}：金额格式无效或超出金额上限。`)
      allocated += BigInt(amount)
      if (!line.reason.trim())
        throw new Error(
          `${source.task_title} · ${line.display_name}：请填写资格与金额的人工理由（无资格也需填写）。`,
        )
      return {
        user_id: line.user_id,
        eligible: line.eligible,
        amount_minor: amount,
        reason: line.reason.trim(),
      }
    })
    if (allocated > BigInt(budget))
      throw new Error(`${source.task_title}：成员金额超出来源预算，请减少金额或调整预算。`)
    if (!source.reason.trim()) throw new Error(`${source.task_title}：请填写来源预算理由。`)
    return {
      confirmation_id: source.confirmation_id,
      source_budget_minor: budget,
      reason: source.reason.trim(),
      lines,
    }
  })
  if (budgets > BigInt(totalMinor)) throw new Error('各来源预算合计超出总预算，请调整后再保存。')
  return { total_budget_minor: totalMinor, sources: inputs }
}
