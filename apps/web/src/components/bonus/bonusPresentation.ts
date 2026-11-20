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

/** CNY fen to a display string. Never rounds to a fake zero for unset values. */
export function formatBonusAmount(minor: number): string {
  if (!Number.isFinite(minor)) return '—'
  const sign = minor < 0 ? '-' : ''
  const abs = Math.abs(Math.trunc(minor))
  const yuan = Math.floor(abs / 100)
  const fen = abs % 100
  return `${sign}¥${yuan.toLocaleString('zh-CN')}.${String(fen).padStart(2, '0')}`
}
