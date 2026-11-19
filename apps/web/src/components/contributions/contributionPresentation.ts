import { ApiError } from '@/api/client'
import type {
  ContributionCase,
  ContributionConfirmation,
  ContributionNotice,
  ContributionShare,
  ContributionVersion,
} from '@/api/contributions'

/** Parse decimal characters only: never multiply a floating-point percentage. */
export function percentToBasisPoints(value: string): number | null {
  const normalized = value.trim()
  if (!/^(?:0|[1-9]\d{0,2})(?:\.\d{1,2})?$/.test(normalized)) return null
  const [whole = '', fractional = ''] = normalized.split('.')
  const basisPoints = Number(whole) * 100 + Number(fractional.padEnd(2, '0'))
  return basisPoints <= 10000 ? basisPoints : null
}

export function formatBasisPoints(value: number): string {
  if (!Number.isSafeInteger(value) || value < 0) return '无效份额'
  return `${Math.trunc(value / 100)}.${String(Math.abs(value % 100)).padStart(2, '0')}%`
}

export function totalBasisPoints(shares: ReadonlyArray<Pick<ContributionShare, 'share_bps'>>) {
  return shares.reduce((total, share) => total + share.share_bps, 0)
}

export function canSubmitShares(shares: ReadonlyArray<Pick<ContributionShare, 'share_bps'>>) {
  return (
    shares.length > 0 &&
    shares.every((share) => Number.isSafeInteger(share.share_bps) && share.share_bps > 0) &&
    totalBasisPoints(shares) === 10000
  )
}

const caseStatuses: Record<ContributionCase['case_workflow_status'], string> = {
  draft: '草案',
  pending_review: '待核定',
  confirmed: '已确认',
  disputed: '存在争议',
  revising: '修订中',
  abandoned: '草案已放弃',
}
const versionStatuses: Record<ContributionVersion['status'], string> = {
  draft: '草案（非正式贡献）',
  pending_review: '待独立核定',
  confirmed: '已确认',
  superseded: '已被替代',
  overturned: '已被推翻',
  abandoned: '已放弃',
}

export const caseStatusLabel = (status: ContributionCase['case_workflow_status']) =>
  caseStatuses[status]
export const versionStatusLabel = (status: ContributionVersion['status']) => versionStatuses[status]

export function contributionError(reason: unknown): string {
  if (!(reason instanceof ApiError)) return '无法连接到 Nexora API，请稍后重试。'
  const guidance: Record<number, string> = {
    401: '登录已失效，请重新登录。',
    403: '当前没有操作权限，或不符合独立性要求。',
    404: '该资源不存在或当前不可访问。',
    409: '状态可能已改变，请以重新读取的权威数据为准。核对后重新操作，不会自动覆盖。',
    422: '输入不符合要求，请检查各项内容。',
    500: '服务暂时不可用，请稍后重试。',
  }
  const prefix = reason.status ? guidance[reason.status] : undefined
  const fields = Object.values(reason.fields ?? {}).join('；')
  return [prefix, reason.message, fields].filter(Boolean).join(' ')
}

/** Only a matching authenticated receipt is evidence; preparing a notice is not delivery. */
export function verifiedReceipt(
  notice: ContributionNotice,
  confirmation: ContributionConfirmation,
): boolean {
  const receipt = notice.receipt
  return Boolean(
    notice.channel === 'authenticated_ack' &&
    receipt &&
    notice.case_version_id === confirmation.case_version_id &&
    notice.confirmation_id === confirmation.id &&
    receipt.notice_id === notice.id &&
    receipt.case_version_id === notice.case_version_id &&
    receipt.confirmation_id === notice.confirmation_id &&
    receipt.subject_id === notice.recipient_id &&
    receipt.content_digest === confirmation.version_digest &&
    [receipt.verified_at, receipt.sent_at, receipt.acknowledged_at].every(
      (value) => value && Number.isFinite(Date.parse(value)),
    ),
  )
}
