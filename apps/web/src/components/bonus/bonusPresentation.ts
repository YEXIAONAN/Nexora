import type { BonusRoundStatus, BonusVersionStatus } from '@/api/bonus'

const roundStatusLabels: Record<BonusRoundStatus, string> = {
  draft: '方案起草中',
  pending_review: '待核定',
  confirmed: '已核定',
  revising: '修订中',
  abandoned: '已放弃',
  unavailable: '不可用',
}

const versionStatusLabels: Record<BonusVersionStatus, string> = {
  draft: '草稿',
  pending_review: '待核定',
  confirmed: '已核定',
  superseded: '已被替代',
  overturned: '已推翻',
  abandoned: '已放弃',
}

export function bonusRoundStatusLabel(status: BonusRoundStatus): string {
  return roundStatusLabels[status] ?? status
}

export function bonusVersionStatusLabel(status: BonusVersionStatus): string {
  return versionStatusLabels[status] ?? status
}

export function bonusRoundStatusTone(
  status: BonusRoundStatus,
): 'neutral' | 'success' | 'danger' | 'warning' {
  switch (status) {
    case 'confirmed':
      return 'success'
    case 'pending_review':
    case 'revising':
      return 'warning'
    case 'abandoned':
      return 'danger'
    default:
      return 'neutral'
  }
}

/** Use integer arithmetic throughout; even aggregate previews can exceed Number's safe range. */
export function formatBonusAmount(minor: number | bigint): string {
  if (typeof minor === 'number' && !Number.isSafeInteger(minor)) return '—'
  const value = BigInt(minor)
  const abs = value < 0n ? -value : value
  return `${value < 0n ? '-' : ''}¥${(abs / 100n).toLocaleString('zh-CN')}.${String(abs % 100n).padStart(2, '0')}`
}

export const MAX_BONUS_MINOR = 900000000000000n

/** Yuan text to fen; never use floating point multiplication or accept scientific notation. */
export function parseBonusAmount(text: string): number | null {
  const match = /^(\d+)(?:\.(\d{1,2}))?$/.exec(text.trim())
  if (!match || match[1]!.length > 16) return null
  const minor = BigInt(match[1]!) * 100n + BigInt((match[2] ?? '').padEnd(2, '0'))
  return minor <= MAX_BONUS_MINOR ? Number(minor) : null
}

export function bonusAmountInput(minor: number): string {
  const value = BigInt(minor)
  return `${value / 100n}.${String(value % 100n).padStart(2, '0')}`
}

export function bonusPlanStatusLabel(plan: {
  status: string
  published_at: string | null
}): string {
  if (plan.published_at) return '已发布'
  if (plan.status === 'confirmed') return '已核定未发布'
  if (plan.status === 'pending_review') return '待审'
  return versionStatusLabels[plan.status as BonusVersionStatus] ?? plan.status
}
